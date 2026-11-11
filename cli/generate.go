// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/config"
	"github.com/azrtydxb/momo/migrate"
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

Line endings: generated files are written with LF. A copy that differs only in
CRLF line endings (for example a checkout with core.autocrlf=true) counts as up
to date. To keep checkouts byte-identical anyway, add this to .gitattributes:

    *_gen.go text eol=lf

Models from a database:

    momo generate --from-db <url|-|ENVVAR> [--out dir] [--package name]
                  [--schema a,b] [--no-snapshot] [--overwrite] [--check]

Inspects the PostgreSQL database and writes one model file per table into the
model package (default: introspect.out, else the first local entry of
packages, else ./models), plus the init migration and snapshot into the
migration directory when it has none. Then run "momo generate" for the
generated code and "momo migrate baseline --version <init version>".

The value of --from-db is a database URL, "-" to read the URL from standard
input, or the name of an environment variable that holds it. Prefer the last
two: a URL on the command line is visible in the process list.

Settings that the database cannot tell (Prisma updatedAt, Json shapes,
Unsupported columns) go in the introspect section of momo.yaml: package and out
(the model package name and directory), skip (table globs), only (table globs,
the inverse of skip: a table matching none gets no model), functions ("all"
writes every function on the first model), dbdefault ("all" tags every NOT NULL
column with a default), uuid (momo or google), types (SQL type to Go type),
columns (match "table.column" globs, go, import, name, tags), tables (name,
tags), inverse (true also writes the has_many or has_one inverse of every
belongs_to) and relations (needs inverse: true; renames or skips one inverse,
keyed "Model.Field" by the name derived before any collision suffix; a key that
matches no inverse is reported as a warning). A glob matches the table name, or
schema.name for a table outside the current schema; a table in the current
schema is matched by its bare name. The rules are applied again on every run.

A re-run rewrites the model files that nobody edited. A file with hand edits,
or one that --from-db did not write, is kept: the regenerated version goes
next to it as <file>.new, the run lists "conflict: <file>" and exits 1.
--overwrite replaces such files, except a hand-edited file whose table did not
change in the database: that one is kept as it is (delete the file to reset it). --check writes nothing and
exits 1 when a file would change. A file whose table is gone is listed as "orphan" and is
never deleted.

Constraints the models cannot declare (renamed or single-column non-primary-key foreign keys,
expression index keys with an order or operator class) are left out of the models and
the snapshot and listed as warnings; the baseline then reports them. A stored generated
column becomes a read_only field with the generated tag. A column
type with no Go mapping is an error and nothing is written.

Plugins: the plugins: section of momo.yaml enables the plugins built into this
binary (graphql, grpc, openapi, viz) and sets their options; a relative dir is
relative to momo.yaml. Your own plugins come from a binary built with
cli.Main(plugins...) from github.com/azrtydxb/momo/cli, or from gen.Emit.
`

// runGenerate implements `momo generate [-c momo.yaml] [--check] [patterns...]`.
func runGenerate(args []string, stdout, stderr io.Writer, available ...gen.Plugin) int {
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
	var fromDB fromDBArgs
	fs.StringVar(&fromDB.src, "from-db", "", "generate models from this database: a URL, - for stdin, or an environment variable name")
	fs.StringVar(&fromDB.out, "out", "", "with --from-db: the model package directory")
	fs.StringVar(&fromDB.pkg, "package", "", "with --from-db: the Go package name")
	fs.StringVar(&fromDB.schemas, "schema", "", "with --from-db: comma-separated PostgreSQL schemas to read")
	fs.StringVar(&fromDB.name, "migration-name", "init", "with --from-db: name of the init migration")
	fs.BoolVar(&fromDB.noSnapshot, "no-snapshot", false, "with --from-db: do not write the init migration and snapshot")
	fs.BoolVar(&fromDB.overwrite, "overwrite", false, "with --from-db: replace model files that have hand edits")
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

	if fromDB.src != "" || fromDB.othersSet() {
		if fromDB.src == "" {
			return fail(stderr, errors.New("--out, --package, --schema, --no-snapshot and --overwrite need --from-db"))
		}
		if fs.NArg() > 0 {
			return fail(stderr, errors.New("--from-db takes no package patterns"))
		}
		fromDB.check = check
		return runGenerateFromDB(fromDB, cfgPath, stdout, stderr)
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
	if cfg.Plugins, err = selectPlugins(file.Plugins, available, cfgDir); err != nil {
		return fail(stderr, fmt.Errorf("momo.yaml: %w", err))
	}

	g, diags, err := gen.Load(patterns, cfg)
	if err != nil {
		return fail(stderr, err)
	}
	if len(diags) > 0 {
		printDiagnostics(stderr, diags, cwd)
		return exitFindings
	}
	printWarnings(stderr, g.Warnings, cwd)
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
		_, _ = fmt.Fprintf(stderr, "momo: %s\n", migrate.RedactURLs(err.Error()))
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
	_, _ = fmt.Fprintf(stderr, "momo: %s\n", migrate.RedactURLs(err.Error()))
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

// selectPlugins returns the plugins the config names: the built-in ones
// (graphql, grpc, openapi, viz) built from their options by buildPlugins, and
// the others from those the binary offers. Naming a plugin that is neither is a
// configuration error. The result is in the order of the sorted names.
func selectPlugins(want map[string]map[string]any, available []gen.Plugin, out string) ([]gen.Plugin, error) {
	if len(want) == 0 {
		return nil, nil
	}
	builtin := map[string]map[string]any{}
	for n, opts := range want {
		if slices.Contains(builtinPlugins, n) {
			builtin[n] = opts
		}
	}
	built, err := buildPlugins(builtin, out)
	if err != nil {
		return nil, err
	}
	have := map[string]gen.Plugin{}
	for _, p := range available {
		have[p.Name()] = p
	}
	for _, p := range built {
		have[p.Name()] = p
	}
	var res []gen.Plugin
	for _, n := range slices.Sorted(maps.Keys(want)) {
		p, ok := have[n]
		if !ok {
			return nil, fmt.Errorf("unknown plugin %q: this binary only runs the plugins built into it; "+
				"build a binary with cli.Main(plugins...) or call gen.Emit with your plugins from Go code", n)
		}
		res = append(res, p)
	}
	return res, nil
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
		_, _ = fmt.Fprintln(w, migrate.RedactURLs(d.String()))
	}
}

// printWarnings writes findings that do not stop generation, one per line
// after "warning:".
func printWarnings(w io.Writer, warns []gen.Diagnostic, cwd string) {
	for _, d := range warns {
		d.Pos = token.Position{Filename: filepath.ToSlash(relPath(d.Pos.Filename, cwd)), Line: d.Pos.Line, Column: d.Pos.Column}
		_, _ = fmt.Fprintln(w, "warning: "+migrate.RedactURLs(d.String()))
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
