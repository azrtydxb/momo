// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// deferrableSnapshot is the schema of the deferrable tests as the JSON of a
// snapshot file, which is the format a user's migration directory holds. With
// deferred false every constraint is plain; with it true the stops unique is
// DEFERRABLE INITIALLY DEFERRED, the stops foreign key is DEFERRABLE and the
// labels primary key is DEFERRABLE INITIALLY DEFERRED.
func deferrableSnapshot(t *testing.T, deferred bool) *schema.Schema {
	t.Helper()
	var uq, fk, pk string
	if deferred {
		uq = `, "deferrable": true, "initially_deferred": true`
		fk = `, "deferrable": true`
		pk = `, "primary_key_deferrable": true, "primary_key_initially_deferred": true`
	}
	raw := fmt.Sprintf(`{
  "format_version": 1,
  "dialect": "postgres",
  "extensions": [{"name": "vector"}],
  "tables": [
    {"name": "routes", "columns": [{"name": "id", "type": "bigint"}], "primary_key": ["id"]},
    {"name": "stops",
     "columns": [{"name": "id", "type": "bigint"}, {"name": "route_id", "type": "bigint"}, {"name": "position", "type": "integer"}],
     "primary_key": ["id"],
     "uniques": [{"name": "stops_route_position_key", "columns": ["route_id", "position"]%s}],
     "foreign_keys": [{"name": "stops_route_id_fkey", "columns": ["route_id"], "ref_table": "routes", "ref_columns": ["id"]%s}]},
    {"name": "labels",
     "columns": [{"name": "id", "type": "bigint"}, {"name": "name", "type": "text"}],
     "primary_key": ["id"]%s}
  ]
}`, uq, fk, pk)
	var s schema.Schema
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	return &s
}

