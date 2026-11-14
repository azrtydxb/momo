// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/fromdb"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// fromDBArgs are the flags of `momo generate --from-db`.
type fromDBArgs struct {
	src, out, pkg, schemas, name string
	noSnapshot, overwrite, check bool
}

// othersSet reports whether any flag that only means something with
// --from-db was given.
func (a fromDBArgs) othersSet() bool {
	return a.out != "" || a.pkg != "" || a.schemas != "" || a.noSnapshot || a.overwrite
}

// runGenerateFromDB inspects the database, writes one model file per table
// and, when the migration directory has no snapshot yet, the init migration
// and snapshot of the inspected schema.
func runGenerateFromDB(a fromDBArgs, cfgPath string, stdout, stderr io.Writer) int {
	l := &cli{out: stdout, errOut: stderr, env: os.Getenv}
	var err error
	if l.cwd, err = os.Getwd(); err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	if l.file, l.cfgDir, err = loadConfigFile(cfgPath, l.cwd); err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	// Resolve the dialect as the other commands do, and refuse one the
	// inspector cannot read before anything is opened or written.
	if l.dialectName, err = l.chooseDialect(); err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	if l.dialectName != string(dialect.Postgres) && l.dialectName != string(dialect.MySQL) {
		return l.errf(exitUsage, "--from-db reads PostgreSQL, MySQL and MariaDB databases only, not %s", l.dialectName)
	}
	if a.schemas != "" {
		l.file.Migrate.Schemas = strings.Split(a.schemas, ",")
	}
	namerName := l.file.Naming
	if namerName == "" {
		namerName = "momo"
	}
	namer, err := naming.ByName(namerName)
	if err != nil {
		return l.errf(exitUsage, "naming: %v", err)
	}
	outDir := l.modelDir(a.out)
	pkg := a.pkg
	if pkg == "" {
		pkg = l.file.Introspect.Package
	}
	if pkg == "" {
		pkg = packageNameFor(outDir)
	}

	ctx, cancel := interruptContext()
	defer cancel()
	dsn, err := l.fromDBURL(a.src)
	if err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	// introspect.skip tables get no model, so inspection skips them before
	// reading them: one the schema model cannot represent must not fail the run.
	savedExclude := l.file.Migrate.Exclude
	l.file.Migrate.Exclude = append(slices.Clone(savedExclude), l.file.Introspect.Skip...)
	e, closeFn, code := l.openFromDBEngine(ctx, dsn)
	l.file.Migrate.Exclude = savedExclude
	if e == nil {
		return code
	}
	defer closeFn()
	live, err := e.Inspect(ctx)
	if err != nil {
		return l.errf(exitDatabase, "inspect: %v", err)
	}
	if err := checkOnlyMatched(l.file.Introspect.Only, live.Tables, l.say); err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	flavor, extra, err := l.mysqlExtras(ctx)
	if err != nil {
		return l.errf(exitDatabase, "inspect: %v", err)
	}
	res, err := fromdb.Generate(live, fromdb.Options{
		Package: pkg, Dialect: dialect.Name(l.dialectName), Flavor: flavor, Extra: extra,
		Namer: namer, Config: l.file.Introspect,
	})
	if err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	for _, f := range res.Findings {
		l.say("%s", f)
	}
	if res.Failed() {
		return l.errf(exitFindings, "nothing was written: fix the errors above, for example with introspect.types or introspect.columns rules in momo.yaml")
	}
	actions, err := fromdb.Plan(outDir, res.Files, a.overwrite)
	if err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	stale := l.reportActions(outDir, actions, a.check)
	if a.check {
		if stale {
			return exitFindings
		}
		return exitOK
	}
	// Build the init migration before any model file is written: a snapshot
	// that fails must not leave model files behind without it.
	var initSnap *initSnapshot
	if !a.noSnapshot {
		var code int
		if initSnap, code = l.planInitSnapshot(ctx, e, res.Snapshot, a.name); code != exitOK {
			return code
		}
	}
	if _, err := fromdb.Apply(outDir, actions, writeModelFile); err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	l.excludeHint()
	if initSnap != nil {
		if code := l.writeInitSnapshot(initSnap); code != exitOK {
			return code
		}
	}
	for _, x := range actions {
		if x.Kind == fromdb.ActionConflict {
			return exitFindings
		}
	}
	return exitOK
}

// checkOnlyMatched reports introspect.only globs that matched no inspected
// table. Inspection already left out every table that matches none, so tables
// holds only the matches: with no match at all it is an error (a typo would
// otherwise read as an empty database), and a glob that matched none while
// others did is named in a warning.
func checkOnlyMatched(only []string, tables []schema.Table, say func(format string, args ...any)) error {
	if len(only) == 0 {
		return nil
	}
	var unmatched []string
	for _, g := range only {
		if !slices.ContainsFunc(tables, func(t schema.Table) bool {
			return schema.MatchObject([]string{g}, t.Schema, t.Name)
		}) {
			unmatched = append(unmatched, g)
		}
	}
	if len(tables) == 0 {
		return fmt.Errorf("introspect.only matched no table (%s): a table in the current schema is matched by its bare name, not public.name; a table outside it by schema.name", strings.Join(only, ", "))
	}
	if len(unmatched) > 0 {
		say("warning: introspect.only: %s matched no table", strings.Join(unmatched, ", "))
	}
	return nil
}

// modelDir resolves the output directory: the flag (against the working
// directory), introspect.out, the first local entry of packages, or models
// (against the config directory).
func (l *cli) modelDir(flagValue string) string {
	if flagValue != "" {
		return absAgainst(l.cwd, flagValue)
	}
	out := l.file.Introspect.Out
	if out == "" {
		for _, p := range l.file.Packages {
			if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
				out = strings.TrimSuffix(strings.TrimSuffix(p, "..."), "/")
				break
			}
		}
	}
	if out == "" {
		out = "models"
	}
	return absAgainst(l.cfgDir, out)
}

