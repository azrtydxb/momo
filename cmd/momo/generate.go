// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/config"
)

// runGenerate implements `momo generate [-c momo.yaml] [--check] [patterns...]`.
func runGenerate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("c", "", "path to momo.yaml (default: momo.yaml in the working directory, if present)")
	check := fs.Bool("check", false, "report stale or orphaned generated files without writing")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: momo generate [-c momo.yaml] [--check] [patterns...]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	file, dir, err := loadConfigFile(*cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "momo: %v\n", err)
		return exitUsage
	}
	cfg, err := config.ToConfig(file, dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "momo: %v\n", err)
		return exitUsage
	}
	cfg.Plugins, err = selectPlugins(file.Plugins)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "momo: %v\n", err)
		return exitUsage
	}

	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = file.Packages
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	g, diags, err := gen.Load(patterns, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "momo: %v\n", err)
		return exitUsage
	}
	if len(diags) > 0 {
		printDiagnostics(stderr, diags)
		return exitFindings
	}

	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: moduleRoot(g, dir), Check: *check})
	if err := gen.Emit(g, w); err != nil {
		// After an error the Writer holds partial output: never Commit it.
		return reportEmitError(stderr, err)
	}
	changed, err := w.Commit()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "momo: %v\n", err)
		if errors.Is(err, gen.ErrHandwritten) || errors.Is(err, gen.ErrCollision) {
			return exitFindings
		}
		return exitUsage
	}
	if !*check {
		for _, p := range changed {
			_, _ = fmt.Fprintln(stdout, p)
		}
		return exitOK
	}

	orphans, err := w.Orphans(packageDirs(g))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "momo: %v\n", err)
		return exitUsage
	}
	stale := dedupe(append(append([]string{}, changed...), orphans...))
	for _, p := range stale {
		_, _ = fmt.Fprintf(stdout, "stale: %s\n", p)
	}
	if len(stale) > 0 {
		return exitFindings
	}
	return exitOK
}

// loadConfigFile reads the explicit -c file, or momo.yaml in the working
// directory when present, or falls back to defaults. dir is the directory the
// patterns resolve against.
func loadConfigFile(path string) (*config.File, string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	explicit := path != ""
	if !explicit {
		path = filepath.Join(wd, "momo.yaml")
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return &config.File{Version: 1}, wd, nil
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
			return nil, fmt.Errorf("momo.yaml: unknown plugin %q", n)
		}
		out = append(out, p)
	}
	return out, nil
}

func printDiagnostics(w io.Writer, diags []gen.Diagnostic) {
	for _, d := range diags {
		_, _ = fmt.Fprintln(w, d.String())
	}
}

func reportEmitError(stderr io.Writer, err error) int {
	var de *gen.ErrDiagnostics
	if errors.As(err, &de) {
		printDiagnostics(stderr, de.Diagnostics)
		return exitFindings
	}
	_, _ = fmt.Fprintf(stderr, "momo: %v\n", err)
	return exitUsage
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
