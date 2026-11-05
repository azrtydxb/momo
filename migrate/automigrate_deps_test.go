// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"strings"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func skippedReasons(rep *migrate.AutoReport) map[string]string {
	m := map[string]string{}
	for _, s := range rep.Skipped {
		m[string(s.Change.Kind)+" "+s.Change.Name] = s.Reason
	}
	return m
}

// TestAutoMigrateSkipsDependentOfSkipped: an index on a column whose add was
// skipped is skipped too, instead of failing the whole run.
func TestAutoMigrateSkipsDependentOfSkipped(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, pinned(t, db), `CREATE TABLE things (id INTEGER PRIMARY KEY)`)
	desired := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{{
		Name: "things", PrimaryKey: []string{"id"},
		Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "must", Type: "TEXT"}, {Name: "fine", Type: "TEXT", Nullable: true}},
		Indexes: []schema.Index{
			{Name: "things_must", Parts: []schema.IndexPart{{Column: "must"}}},
			{Name: "things_fine", Parts: []schema.IndexPart{{Column: "fine"}}},
		},
	}}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	rep, err := e.AutoMigrate(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if columnExists(t, db, "things", "must") || !columnExists(t, db, "things", "fine") {
		t.Fatal("wrong columns applied")
	}
	r := skippedReasons(rep)
	if !strings.HasPrefix(r["create_index things_must"], "depends on a skipped change: add_column") {
		t.Fatalf("reasons: %+v", r)
	}
	if _, ok := r["create_index things_fine"]; ok {
		t.Fatalf("independent index must be kept: %+v", r)
	}
	if got := firstValue(t, db, `SELECT count(*)::text FROM pg_indexes WHERE tablename = 'things' AND indexname = 'things_fine'`); got != "1" {
		t.Fatal("things_fine not created")
	}
}

// TestAutoMigrateSkipsIndexOnSkippedTypeChange: an index in the desired schema
// assumes the new column type; with the type change skipped it is skipped.
func TestAutoMigrateSkipsIndexOnSkippedTypeChange(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, pinned(t, db), `CREATE TABLE things (id INTEGER PRIMARY KEY, kind TEXT)`)
	desired := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{{
		Name: "things", PrimaryKey: []string{"id"},
		Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "kind", Type: "INTEGER", Nullable: true}},
		Indexes: []schema.Index{{Name: "things_kind", Parts: []schema.IndexPart{{Column: "kind"}}}},
	}}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	rep, err := e.AutoMigrate(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	r := skippedReasons(rep)
	if !strings.HasPrefix(r["create_index things_kind"], "depends on a skipped change: alter_column") || len(rep.Applied) != 0 {
		t.Fatalf("reasons %+v applied %+v", r, rep.Applied)
	}
}

// TestAutoMigrateSkipsRewritingAddColumn: stored generated columns, volatile
// defaults and the indexes on them are skipped; constants and now() apply.
func TestAutoMigrateSkipsRewritingAddColumn(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, pinned(t, db), `CREATE TABLE things (id INTEGER PRIMARY KEY)`)
	desired := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{{
		Name: "things", PrimaryKey: []string{"id"},
		Columns: []schema.Column{
			{Name: "id", Type: "INTEGER"},
			{Name: "gen", Type: "INTEGER", Generated: "id * 2", Nullable: true},
			{Name: "rnd", Type: "DOUBLE PRECISION", Default: "random()"},
			{Name: "uid", Type: "UUID", Default: "gen_random_uuid()"},
			{Name: "lit", Type: "TEXT", Default: "'x'"},
			{Name: "num", Type: "INTEGER", Default: "0"},
			{Name: "ts", Type: "TIMESTAMPTZ", Default: "now()"},
		},
		Indexes: []schema.Index{{Name: "things_rnd", Parts: []schema.IndexPart{{Column: "rnd"}}}},
	}}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	rep, err := e.AutoMigrate(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"gen", "rnd", "uid"} {
		if columnExists(t, db, "things", c) {
			t.Fatalf("%s must be skipped", c)
		}
	}
	for _, c := range []string{"lit", "num", "ts"} {
		if !columnExists(t, db, "things", c) {
			t.Fatalf("%s must be applied", c)
		}
	}
	r := skippedReasons(rep)
	for _, k := range []string{"add_column gen", "add_column rnd", "add_column uid"} {
		if r[k] == "" {
			t.Fatalf("%s not skipped: %+v", k, r)
		}
	}
	if !strings.HasPrefix(r["create_index things_rnd"], "depends on a skipped change") {
		t.Fatalf("reasons: %+v", r)
	}
}
