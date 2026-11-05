// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func docsSchema() *schema.Schema {
	// The test database has vector installed and inspection reports every
	// extension, so the desired schema declares it.
	return &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{{
		Name:       "docs",
		Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "title", Type: "TEXT", Default: "''"}},
		PrimaryKey: []string{"id"},
		Indexes:    []schema.Index{{Name: "docs_title", Parts: []schema.IndexPart{{Column: "title"}}}},
	}}}
}

func TestMigrationRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := docsSchema()
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := m.Diff(ctx, desired, "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	fsys := migratetest.FS(mig.Files)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	inspected, err := e.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !schema.Equal(inspected, desired) {
		t.Fatalf("round trip not empty\n%s", migratetest.Diff(inspected, desired))
	}
	again, err := e.Diff(ctx, desired, "noop", migrate.DiffOptions{})
	if err != nil || again != nil {
		t.Fatalf("no-change diff must write nothing: %v %v", again, err)
	}
}

func TestNoChangeWritesNothing(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := m.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
	for i := 0; i < 2; i++ {
		again, err := e.Diff(ctx, docsSchema(), "noop", migrate.DiffOptions{})
		if err != nil || again != nil {
			t.Fatalf("diff %d must write nothing: %v %v", i, again, err)
		}
	}
}

// TestInspectIgnoresHistoryTable catches an engine Inspect that reports the
// momo_migrations history table, which would show up as drift or a DROP TABLE.
func TestInspectIgnoresHistoryTable(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := m.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if !tableExists(t, db, "momo_migrations") {
		t.Fatal("setup: history table must exist")
	}
	s, err := e.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tb := range s.Tables {
		if tb.Name == "momo_migrations" {
			t.Fatalf("Inspect must not report the history table: %+v", s.Tables)
		}
	}
	// Diffing the live schema against an empty snapshot must not propose to
	// drop the history table either.
	changes, _, err := migrate.Diff(s, &schema.Schema{Dialect: "postgres"}, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Kind == migrate.DropTable && c.Table == "momo_migrations" {
			t.Fatalf("history table proposed for drop: %+v", c)
		}
	}
}

func offlineEngine(t *testing.T, fsys fstest.MapFS, now time.Time) *migrate.Engine {
	t.Helper()
	return migratetest.NewEngine(t, mpg.New(), nil, fsys, migrate.WithClock(func() time.Time { return now }))
}

func mapFS(files map[string][]byte) fstest.MapFS {
	dir := fstest.MapFS{}
	for k, v := range files {
		dir[k] = &fstest.MapFile{Data: v}
	}
	return dir
}

func TestDiffFilesAndHeader(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
	e := offlineEngine(t, fstest.MapFS{}, now)
	mig, err := e.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	if mig.Version != "20260102020405" || mig.Name != "init" {
		t.Fatalf("version/name = %s/%s", mig.Version, mig.Name)
	}
	up := string(mig.Files["20260102020405_init.up.sql"])
	if !strings.HasPrefix(up, "-- momo:generated\n") || strings.Contains(up, "momo:destructive") {
		t.Fatalf("unexpected header:\n%s", up)
	}
	if !strings.Contains(up, "CREATE TABLE") || !strings.Contains(up, ";\n\n") {
		t.Fatalf("statements not rendered or separated:\n%s", up)
	}
	snap, err := migrate.ReadSnapshot(bytes.NewReader(mig.Files["20260102020405_init.snapshot.json"]))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != "20260102020405" || snap.ParentVersion != "" {
		t.Fatalf("snapshot version/parent = %q/%q", snap.Version, snap.ParentVersion)
	}
	sum, err := migrate.ParseSum(bytes.NewReader(mig.Files["momo.sum"]))
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Files) != 2 {
		t.Fatalf("momo.sum entries = %v", sum.Files)
	}
	if bad := migrate.VerifySum(migratetest.FS(mig.Files), sum); len(bad) != 0 {
		t.Fatalf("momo.sum does not match files: %v", bad)
	}
}