func absAgainst(base, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(base, p)
}

// packageNameFor derives a Go package name from the directory's base name.
func packageNameFor(dir string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(filepath.Base(dir)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	name := b.String()
	if name == "" || !token.IsIdentifier(name) {
		return "models"
	}
	return name
}

// fromDBURL resolves --from-db: "-" reads the URL from standard input, a
// value that looks like a URL or a key=value string is the connection string
// itself, and anything else names an environment variable.
func (l *cli) fromDBURL(src string) (string, error) {
	switch {
	case src == "-":
		if f, ok := migrateStdin.(*os.File); ok {
			if fi, err := f.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
				return "", errors.New("--from-db -: standard input is a terminal or a character device; pipe the URL in")
			}
		}
		b, err := io.ReadAll(io.LimitReader(migrateStdin, 1<<16))
		if err != nil {
			return "", fmt.Errorf("read the database URL from stdin: %w", err)
		}
		u := strings.TrimSpace(string(b))
		if u == "" {
			return "", errors.New("--from-db -: standard input holds no database URL")
		}
		l.noteURL(u)
		return u, nil
	case strings.Contains(src, "://") || strings.Contains(src, "=") || looksLikeMySQLDSN(src):
		_, _ = fmt.Fprintln(l.errOut, "momo: warning: --from-db puts the database URL in the process list, where other users can read it; pass - to read it from standard input, or the name of an environment variable")
		l.noteURL(src)
		return src, nil
	}
	u := l.env(src)
	if u == "" {
		return "", errors.New("--from-db " + src + ": the environment variable is empty; pass a database URL, - for standard input, or the name of a variable that holds it")
	}
	l.noteURL(u)
	return u, nil
}

// reportActions prints what the run does with each file and reports whether
// anything differs from the directory.
func (l *cli) reportActions(dir string, actions []fromdb.Action, check bool) (stale bool) {
	for _, a := range actions {
		p := l.rel(filepath.Join(dir, a.File))
		switch a.Kind {
		case fromdb.ActionUnchanged:
			continue
		case fromdb.ActionHeader:
			// The body equals the regenerated one: current for --check.
			if check {
				continue
			}
			l.say("header: %s: the body is current, the hash in the header line is refreshed", p)
		case fromdb.ActionOrphan:
			l.say("orphan: %s: %s", p, a.Detail)
		case fromdb.ActionConflict:
			if check {
				l.say("stale: %s", p)
			} else {
				l.say("conflict: %s: %s; the regenerated version is in %s.new, merge it by hand or rerun with --overwrite", p, a.Detail, p)
			}
		default:
			if check {
				l.say("stale: %s", p)
			} else {
				l.say("%s: %s", a.Kind, p)
			}
		}
		stale = true
	}
	return stale
}

// excludeHint names the skipped tables that migrate.exclude does not hide.
func (l *cli) excludeHint() {
	var missing []string
	for _, g := range l.file.Introspect.Skip {
		if !slices.Contains(l.file.Migrate.Exclude, g) {
			missing = append(missing, g)
		}
	}
	if len(missing) > 0 {
		l.say("hint: add %s to migrate.exclude in momo.yaml so the baseline ignores the skipped tables", strings.Join(missing, ", "))
	}
}

// initSnapshot is the outcome of planInitSnapshot: the migration to write, or
// the version of the snapshot that is already there, or neither when the
// database has no objects.
type initSnapshot struct {
	dir      string
	m        *migrate.Migration
	existing string
}

// planInitSnapshot builds the init migration and snapshot of the inspected
// schema without writing anything, unless the migration directory already has
// a snapshot.
func (l *cli) planInitSnapshot(ctx context.Context, e *migrate.Engine, snap *schema.Schema, name string) (*initSnapshot, int) {
	if name == "" {
		name = "init"
	}
	dir := l.migrationDir()
	switch latest, err := migrate.LatestSnapshot(dirFS(dir)); {
	case err == nil:
		return &initSnapshot{dir: dir, existing: latest.Version}, exitOK
	case !errors.Is(err, migrate.ErrNoSnapshot):
		return nil, l.failure(err)
	}
	m, err := e.Diff(ctx, snap, name, migrate.DiffOptions{})
	if err != nil {
		return nil, l.failure(err)
	}
	return &initSnapshot{dir: dir, m: m}, exitOK
}

// writeInitSnapshot writes the init migration planned by planInitSnapshot.
func (l *cli) writeInitSnapshot(s *initSnapshot) int {
	switch {
	case s.existing != "":
		l.say("%s already holds the snapshot %s: no init migration written; use momo migrate diff for later changes", l.rel(s.dir), s.existing)
		return exitOK
	case s.m == nil:
		l.say("the database has no objects to snapshot")
		return exitOK
	}
	written, err := writeNewFiles(s.dir, s.m.Files)
	for _, p := range written {
		l.say("%s", l.rel(p))
	}
	if err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	last := s.m
	if s.m.Next != nil {
		last = s.m.Next
	}
	l.say("next: run momo generate, then momo migrate baseline --version %s", last.Version)
	return exitOK
}

// writeModelFile writes through a temporary file in the same directory, so a
// crash never leaves a half-written model.
func writeModelFile(path string, content []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- model source directories are committed, not secret
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".momo-*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if _, err = tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err = tmp.Chmod(0o644); err != nil { // #nosec G302 -- model source files are committed and read by the team
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
