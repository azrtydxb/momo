// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/config"
)

const generateUsage = `Usage: momo generate [-c momo.yaml | --config momo.yaml] [--check] [patterns...]

Generates code for the model packages matching patterns. Without patterns it
uses "packages" from momo.yaml (resolved against the config directory), then
./... (resolved against the working directory).

momo.yaml is found by walking up from the working directory to the directory
holding go.mod, so "//go:generate momo generate" works from any package
directory. Command line patterns always resolve against the working directory.

With --check nothing is written: stale or orphaned generated files are listed
as "stale: <path>" and the exit code is 1.

Plugins: only the plugins built into this binary can be enabled in momo.yaml.
To run custom generator plugins, call gen.Emit with your plugins from Go code.
`

// runGenerate implements `momo generate [-c momo.yaml] [--check] [patterns...]`.
func runGenerate(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "-h" || a == "-help" || a == "--help" {
			_, _ = fmt.Fprint(stdout, generateUsage)
			return exitOK
		}
	}
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // bad values are echoed by the flag package; print them redacted below
	var cfgPath string
	var check bool
	fs.StringVar(&cfgPath, "c", "", "path to momo.yaml")
	fs.StringVar(&cfgPath, "config", "", "path to momo.yaml")
	fs.BoolVar(&check, "check", false, "report stale or orphaned generated files without writing")
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(stdout, generateUsage)
			return exitOK
		}
		code := fail(stderr, err)
		_, _ = fmt.Fprintf(stderr, "\n%s", generateUsage)
		return code
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fail(stderr, err)
	}
	file, cfgDir, err := loadConfigFile(cfgPath, cwd)
	if err != nil {
		return fail(stderr, err)
	}
	patterns, base := fs.Args(), cwd
	if len(patterns) == 0 && len(file.Packages) > 0 {
		patterns, base = file.Packages, cfgDir
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	if err := checkPatterns(patterns, base); err != nil {
		return fail(stderr, err)
	}
	cfg, err := config.ToConfig(file, base)
	if err != nil {
		return fail(stderr, err)
	}
	if cfg.Plugins, err = selectPlugins(file.Plugins); err != nil {
		return fail(stderr, err)
	}

	g, diags, err := gen.Load(patterns, cfg)
	if err != nil {
		return fail(stderr, err)
	}
	if len(diags) > 0 {
		printDiagnostics(stderr, diags, cwd)
		return exitFindings
	}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: moduleRoot(g, base), Check: check})
	if err := gen.Emit(g, w); err != nil {
		// After an error the Writer holds partial output: never Commit it.
		var de *gen.ErrDiagnostics
		if errors.As(err, &de) {
			printDiagnostics(stderr, de.Diagnostics, cwd)
			return exitFindings
		}
		return fail(stderr, err)
	}
	changed, err := w.Commit()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "momo: %s\n", redactURL(err.Error()))
		if errors.Is(err, gen.ErrHandwritten) || errors.Is(err, gen.ErrCollision) {
			return exitFindings
		}
		return exitUsage
	}
	if !check {
		for _, p := range changed {
			_, _ = fmt.Fprintln(stdout, filepath.ToSlash(relPath(p, cwd)))
		}
		return exitOK
	}
	orphans, err := w.Orphans(packageDirs(g))
	if err != nil {
		return fail(stderr, err)
	}
	stale := dedupe(append(append([]string{}, changed...), orphans...))
	for _, p := range stale {
		_, _ = fmt.Fprintf(stdout, "stale: %s\n", filepath.ToSlash(relPath(p, cwd)))
	}
	if len(stale) > 0 {
		return exitFindings
	}
	return exitOK
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "momo: %s\n", redactURL(err.Error()))
	return exitUsage
}