func TestDiffSnapshotParentAndSum(t *testing.T) {
	ctx := context.Background()
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	desired := docsSchema()
	desired.Tables[0].Columns = append(desired.Tables[0].Columns, schema.Column{Name: "body", Type: "TEXT", Nullable: true})
	second := offlineEngine(t, mapFS(m1.Files), time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	m2, err := second.Diff(ctx, desired, "add_body", migrate.DiffOptions{})
	if err != nil || m2 == nil {
		t.Fatal(m2, err)
	}
	snap, err := migrate.ReadSnapshot(bytes.NewReader(m2.Files["20260102000000_add_body.snapshot.json"]))
	if err != nil {
		t.Fatal(err)
	}
	if snap.ParentVersion != "20260101000000" {
		t.Fatalf("parent = %q", snap.ParentVersion)
	}
	if !strings.Contains(string(m2.Files["20260102000000_add_body.up.sql"]), "ADD COLUMN") {
		t.Fatalf("second migration must only add the column:\n%s", m2.Files["20260102000000_add_body.up.sql"])
	}
	sum, err := migrate.ParseSum(bytes.NewReader(m2.Files["momo.sum"]))
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Files) != 4 {
		t.Fatalf("momo.sum must cover old and new files: %v", sum.Files)
	}
	if _, ok := m2.Files["20260101000000_init.up.sql"]; ok {
		t.Fatal("Files must hold only new files")
	}
}

func TestDiffDestructiveHeader(t *testing.T) {
	ctx := context.Background()
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	e := offlineEngine(t, mapFS(m1.Files), time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	empty := &schema.Schema{Dialect: "postgres"}
	if _, err := e.Diff(ctx, empty, "drop_docs", migrate.DiffOptions{}); !errors.Is(err, migrate.ErrDestructive) {
		t.Fatalf("want ErrDestructive, got %v", err)
	}
	m, err := e.Diff(ctx, empty, "drop_docs", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	if !m.Destructive || !strings.Contains(string(m.Files["20260102000000_drop_docs.up.sql"]), "-- momo:destructive\n") {
		t.Fatalf("destructive header missing:\n%s", m.Files["20260102000000_drop_docs.up.sql"])
	}
}

func TestDiffNoTransactionHeader(t *testing.T) {
	ctx := context.Background()
	d := docsSchema()
	d.Tables[0].Indexes[0].Concurrently = true
	e := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m, err := e.Diff(ctx, d, "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	up := string(m.Files["20260101000000_init.up.sql"])
	if !m.NoTx || !strings.Contains(up, "-- momo:no-transaction\n") {
		t.Fatalf("no-transaction header missing:\n%s", up)
	}
	if strings.Index(up, "momo:generated") > strings.Index(up, "momo:no-transaction") {
		t.Fatalf("generated must come first:\n%s", up)
	}
}

func TestDiffRejectsBadNameAndBumpsClash(t *testing.T) {
	ctx := context.Background()
	e := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	for _, n := range []string{"", "Init", "1init", "add-col", "a b", "../x"} {
		if _, err := e.Diff(ctx, docsSchema(), n, migrate.DiffOptions{}); err == nil {
			t.Errorf("name %q must be rejected", n)
		}
	}
	m, err := e.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	same := offlineEngine(t, mapFS(m.Files), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	other := docsSchema()
	other.Tables[0].Columns = append(other.Tables[0].Columns, schema.Column{Name: "b", Type: "TEXT", Nullable: true})
	m2, err := same.Diff(ctx, other, "more", migrate.DiffOptions{})
	if err != nil || m2 == nil {
		t.Fatal(m2, err)
	}
	if m2.Version != "20260101000001" {
		t.Fatalf("a version equal to the latest must move to the next second, got %s", m2.Version)
	}
}

func TestDiffSurfacesOrderCycle(t *testing.T) {
	ctx := context.Background()
	d := &schema.Schema{Dialect: "postgres", Views: []schema.View{
		{Name: "a", Query: "SELECT 1 FROM b", Depends: []string{"b"}},
		{Name: "b", Query: "SELECT 1 FROM a", Depends: []string{"a"}},
	}}
	e := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if m, err := e.Diff(ctx, d, "views", migrate.DiffOptions{}); err == nil {
		t.Fatalf("a view cycle must be an error, got %v", m)
	}
}
