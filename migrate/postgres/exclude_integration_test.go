// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"slices"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Catches: an excluded (or introspect.skip) table still being read, so a
// column the schema model rejects (a VIRTUAL generated column, PostgreSQL 18)
// fails the whole inspection instead of just that table.
func TestPostgresInspectExcludedTableIsNotRead(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := db.Driver().Exec(ctx, `CREATE TABLE legacy_virtual (a int, b int GENERATED ALWAYS AS (a * 2) VIRTUAL)`, nil); err != nil {
		t.Skipf("virtual generated columns need PostgreSQL 18: %v", err)
	}
	mustExec(t, db.Driver(), `CREATE TABLE kept (id bigint)`)
	if _, err := inspectOnce(ctx, t, db, migrate.InspectOptions{}); err == nil {
		t.Fatal("without an exclude, the virtual generated column must be refused")
	}
	got, err := inspectOnce(ctx, t, db, migrate.InspectOptions{Exclude: []string{"legacy_*"}})
	if err != nil {
		t.Fatalf("an excluded table must not fail the inspection: %v", err)
	}
	for _, tb := range got.Tables {
		if tb.Name == "legacy_virtual" {
			t.Fatal("the excluded table is still in the result")
		}
	}
	if _, ok := got.Table("kept"); !ok {
		t.Fatal("the table that is not excluded must be read")
	}
}

// Catches: an excluded view or function being read.
func TestPostgresInspectExcludedViewAndFunction(t *testing.T) {
	db := testopen.Postgres(t)
	mustExec(t, db.Driver(), `CREATE TABLE t1 (id bigint)`)
	mustExec(t, db.Driver(), `CREATE VIEW skip_v AS SELECT id FROM t1`)
	mustExec(t, db.Driver(), `CREATE VIEW keep_v AS SELECT id FROM t1`)
	mustExec(t, db.Driver(), `CREATE FUNCTION skip_f() RETURNS int LANGUAGE sql AS $$ SELECT 1 $$`)
	got, err := inspectOnce(context.Background(), t, db, migrate.InspectOptions{Exclude: []string{"skip_*"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Views) != 1 || got.Views[0].Name != "keep_v" || len(got.Functions) != 0 {
		t.Fatalf("views %+v functions %+v", got.Views, got.Functions)
	}
}

// Catches: the UPDATE OF list being returned in attribute-number order instead
// of the order written, which would show as drift for UPDATE OF b, a.
func TestPostgresTriggerUpdateOfKeepsWrittenOrder(t *testing.T) {
	db := testopen.Postgres(t)
	mustExec(t, db.Driver(), `CREATE TABLE ord (a int, b int)`)
	mustExec(t, db.Driver(), `CREATE FUNCTION ord_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
	mustExec(t, db.Driver(), `CREATE TRIGGER ord_upd AFTER UPDATE OF b, a ON ord FOR EACH ROW EXECUTE FUNCTION ord_fn()`)
	got, err := inspectOnce(context.Background(), t, db, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Triggers) != 1 || !slices.Equal(got.Triggers[0].UpdateOf, []string{"b", "a"}) {
		t.Fatalf("triggers: %+v", got.Triggers)
	}
}

// Catches: UPDATE OF b, a being rendered or read back as a, b, which would make
// every later diff recreate the trigger.
func TestPostgresTriggerUpdateOfOrderRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	roundTrip(t, db, &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}}, // installed in the test database; inspection reports every extension
		Tables: []schema.Table{{
			Name:    "ord",
			Columns: []schema.Column{{Name: "a", Type: "integer"}, {Name: "b", Type: "integer"}},
		}},
		Functions: []schema.Function{{Name: "ord_fn", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: "BEGIN RETURN NEW; END;"}},
		Triggers: []schema.Trigger{{
			Table: "ord", Name: "ord_upd", Timing: "AFTER", Events: []string{"UPDATE"},
			UpdateOf: []string{"b", "a"}, ForEachRow: true, Function: "ord_fn()",
		}},
	})
}
