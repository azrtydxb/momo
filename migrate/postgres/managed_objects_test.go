// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	"github.com/azrtydxb/momo/schema"
)

// managedObjects returns a range-partitioned table with one child partition, a
// plpgsql trigger function and a row trigger on the parent that calls it.
func managedObjects(body string) *schema.Schema {
	cols := []schema.Column{
		{Name: "id", Type: "bigint"},
		{Name: "tenant", Type: "text"},
		{Name: "created_at", Type: "date"},
	}
	return &schema.Schema{
		Dialect: "postgres",
		Tables: []schema.Table{
			{
				Name:       "events",
				Columns:    cols,
				PrimaryKey: []string{"id", "created_at"},
				Partition:  &schema.Partition{Strategy: "range", Key: "RANGE (created_at)"},
			},
			{
				Name:        "events_2026",
				Columns:     cols,
				PartitionOf: &schema.PartitionOf{Parent: "events", Bound: "FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')"},
			},
		},
		Functions: []schema.Function{{
			Name: "touch", Args: "", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: body,
		}},
		Triggers: []schema.Trigger{{
			Table: "events", Name: "events_touch", Timing: "BEFORE", Events: []string{"INSERT", "UPDATE"},
			ForEachRow: true, Function: "touch()",
		}},
	}
}

// TestManagedFunctionsTriggersPartitions applies a partitioned table, a
// function and a trigger through the engine, proves they work in the live
// database, reads them back with no diff (so none of them is unmanaged: an
// object the model does not declare, or one it declares and the database lacks,
// would make schema.Equal and the second diff fail) and proves that a body
// change renders CREATE OR REPLACE FUNCTION and applies through the engine.
func TestManagedFunctionsTriggersPartitions(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := managedObjects("BEGIN NEW.tenant := lower(NEW.tenant); RETURN NEW; END;")
	empty := &schema.Schema{Dialect: "postgres"}
	files := migrationFiles(planStatements(t, empty, desired, migrate.DiffOptions{}), 0)
	if _, err := engineOn(t, db, migratetest.FS(files)).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	// The trigger fires through the partition parent and the child holds the row.
	mustExec(t, db.Driver(), `INSERT INTO events (id, tenant, created_at) VALUES (1, 'ACME', '2026-03-01')`)
	var tenant string
	rows, err := db.Driver().Query(ctx, `SELECT tenant FROM events_2026 WHERE id = 1`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("row missing from the partition")
	}
	if err := rows.Scan(&tenant); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if tenant != "acme" {
		t.Fatalf("trigger function did not run, tenant = %q", tenant)
	}

	// Everything reads back: no diff, no extra and no missing object.
	got := inspectNow(t, db)
	desired.Extensions = got.Extensions // database-wide; testdb installs vector
	if !schema.Equal(got, desired) {
		t.Fatalf("read-back has a diff:\n%s", migratetest.Diff(got, desired))
	}
	if again, _, err := migrate.Diff(got, desired, migrate.DiffOptions{}); err != nil || len(again) != 0 {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}
	if len(got.Functions) != 1 || got.Functions[0].Name != "touch" || len(got.Triggers) != 1 || got.Triggers[0].Name != "events_touch" || len(got.Tables) != 2 {
		t.Fatalf("function, trigger and tables must all be read back as managed objects: %d functions, %d triggers, %d tables", len(got.Functions), len(got.Triggers), len(got.Tables))
	}
	for _, name := range []string{"events", "events_2026"} {
		g, ok := got.Table(name)
		w, _ := desired.Table(name)
		if !ok || (g.Partition == nil) != (w.Partition == nil) || (g.PartitionOf == nil) != (w.PartitionOf == nil) {
			t.Fatalf("partition table %s did not round trip: %+v", name, g)
		}
	}

	// Body change: the function is replaced, not dropped and recreated.
	changed := managedObjects("BEGIN NEW.tenant := upper(NEW.tenant); RETURN NEW; END;")
	stmts := planStatements(t, desired, changed, migrate.DiffOptions{})
	var up strings.Builder
	for _, st := range stmts {
		up.WriteString(st.SQL)
		up.WriteString(";\n")
	}
	if !strings.Contains(up.String(), "CREATE OR REPLACE FUNCTION") || strings.Contains(up.String(), "DROP FUNCTION") {
		t.Fatalf("body change must render CREATE OR REPLACE FUNCTION only, got:\n%s", up.String())
	}
	second := migrationFiles(stmts, len(files))
	both := map[string][]byte{}
	for k, v := range files {
		both[k] = v
	}
	for k, v := range second {
		both[k] = v
	}
	if _, err := engineOn(t, db, migratetest.FS(both)).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db.Driver(), `INSERT INTO events (id, tenant, created_at) VALUES (2, 'acme', '2026-03-02')`)
	var upper string
	r2, err := db.Driver().Query(ctx, `SELECT tenant FROM events_2026 WHERE id = 2`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Next() {
		t.Fatal("row 2 missing")
	}
	if err := r2.Scan(&upper); err != nil {
		t.Fatal(err)
	}
	_ = r2.Close()
	if upper != "ACME" {
		t.Fatalf("the replaced function body did not take effect, tenant = %q", upper)
	}
	changed.Extensions = got.Extensions
	if after := inspectNow(t, db); !schema.Equal(after, changed) {
		t.Fatalf("read-back after the body change has a diff:\n%s", migratetest.Diff(after, changed))
	}
}