// constraintFlags reads condeferrable and condeferred of every constraint of
// deferrableSnapshot from the catalog of the public schema.
func constraintFlags(t *testing.T, db *momo.DB) map[string][2]bool {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(), `SELECT c.conname::text, c.condeferrable, c.condeferred
FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
WHERE n.nspname = 'public' AND c.contype IN ('p', 'u', 'f')`, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string][2]bool{}
	for rows.Next() {
		var name string
		var flags [2]bool
		if err := rows.Scan(&name, &flags[0], &flags[1]); err != nil {
			t.Fatal(err)
		}
		got[name] = flags
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

// requireFlags fails t unless the catalog holds the deferrability of
// deferrableSnapshot(deferred).
func requireFlags(t *testing.T, db *momo.DB, deferred bool) {
	t.Helper()
	want := map[string][2]bool{
		"routes_pkey":              {false, false},
		"stops_pkey":               {false, false},
		"stops_route_position_key": {false, false},
		"stops_route_id_fkey":      {false, false},
		"labels_pkey":              {false, false},
	}
	if deferred {
		want["stops_route_position_key"] = [2]bool{true, true}
		want["stops_route_id_fkey"] = [2]bool{true, false}
		want["labels_pkey"] = [2]bool{true, true}
	}
	got := constraintFlags(t, db)
	for name, w := range want {
		if got[name] != w {
			t.Errorf("constraint %s: (deferrable, initially deferred) = %v, want %v", name, got[name], w)
		}
	}
}

// Catches: DEFERRABLE and INITIALLY DEFERRED lost in CREATE TABLE, or in
// inspection, so a database built from the model differs from it: the
// transaction that reorders rows under a deferred unique fails with a unique
// violation, and the inspection of the table reads every constraint as plain.
func TestPostgresDeferrableRoundTrip(t *testing.T) {
	db := concurrentIndexDB(t)
	desired := deferrableSnapshot(t, true)
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{}))
	requireFlags(t, db, true)

	got := inspectIn(t, db, concurrentIndexSchema)
	if !schema.Equal(got, desired) {
		t.Fatalf("round trip has a diff:\n%s", jsonDiff(got, desired))
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"initially_deferred":true`, `"deferrable":true`, `"primary_key_initially_deferred":true`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the inspection lacks %s:\n%s", want, raw)
		}
	}
	if again, _, err := migrate.Diff(got, desired, migrate.DiffOptions{}); err != nil || len(again) != 0 {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}

	// A deferred unique lets one statement swap two positions.
	applyStatements(t, db, []migrate.Statement{
		{SQL: `INSERT INTO routes VALUES (1)`},
		{SQL: `INSERT INTO stops VALUES (1, 1, 1), (2, 1, 2)`},
		{SQL: `UPDATE stops SET position = 3 - position WHERE route_id = 1`},
	})
}

// Catches: a change of deferrability that diff does not see (the migration is
// empty and the database stays as it was), a plan that does not carry the new
// clause, and a change back to NOT DEFERRABLE that leaves the constraint
// deferrable.
func TestPostgresDeferrableChangeDiff(t *testing.T) {
	db := concurrentIndexDB(t)
	plain, deferred := deferrableSnapshot(t, false), deferrableSnapshot(t, true)
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, plain, migrate.DiffOptions{}))
	requireFlags(t, db, false)

	forward := planStatements(t, plain, deferred, migrate.DiffOptions{})
	if len(forward) == 0 {
		t.Fatal("a change of deferrability produced no statement")
	}
	var script []string
	for _, s := range forward {
		script = append(script, s.SQL)
	}
	sql := strings.Join(script, "\n")
	for _, want := range []string{"DEFERRABLE INITIALLY DEFERRED", `"stops_route_position_key"`, `"stops_route_id_fkey"`, `"labels_pkey"`} {
		if !strings.Contains(sql, want) {
			t.Errorf("the plan lacks %s:\n%s", want, sql)
		}
	}
	applyStatements(t, db, forward)
	requireFlags(t, db, true)
	if got := inspectIn(t, db, concurrentIndexSchema); !schema.Equal(got, deferred) {
		t.Fatalf("after the change the database differs from the model:\n%s", jsonDiff(got, deferred))
	}

	back := planStatements(t, deferred, plain, migrate.DiffOptions{})
	if len(back) == 0 {
		t.Fatal("a change back to NOT DEFERRABLE produced no statement")
	}
	applyStatements(t, db, back)
	requireFlags(t, db, false)
}

// Catches: a foreign key that references a deferrable primary key or unique
// constraint passing the planner. PostgreSQL refuses to create such a foreign
// key ("cannot use a deferrable unique constraint for referenced table"), so
// the plan would fail half way through its migration; Diff has to refuse the
// schema before it plans anything. The first half of each case proves the
// refusal is justified: the real database rejects the DDL.
func TestPostgresForeignKeyToDeferrableKeyIsRefused(t *testing.T) {
	cases := []struct {
		name string
		ddl  []string // the last statement is the one PostgreSQL rejects
		keys string   // the routes table of the desired schema, after its columns
	}{
		{
			name: "primary key",
			ddl: []string{
				`CREATE TABLE routes (id bigint NOT NULL, code text NOT NULL, CONSTRAINT routes_pkey PRIMARY KEY (id) DEFERRABLE)`,
				`CREATE TABLE stops (id bigint PRIMARY KEY, route_id bigint NOT NULL, CONSTRAINT stops_route_id_fkey FOREIGN KEY (route_id) REFERENCES routes (id))`,
			},
			keys: `"primary_key": ["id"], "primary_key_deferrable": true`,
		},
		{
			name: "unique",
			ddl: []string{
				`CREATE TABLE routes (id bigint PRIMARY KEY, code text NOT NULL, CONSTRAINT routes_code_key UNIQUE (code) DEFERRABLE)`,
				`CREATE TABLE stops (id bigint PRIMARY KEY, route_id bigint NOT NULL, CONSTRAINT stops_route_id_fkey FOREIGN KEY (route_id) REFERENCES routes (code))`,
			},
			keys: `"primary_key": ["id"], "uniques": [{"name": "routes_code_key", "columns": ["code"], "deferrable": true}]`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := concurrentIndexDB(t)
			var err error
			for _, stmt := range c.ddl {
				_, err = db.Driver().Exec(context.Background(), stmt, nil)
			}
			if err == nil || !strings.Contains(err.Error(), "cannot use a deferrable") {
				t.Fatalf("PostgreSQL accepted a foreign key to a deferrable key, or failed for another reason: %v", err)
			}

			refCol := "id"
			if c.name == "unique" {
				refCol = "code"
			}
			raw := fmt.Sprintf(`{
  "format_version": 1,
  "dialect": "postgres",
  "tables": [
    {"name": "routes", "columns": [{"name": "id", "type": "bigint"}, {"name": "code", "type": "text"}], %s},
    {"name": "stops",
     "columns": [{"name": "id", "type": "bigint"}, {"name": "route_id", "type": "bigint"}],
     "primary_key": ["id"],
     "foreign_keys": [{"name": "stops_route_id_fkey", "columns": ["route_id"], "ref_table": "routes", "ref_columns": [%q]}]}
  ]
}`, c.keys, refCol)
			var desired schema.Schema
			if err := json.Unmarshal([]byte(raw), &desired); err != nil {
				t.Fatal(err)
			}
			changes, _, err := migrate.Diff(&schema.Schema{Dialect: "postgres"}, &desired, migrate.DiffOptions{})
			if err == nil || !strings.Contains(err.Error(), "deferrable") {
				t.Errorf("Diff accepted a schema that PostgreSQL rejects: err = %v", err)
			}
			if len(changes) != 0 {
				t.Errorf("a refused schema produced %d changes", len(changes))
			}
		})
	}
}
