// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func TestPostgresObjectInspection(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	d := db.Driver()
	mustExec(t, d, `CREATE TABLE events (id bigint, created_at timestamptz NOT NULL, tenant text NOT NULL, PRIMARY KEY (id, created_at)) PARTITION BY RANGE (created_at)`)
	mustExec(t, d, `CREATE TABLE events_2026 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')`)
	mustExec(t, d, `CREATE FUNCTION touch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
	mustExec(t, d, `CREATE TRIGGER events_touch BEFORE INSERT OR UPDATE ON events FOR EACH ROW EXECUTE FUNCTION touch()`)
	mustExec(t, d, `ALTER TABLE events ENABLE ROW LEVEL SECURITY`)
	mustExec(t, d, `CREATE POLICY tenant_only ON events USING (tenant = current_setting('app.tenant', true))`)
	mustExec(t, d, `CREATE VIEW recent AS SELECT id FROM events`)
	got, err := mpg.New().Inspect(ctx, pinned(t, db), migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := got.Table("events")
	if !ok {
		t.Fatalf("table %q missing", "events")
	}
	if ev.Partition == nil || ev.Partition.Strategy != "range" {
		t.Fatalf("partition not inspected: %+v", ev.Partition)
	}
	if ev.RLS == nil || !ev.RLS.Enabled {
		t.Fatal("RLS flag not inspected")
	}
	child, ok := got.Table("events_2026")
	if !ok || child.PartitionOf == nil || child.PartitionOf.Parent != "events" {
		t.Fatalf("partition child must appear as a table carrying PartitionOf: %+v", child)
	}
	if len(child.PrimaryKey) != 0 {
		t.Fatalf("constraints inherited from the parent must be hidden on the child: %+v", child.PrimaryKey)
	}
	if len(got.Triggers) != 1 || got.Triggers[0].Timing != "BEFORE" || len(got.Triggers[0].Events) != 2 {
		t.Fatalf("trigger: %+v", got.Triggers)
	}
	if len(got.Functions) != 1 || got.Functions[0].Language != "plpgsql" {
		t.Fatalf("function: %+v", got.Functions)
	}
	if len(got.Policies) != 1 || got.Policies[0].Command != "ALL" {
		t.Fatalf("policy: %+v", got.Policies)
	}
	if len(got.Views) != 1 || got.Views[0].Name != "recent" {
		t.Fatalf("view: %+v", got.Views)
	}
}

func TestPostgresObjectInspectionDetails(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	d := db.Driver()
	mustExec(t, d, `CREATE TABLE items (id bigint PRIMARY KEY, owner text NOT NULL, qty int NOT NULL)`)
	mustExec(t, d, `CREATE VIEW big AS SELECT id, qty FROM items WHERE qty > 10`)
	mustExec(t, d, `CREATE VIEW bigger AS SELECT id FROM big WHERE qty > 100`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW totals AS SELECT owner, sum(qty) AS total FROM items GROUP BY owner`)
	mustExec(t, d, `CREATE UNIQUE INDEX totals_owner ON totals (owner)`)
	mustExec(t, d, `CREATE FUNCTION twice(x int) RETURNS int LANGUAGE sql IMMUTABLE AS $$ SELECT x * 2 $$`)
	mustExec(t, d, `CREATE FUNCTION touch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
	mustExec(t, d, `CREATE TRIGGER items_after AFTER DELETE OR TRUNCATE ON items FOR EACH STATEMENT EXECUTE FUNCTION touch()`)
	mustExec(t, d, `CREATE TRIGGER items_when BEFORE UPDATE ON items FOR EACH ROW WHEN (OLD.qty IS DISTINCT FROM NEW.qty) EXECUTE FUNCTION touch()`)
	mustExec(t, d, `ALTER TABLE items ENABLE ROW LEVEL SECURITY`)
	mustExec(t, d, `ALTER TABLE items FORCE ROW LEVEL SECURITY`)
	mustExec(t, d, `CREATE POLICY p_all ON items AS RESTRICTIVE FOR SELECT TO PUBLIC USING (qty > 0)`)
	mustExec(t, d, `CREATE POLICY p_ins ON items FOR INSERT WITH CHECK (qty >= 0)`)
	got, err := mpg.New().Inspect(ctx, pinned(t, db), migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	items, _ := got.Table("items")
	if items.RLS == nil || !items.RLS.Enabled || !items.RLS.Forced {
		t.Fatalf("rls: %+v", items.RLS)
	}
	views := map[string]schema.View{}
	for _, v := range got.Views {
		views[v.Name] = v
	}
	if len(views) != 3 {
		t.Fatalf("views: %+v", got.Views)
	}
	if !slices.Equal(views["bigger"].Depends, []string{"big"}) {
		t.Fatalf("bigger depends: %v", views["bigger"].Depends)
	}
	if !slices.Equal(views["big"].Depends, []string{"items"}) {
		t.Fatalf("big depends: %v", views["big"].Depends)
	}
	tot := views["totals"]
	if !tot.Materialized || !slices.Equal(tot.Depends, []string{"items"}) || len(tot.Indexes) != 1 || tot.Indexes[0].Name != "totals_owner" {
		t.Fatalf("matview: %+v", tot)
	}
	// A model-declared view compares Equal to the inspected one.
	declared := *got
	declared.Views = slices.Clone(got.Views)
	for i := range declared.Views {
		if declared.Views[i].Name == "big" {
			declared.Views[i].Query = "SELECT id,   qty\n FROM items WHERE qty > 10"
		}
	}
	if !schema.Equal(got, &declared) {
		t.Fatalf("respelled view query must compare Equal: inspected %q", views["big"].Query)
	}
	fns := map[string]schema.Function{}
	for _, f := range got.Functions {
		fns[f.Name] = f
	}
	tw := fns["twice"]
	if tw.Volatility != "IMMUTABLE" || tw.Language != "sql" || tw.Args != "x integer" || tw.Returns != "integer" {
		t.Fatalf("function: %+v", tw)
	}
	trg := map[string]schema.Trigger{}
	for _, tr := range got.Triggers {
		trg[tr.Name] = tr
	}
	a := trg["items_after"]
	if a.Timing != "AFTER" || a.ForEachRow || !slices.Equal(a.Events, []string{"DELETE", "TRUNCATE"}) || a.Function != "touch()" {
		t.Fatalf("items_after: %+v", a)
	}
	w := trg["items_when"]
	if !w.ForEachRow || w.When == "" || w.Table != "items" {
		t.Fatalf("items_when: %+v", w)
	}
	pol := map[string]schema.Policy{}
	for _, p := range got.Policies {
		pol[p.Name] = p
	}
	if p := pol["p_all"]; p.Command != "SELECT" || p.Permissive || len(p.Roles) != 0 || p.Using == "" {
		t.Fatalf("p_all: %+v", p)
	}
	if p := pol["p_ins"]; p.Command != "INSERT" || !p.Permissive || p.Check == "" || p.Using != "" {
		t.Fatalf("p_ins: %+v", p)
	}
}

func TestPostgresInspectTriggerUpdateOfUnsupported(t *testing.T) {
	db := testopen.Postgres(t)
	d := db.Driver()
	mustExec(t, d, `CREATE TABLE t1 (a int, b int)`)
	mustExec(t, d, `CREATE FUNCTION touch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
	mustExec(t, d, `CREATE TRIGGER t1_upd BEFORE UPDATE OF a ON t1 FOR EACH ROW EXECUTE FUNCTION touch()`)
	_, err := mpg.New().Inspect(context.Background(), pinned(t, db), migrate.InspectOptions{})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if !strings.Contains(err.Error(), "t1_upd") {
		t.Fatalf("error must name the trigger: %v", err)
	}
}
