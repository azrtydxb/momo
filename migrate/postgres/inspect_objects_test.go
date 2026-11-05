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
	got, err := inspectOnce(ctx, t, db, migrate.InspectOptions{})
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
	mustExec(t, d, `CREATE FUNCTION twice(x int) RETURNS int LANGUAGE sql IMMUTABLE STRICT SECURITY DEFINER AS $$ SELECT x * 2 $$`)
	mustExec(t, d, `CREATE FUNCTION touch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
	mustExec(t, d, `CREATE TRIGGER items_after AFTER DELETE OR TRUNCATE ON items FOR EACH STATEMENT EXECUTE FUNCTION touch()`)
	mustExec(t, d, `CREATE TRIGGER items_when BEFORE UPDATE ON items FOR EACH ROW WHEN (OLD.qty IS DISTINCT FROM NEW.qty) EXECUTE FUNCTION touch()`)
	mustExec(t, d, `CREATE TRIGGER items_lit BEFORE INSERT ON items FOR EACH ROW WHEN (NEW.owner <> 'x) EXECUTE FUNCTION y') EXECUTE FUNCTION touch()`)
	mustExec(t, d, `ALTER TABLE items ENABLE ROW LEVEL SECURITY`)
	mustExec(t, d, `ALTER TABLE items FORCE ROW LEVEL SECURITY`)
	mustExec(t, d, `CREATE POLICY p_all ON items AS RESTRICTIVE FOR SELECT TO PUBLIC USING (qty > 0)`)
	mustExec(t, d, `CREATE POLICY p_ins ON items FOR INSERT WITH CHECK (qty >= 0)`)
	got, err := inspectOnce(ctx, t, db, migrate.InspectOptions{})
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
	// The inspected query is pg_get_viewdef's rewritten text (search_path
	// dependent qualification, its own line breaks). Only whitespace and
	// letter case are normalized by schema.Equal; a declared view written in
	// another way compares Equal only after the baseline canonicalisation
	// step (momo-04 Task 18), which this inspection test does not cover.
	if got := schema.NormalizeExpr("postgres", views["big"].Query); got != "select id, qty from items where qty > 10" {
		t.Fatalf("inspected view query = %q", views["big"].Query)
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
	if !w.ForEachRow || w.When != "old.qty IS DISTINCT FROM new.qty" || w.Table != "items" {
		t.Fatalf("items_when: %+v", w)
	}
	// A WHEN literal that spells the EXECUTE FUNCTION keyword must not end the condition early.
	if l := trg["items_lit"]; l.When != "new.owner <> 'x) EXECUTE FUNCTION y'::text" {
		t.Fatalf("items_lit when = %q", l.When)
	}
	if !tw.Strict || !tw.SecurityDefiner || fns["touch"].Strict || fns["touch"].SecurityDefiner {
		t.Fatalf("strict/security definer: twice %+v touch %+v", tw, fns["touch"])
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
	_, err := inspectOnce(context.Background(), t, db, migrate.InspectOptions{})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if !strings.Contains(err.Error(), "t1_upd") {
		t.Fatalf("error must name the trigger: %v", err)
	}
}

func TestPostgresInspectPolicyPublicMixedWithRole(t *testing.T) {
	db := testopen.Postgres(t)
	d := db.Driver()
	const role = "momo_t12_policy_role"
	_, _ = d.Exec(context.Background(), `DROP ROLE IF EXISTS `+role, nil)
	if _, err := d.Exec(context.Background(), `CREATE ROLE `+role, nil); err != nil {
		t.Skipf("cannot create a role for this test user: %v", err)
	}
	mustExec(t, d, `CREATE TABLE docs (id bigint)`)
	mustExec(t, d, `ALTER TABLE docs ENABLE ROW LEVEL SECURITY`)
	mustExec(t, d, `CREATE POLICY mixed ON docs TO PUBLIC, `+role+` USING (true)`)
	t.Cleanup(func() {
		_, _ = d.Exec(context.Background(), `DROP POLICY IF EXISTS named ON docs`, nil)
		_, _ = d.Exec(context.Background(), `DROP ROLE IF EXISTS `+role, nil)
	})
	got, err := inspectOnce(context.Background(), t, db, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// PostgreSQL itself collapses "TO PUBLIC, role" to PUBLIC alone in
	// pg_policy.polroles ({0}), because PUBLIC already covers every role, so
	// the model's empty list (PUBLIC) is the faithful result. A named role
	// alone is kept (checked below).
	if len(got.Policies) != 1 || len(got.Policies[0].Roles) != 0 {
		t.Fatalf("PUBLIC mixed with a role is stored as PUBLIC alone: %+v", got.Policies)
	}
	mustExec(t, d, `DROP POLICY mixed ON docs`)
	mustExec(t, d, `CREATE POLICY named ON docs TO `+role+` USING (true)`)
	got, err = inspectOnce(context.Background(), t, db, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Policies) != 1 || !slices.Equal(got.Policies[0].Roles, []string{role}) {
		t.Fatalf("named role must be kept: %+v", got.Policies)
	}
}
