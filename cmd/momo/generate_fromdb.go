// SPDX-License-Identifier: Apache-2.0

package main

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
	l := &cli{out: stdout, errOut: stderr, env: os.Getenv, dialectName: "postgres"}
	var err error
	if l.cwd, err = os.Getwd(); err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	if l.file, l.cfgDir, err = loadConfigFile(cfgPath, l.cwd); err != nil {
		return l.errf(exitUsage, "%v", err)
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
	e, closeFn, code := l.openEngineAt(ctx, dsn)
	if e == nil {
		return code
	}
	defer closeFn()
	live, err := e.Inspect(ctx)
	if err != nil {
		return l.errf(exitDatabase, "inspect: %v", err)
	}
	res, err := fromdb.Generate(live, fromdb.Options{Package: pkg, Namer: namer, Config: l.file.Introspect})
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
	if _, err := fromdb.Apply(outDir, actions, writeModelFile); err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	l.excludeHint()
	if !a.noSnapshot {
		if code := l.writeInitSnapshot(ctx, e, res.Snapshot, a.name); code != exitOK {
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
	if src == "-" || strings.Contains(src, "://") || strings.Contains(src, "=") {
		l.c.url = src
		return l.databaseURL()
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

// writeInitSnapshot writes the init migration and snapshot of the inspected
// schema, unless the migration directory already has a snapshot.
func (l *cli) writeInitSnapshot(ctx context.Context, e *migrate.Engine, snap *schema.Schema, name string) int {
	if name == "" {
		name = "init"
	}
	dir := l.migrationDir()
	switch latest, err := migrate.LatestSnapshot(dirFS(dir)); {
	case err == nil:
		l.say("%s already holds the snapshot %s: no init migration written; use momo migrate diff for later changes", l.rel(dir), latest.Version)
		return exitOK
	case !errors.Is(err, migrate.ErrNoSnapshot):
		return l.failure(err)
	}
	m, err := e.Diff(ctx, snap, name, migrate.DiffOptions{})
	if err != nil {
		return l.failure(err)
	}
	if m == nil {
		l.say("the database has no objects to snapshot")
		return exitOK
	}
	written, err := writeNewFiles(dir, m.Files)
	for _, p := range written {
		l.say("%s", l.rel(p))
	}
	if err != nil {
		return l.errf(exitUsage, "%v", err)
	}
	last := m
	if m.Next != nil {
		last = m.Next
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
