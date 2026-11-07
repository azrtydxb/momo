// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

var lockfreeClock = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

// baseDocs returns a first migration for docsSchema and an engine over it.
func baseDocs(t *testing.T) (*migrate.Engine, fstest.MapFS) {
	t.Helper()
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m, err := first.Diff(context.Background(), docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	dir := mapFS(m.Files)
	return offlineEngine(t, dir, lockfreeClock), dir
}

func upFiles(m *migrate.Migration) []string {
	var names []string
	for n := range m.Files {
		if strings.HasSuffix(n, ".up.sql") {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}

// TestDiffConcurrentlyReachesRenderer: a model index declared Concurrently()
// on an existing table must come out as CREATE INDEX CONCURRENTLY in a
// no-transaction file. Normalization used to clear the flag, so diff wrote a
// plain CREATE INDEX that blocks writes for the whole build.
func TestDiffConcurrentlyReachesRenderer(t *testing.T) {
	e, dir := baseDocs(t)
	d := docsSchema()
	d.Tables[0].Indexes = append(d.Tables[0].Indexes, schema.Index{
		Name: "docs_id_title", Concurrently: true,
		Parts: []schema.IndexPart{{Column: "id"}, {Column: "title"}},
	})
	m, err := e.Diff(context.Background(), d, "more_idx", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	if m.Next != nil || len(upFiles(m)) != 1 {
		t.Fatalf("only a concurrent index: want one file, got %v", upFiles(m))
	}
	up := string(m.Files[upFiles(m)[0]])
	if !m.NoTx || !strings.Contains(up, "-- momo:no-transaction\n") || !strings.Contains(up, `CREATE INDEX CONCURRENTLY "docs_id_title"`) {
		t.Fatalf("want a no-transaction CONCURRENTLY index:\n%s", up)
	}
	// Idempotent against the snapshot just written.
	for n, b := range m.Files {
		dir[n] = &fstest.MapFile{Data: b}
	}
	again, err := offlineEngine(t, dir, lockfreeClock.Add(time.Hour)).Diff(context.Background(), d, "noop", migrate.DiffOptions{})
	if err != nil || again != nil {
		t.Fatalf("a repeated diff must report no changes: %v %v", again, err)
	}
}

// TestDiffSplitsConcurrentIndexFromTransactionalDDL: CREATE TABLE and a
// concurrent index in one diff must not share a no-transaction file, or a
// failure midway leaves half the DDL applied.
func TestDiffSplitsConcurrentIndexFromTransactionalDDL(t *testing.T) {
	d := docsSchema()
	d.Tables[0].Indexes[0].Concurrently = true
	d.Tables = append(d.Tables, schema.Table{
		Name: "notes", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"},
	})
	e := offlineEngine(t, fstest.MapFS{}, lockfreeClock)
	m, err := e.Diff(context.Background(), d, "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	if m.NoTx || m.Next == nil || !m.Next.NoTx {
		t.Fatalf("want a transactional head and a no-transaction tail, got NoTx=%v Next=%v", m.NoTx, m.Next)
	}
	ups := upFiles(m)
	if len(ups) != 2 {
		t.Fatalf("want two up files, got %v", ups)
	}
	head, tail := string(m.Files[ups[0]]), string(m.Files[ups[1]])
	if strings.Contains(head, "no-transaction") || strings.Contains(head, "CONCURRENTLY") || !strings.Contains(head, `CREATE TABLE "docs"`) {
		t.Fatalf("head must hold the tables, transactional:\n%s", head)
	}
	if !strings.Contains(tail, "-- momo:no-transaction\n") || !strings.Contains(tail, `CREATE INDEX CONCURRENTLY "docs_title"`) || strings.Contains(tail, "CREATE TABLE") {
		t.Fatalf("tail must hold only the concurrent index:\n%s", tail)
	}
	if m.Next.Version <= m.Version {
		t.Fatalf("tail version %s must follow head %s", m.Next.Version, m.Version)
	}
	// The pair is a valid directory, and a repeated diff reports nothing.
	dir := mapFS(m.Files)
	rep, err := migrate.ValidateOffline(dir)
	if err != nil || len(rep.Problems) > 0 {
		t.Fatalf("validate: %v %v", rep, err)
	}
	again, err := offlineEngine(t, dir, lockfreeClock.Add(time.Hour)).Diff(context.Background(), d, "noop", migrate.DiffOptions{})
	if err != nil || again != nil {
		t.Fatalf("a repeated diff must report no changes: %v %v", again, err)
	}
}

// TestDiffUniqueAndCheckAreLockSafe: ADD CHECK is NOT VALID with a separate
// VALIDATE CONSTRAINT after the transactional file; ADD UNIQUE is a
// concurrent unique index adopted with USING INDEX, in the no-transaction file.
func TestDiffUniqueAndCheckAreLockSafe(t *testing.T) {
	e, _ := baseDocs(t)
	d := docsSchema()
	d.Tables[0].Uniques = []schema.Unique{{Name: "docs_title_key", Columns: []string{"title"}}}
	d.Tables[0].Checks = []schema.Check{{Name: "docs_id_pos", Expr: "id > 0"}}
	m, err := e.Diff(context.Background(), d, "constraints", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	if m.Next == nil {
		t.Fatal("want a no-transaction tail")
	}
	head, tail := string(m.Files[m.Version+"_constraints.up.sql"]), string(m.Files[m.Next.Version+"_"+m.Next.Name+".up.sql"])
	if !strings.Contains(head, `ADD CONSTRAINT "docs_id_pos" CHECK (id > 0) NOT VALID`) || strings.Contains(head, "CONCURRENTLY") || strings.Contains(head, "no-transaction") {
		t.Fatalf("head must add the check NOT VALID and stay transactional:\n%s", head)
	}
	for _, want := range []string{
		"-- momo:no-transaction\n",
		`CREATE UNIQUE INDEX CONCURRENTLY "docs_title_key" ON "docs" ("title")`,
		`ADD CONSTRAINT "docs_title_key" UNIQUE USING INDEX "docs_title_key"`,
		`VALIDATE CONSTRAINT "docs_id_pos"`,
	} {
		if !strings.Contains(tail, want) {
			t.Fatalf("tail lacks %q:\n%s", want, tail)
		}
	}
}

// TestDiffReportsRiskyChanges: dropping an index or constraint and adding a
// unique or check constraint are not destructive, but each gets a risk hint.
func TestDiffReportsRiskyChanges(t *testing.T) {
	e, _ := baseDocs(t)
	add := docsSchema()
	add.Tables[0].Uniques = []schema.Unique{{Name: "docs_title_key", Columns: []string{"title"}}}
	add.Tables[0].Checks = []schema.Check{{Name: "docs_id_pos", Expr: "id > 0"}}
	m, err := e.Diff(context.Background(), add, "constraints", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	wantHints(t, m.Hints, "add_unique docs.docs_title_key", "add_check docs.docs_id_pos")
	for _, c := range m.Changes {
		if (c.Kind == migrate.AddUnique || c.Kind == migrate.AddCheck) && !c.Risky {
			t.Errorf("%s must be flagged Risky", c.Kind)
		}
	}

	drop := docsSchema()
	drop.Tables[0].Indexes = nil
	m, err = e.Diff(context.Background(), drop, "drop_idx", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	wantHints(t, m.Hints, "drop_index docs.docs_title")
	if m.Destructive {
		t.Error("dropping an index is risky, not destructive")
	}
}

func wantHints(t *testing.T, hints []string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !slices.ContainsFunc(hints, func(h string) bool { return strings.HasPrefix(h, "risky: ") && strings.Contains(h, s) }) {
			t.Errorf("no risky hint mentioning %q in %q", s, hints)
		}
	}
}

// TestDestructiveErrorTextMatchesBehavior: ErrDestructive is raised by diff
// only, so its text must not promise that a file directive authorizes it, and
// the directive must stay an accepted, informational marker.
func TestDestructiveErrorTextMatchesBehavior(t *testing.T) {
	if strings.Contains(migrate.ErrDestructive.Error(), "momo:destructive") {
		t.Fatalf("ErrDestructive text points at the informational directive: %q", migrate.ErrDestructive)
	}
	if !strings.Contains(migrate.ErrDestructive.Error(), "--allow-destructive") {
		t.Fatalf("ErrDestructive must name the flag: %q", migrate.ErrDestructive)
	}
	e, _ := baseDocs(t)
	_, err := e.Diff(context.Background(), &schema.Schema{Dialect: "postgres"}, "drop_docs", migrate.DiffOptions{})
	if !errors.Is(err, migrate.ErrDestructive) {
		t.Fatalf("want ErrDestructive, got %v", err)
	}
	dirs, err := migrate.ParseDirectives([]byte("-- momo:destructive\nDROP TABLE x;"))
	if err != nil || !dirs.Destructive {
		t.Fatalf("directive must stay accepted: %v %v", dirs, err)
	}
}