// loadConfigFile reads the explicit config, or the first momo.yaml found
// walking up from cwd to the directory holding go.mod, or falls back to
// defaults. dir is the directory of the file read (cwd for defaults).
func loadConfigFile(path, cwd string) (*config.File, string, error) {
	if path == "" {
		path = discoverConfig(cwd)
		if path == "" {
			return &config.File{Version: 1}, cwd, nil
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	f, err := config.Load(abs)
	if err != nil {
		return nil, "", err
	}
	return f, filepath.Dir(abs), nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// discoverConfig returns the nearest momo.yaml between cwd and the module
// root, or "" when there is none. Without a go.mod only cwd is searched.
func discoverConfig(cwd string) string {
	root := cwd
	for d := cwd; ; d = filepath.Dir(d) {
		if exists(filepath.Join(d, "go.mod")) {
			root = d
			break
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	for d := cwd; ; d = filepath.Dir(d) {
		if p := filepath.Join(d, "momo.yaml"); exists(p) {
			return p
		}
		if d == root || filepath.Dir(d) == d {
			return ""
		}
	}
}

// checkPatterns rejects local patterns whose directory does not exist.
func checkPatterns(patterns []string, base string) error {
	for _, p := range patterns {
		local := p == "." || p == ".." || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") || filepath.IsAbs(p)
		if !local {
			continue
		}
		dir := strings.TrimSuffix(strings.TrimSuffix(p, "..."), "/")
		if dir == "" {
			dir = "."
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(base, dir)
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return fmt.Errorf("pattern %q matches no package directory", p)
		}
	}
	return nil
}

// selectPlugins returns the built-in plugins named in the config. Naming a
// plugin that is not built in is a configuration error.
func selectPlugins(want map[string]map[string]any) ([]gen.Plugin, error) {
	if len(want) == 0 {
		return nil, nil
	}
	have := map[string]gen.Plugin{}
	for _, p := range plugins() {
		have[p.Name()] = p
	}
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []gen.Plugin
	for _, n := range names {
		p, ok := have[n]
		if !ok {
			return nil, fmt.Errorf("momo.yaml: unknown plugin %q: the momo binary only runs its built-in plugins; "+
				"for custom generators call gen.Emit with your plugins from Go code", n)
		}
		out = append(out, p)
	}
	return out, nil
}

// relPath renders p relative to cwd, or unchanged when that is not possible.
func relPath(p, cwd string) string {
	if !filepath.IsAbs(p) {
		return p
	}
	bases := []string{cwd}
	if r, err := filepath.EvalSymlinks(cwd); err == nil && r != cwd {
		bases = append(bases, r)
	}
	for _, b := range bases {
		if r, err := filepath.Rel(b, p); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	for _, b := range bases {
		if r, err := filepath.Rel(b, p); err == nil {
			return r
		}
	}
	return p
}

func printDiagnostics(w io.Writer, diags []gen.Diagnostic, cwd string) {
	for _, d := range diags {
		d.Pos = token.Position{Filename: filepath.ToSlash(relPath(d.Pos.Filename, cwd)), Line: d.Pos.Line, Column: d.Pos.Column}
		_, _ = fmt.Fprintln(w, redactURL(d.String()))
	}
}

// moduleRoot is the directory every output must live under: the module of the
// loaded models, or dir when no model was found.
func moduleRoot(g *gen.Graph, dir string) string {
	for _, m := range g.Models {
		if m.ModuleDir != "" {
			return m.ModuleDir
		}
	}
	return dir
}

// packageDirs returns the loaded package directories without duplicates.
func packageDirs(g *gen.Graph) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, p := range g.Packages {
		if p.Dir != "" && !seen[p.Dir] {
			seen[p.Dir] = true
			dirs = append(dirs, p.Dir)
		}
	}
	sort.Strings(dirs)
	return dirs
}

// dedupe returns the sorted unique entries of in (package directories may
// nest, so one orphan can be found more than once).
func dedupe(in []string) []string {
	sort.Strings(in)
	out := in[:0]
	for i, p := range in {
		if i == 0 || p != in[i-1] {
			out = append(out, p)
		}
	}
	return out
}