func flagSchema(strict, definer bool) *schema.Schema {
	return &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: "postgres", Tables: []schema.Table{}, Functions: []schema.Function{{
		Name: "twice", Args: "x integer", Returns: "integer", Language: "sql", Volatility: "IMMUTABLE",
		Strict: strict, SecurityDefiner: definer, Body: "SELECT x * 2",
	}}}
}

func withExt(s, from *schema.Schema) *schema.Schema {
	s.Extensions = from.Extensions
	return s
}

// TestFunctionStrictSecurityDefinerRoundTrip proves the flags survive render,
// apply and inspect, that a second diff is empty, and that flipping
// SecurityDefiner renders a CREATE OR REPLACE the server accepts.
func TestFunctionStrictSecurityDefinerRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	desired := flagSchema(true, true)
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{}))

	got := inspectNow(t, db)
	if len(got.Functions) != 1 || !got.Functions[0].Strict || !got.Functions[0].SecurityDefiner {
		t.Fatalf("flags not inspected: %+v", got.Functions)
	}
	desired.Extensions = got.Extensions // database-wide; testdb installs vector
	if !schema.Equal(got, desired) {
		t.Fatalf("round trip differs:\n%s", jsonDiff(got, desired))
	}
	if again := planStatements(t, got, desired, migrate.DiffOptions{}); len(again) != 0 {
		t.Fatalf("second diff not empty: %+v", again)
	}

	flipped := flagSchema(true, false)
	flipped.Extensions = got.Extensions
	stmts := planStatements(t, got, flipped, migrate.DiffOptions{})
	if len(stmts) != 1 || !strings.HasPrefix(stmts[0].SQL, "CREATE OR REPLACE FUNCTION") || strings.Contains(stmts[0].SQL, "SECURITY") {
		t.Fatalf("flip must be one CREATE OR REPLACE without SECURITY DEFINER: %+v", stmts)
	}
	applyStatements(t, db, stmts)
	after := inspectNow(t, db)
	if after.Functions[0].SecurityDefiner || !after.Functions[0].Strict || !schema.Equal(after, flipped) {
		t.Fatalf("flip not applied: %+v", after.Functions)
	}

	// Strict flip alone is also a function change.
	stmts = planStatements(t, after, withExt(flagSchema(false, false), after), migrate.DiffOptions{})
	if len(stmts) != 1 || !strings.HasPrefix(stmts[0].SQL, "CREATE OR REPLACE FUNCTION") {
		t.Fatalf("strict flip: %+v", stmts)
	}
	applyStatements(t, db, stmts)
}
