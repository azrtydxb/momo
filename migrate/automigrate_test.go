// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func columnExists(t *testing.T, db *momo.DB, table, column string) bool {
	t.Helper()
	return firstValue(t, db, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = '`+table+`' AND column_name = '`+column+`')::text`) == "true"
}

// firstValue returns the first column of the first row as a string.
func firstValue(t *testing.T, db *momo.DB, q string) string {
	t.Helper()
	var s string
	withPinned(t, db, func(c momo.PinnedConn) {
		r, err := c.Query(context.Background(), q, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Close() }()
		if !r.Next() {
			t.Fatal("no row")
		}
		if err := r.Scan(&s); err != nil {
			t.Fatal(err)
		}
	})
	return s
}

func autoDesired() *schema.Schema {
	return &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{
		{Name: "things", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "kind", Type: "INTEGER"}, {Name: "added", Type: "TEXT", Nullable: true}}, PrimaryKey: []string{"id"},
			Indexes: []schema.Index{{Name: "things_added", Parts: []schema.IndexPart{{Column: "added"}}}}},
		{Name: "fresh", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}},
	}}
}

func TestAutoMigrateAdditive(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecDB(t, db, `CREATE TABLE things (id INTEGER PRIMARY KEY, kind TEXT, old_col TEXT)`)
	cd := migratetest.NewCountingDriver(db.Driver())
	e := migratetest.NewEngine(t, mpg.New(), cd, migratetest.EmptyFS())
	rep, err := e.AutoMigrate(ctx, autoDesired())
	if err != nil {
		t.Fatal(err)
	}
	if !tableExists(t, db, "fresh") || !columnExists(t, db, "things", "added") {
		t.Fatal("additive changes not applied")
	}
	if !columnExists(t, db, "things", "old_col") {
		t.Fatal("AutoMigrate must never drop a column")
	}
	if len(rep.Skipped) != 2 {
		t.Fatalf("type change and removed column must be skipped, got %+v", rep.Skipped)
	}
	reasons := map[string]string{}
	for _, s := range rep.Skipped {
		reasons[string(s.Change.Kind)+" "+s.Change.Name] = s.Reason
	}
	if reasons["alter_column kind"] != "type change requires a migration" ||
		reasons["drop_column old_col"] != "column removed in models; AutoMigrate never drops" {
		t.Fatalf("reasons: %+v", reasons)
	}
	kinds := map[migrate.ChangeKind]int{}
	for _, c := range rep.Applied {
		kinds[c.Kind]++
	}
	if kinds[migrate.CreateTable] != 1 || kinds[migrate.AddColumn] != 1 || kinds[migrate.CreateIndex] != 1 {
		t.Fatalf("applied: %+v", rep.Applied)
	}
	if got := firstValue(t, db, `SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'things' AND column_name = 'kind'`); got != "text" {
		t.Fatalf("column type must stay text, got %s", got)
	}
	// A second run finds nothing to apply and still reports the skips.
	before := cd.DDL()
	rep2, err := e.AutoMigrate(ctx, autoDesired())
	if err != nil || len(rep2.Applied) != 0 || len(rep2.Skipped) != 2 || cd.DDL() != before {
		t.Fatalf("second run: %+v %v ddl %d->%d", rep2, err, before, cd.DDL())
	}
	// AutoMigrate neither creates nor writes the history table.
	if got := firstValue(t, db, `SELECT (to_regclass('momo_migrations') IS NULL)::text`); got != "true" {
		t.Fatal("AutoMigrate must not create the history table")
	}
}

// TestAutoMigrateNeverDestroys: an unmanaged table, a NOT NULL column without
// default and a unique index on an existing table are skipped, never applied.
func TestAutoMigrateNeverDestroys(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecDB(t, db, `CREATE TABLE things (id INTEGER PRIMARY KEY, email TEXT)`)
	mustExecDB(t, db, `CREATE TABLE legacy (id INTEGER PRIMARY KEY)`)
	desired := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{{
		Name: "things", PrimaryKey: []string{"id"},
		Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "email", Type: "TEXT", Nullable: true}, {Name: "must", Type: "TEXT"}, {Name: "ok", Type: "TEXT", Default: "'x'"}},
		Indexes: []schema.Index{{Name: "things_email_uq", Unique: true, Parts: []schema.IndexPart{{Column: "email"}}}},
	}}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	rep, err := e.AutoMigrate(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if !tableExists(t, db, "legacy") || columnExists(t, db, "things", "must") || !columnExists(t, db, "things", "ok") {
		t.Fatal("wrong objects applied")
	}
	got := map[string]bool{}
	for _, s := range rep.Skipped {
		got[string(s.Change.Kind)+" "+s.Change.Name] = true
		if s.Reason == "" {
			t.Fatalf("skip without reason: %+v", s)
		}
	}
	for _, k := range []string{"drop_table legacy", "add_column must", "create_index things_email_uq"} {
		if !got[k] {
			t.Fatalf("%s not skipped: %+v", k, rep.Skipped)
		}
	}
	if len(rep.Applied) != 1 || rep.Applied[0].Name != "ok" {
		t.Fatalf("applied: %+v", rep.Applied)
	}
}

// TestAutoMigrateNewTableKeepsFKAndRLS: constraints that belong to a table
// created in this run are applied with it.
func TestAutoMigrateNewTableKeepsFKAndRLS(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{
		{Name: "owners", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}},
		{Name: "pets", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "owner_id", Type: "INTEGER"}}, PrimaryKey: []string{"id"},
			RLS:         &schema.RLS{Enabled: true},
			ForeignKeys: []schema.ForeignKey{{Name: "pets_owner", Columns: []string{"owner_id"}, RefTable: "owners", RefColumns: []string{"id"}}}},
	}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	rep, err := e.AutoMigrate(ctx, desired)
	if err != nil || len(rep.Skipped) != 0 {
		t.Fatalf("%+v %v", rep, err)
	}
	if got := firstValue(t, db, `SELECT relrowsecurity::text FROM pg_class WHERE oid = to_regclass('pets')`); got != "true" {
		t.Fatalf("RLS not enabled on new table: %s", got)
	}
}

// TestAutoMigrateConcurrent: several instances starting together converge on
// one result and none fails.
func TestAutoMigrateConcurrent(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	const n = 6
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e, err := migrate.NewEngine(db.Driver(), migratetest.EmptyFS(), migrate.WithDialect(mpg.New()), migrate.WithoutSum())
			if err == nil {
				_, err = e.AutoMigrate(ctx, autoDesired())
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !tableExists(t, db, "fresh") || !tableExists(t, db, "things") {
		t.Fatal("tables missing")
	}
}

func TestAutoMigrateNilDesired(t *testing.T) {
	db := testopen.Postgres(t)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	if _, err := e.AutoMigrate(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "desired") {
		t.Fatalf("err = %v", err)
	}
}
