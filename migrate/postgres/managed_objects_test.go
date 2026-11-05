// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// managedObjects returns a range-partitioned table with one child partition, a
// plpgsql trigger function and a row trigger on the parent that calls it.
func managedObjects(body string) *schema.Schema {
	cols := []schema.Column{
		{Name: "id", Type: "bigint"},
		{Name: "tenant", Type: "text"},
		{Name: "created_at", Type: "timestamp with time zone"},
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

// TestManagedFunctionsTriggersPartitions renders and applies a partitioned
// table, a function and a trigger, proves they work in the live database, and
// proves that a body change renders CREATE OR REPLACE FUNCTION. The inspection
// half (function and trigger read back, no diff, not unmanaged) needs story
// 081 and is asserted here only for the partitioned tables.
func TestManagedFunctionsTriggersPartitions(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := managedObjects("BEGIN NEW.tenant := lower(NEW.tenant); RETURN NEW; END;")
	empty := &schema.Schema{Dialect: "postgres"}
	applyStatements(t, db, planStatements(t, empty, desired, migrate.DiffOptions{}))

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

	// The partitioned tables inspect back with their partition metadata.
	got := inspectNow(t, db)
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
	applyStatements(t, db, stmts)
	mustExec(t, db.Driver(), `INSERT INTO events (id, tenant, created_at) VALUES (2, 'acme', '2026-03-02')`)
}
