// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// deferrableTables is routes plus the stops table with a deferrable unique, a
// deferrable foreign key and an initially deferred primary key.
func deferrableTables(deferred bool) *schema.Schema {
	stops := schema.Table{
		Name: "stops",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "route_id", Type: "bigint"},
			{Name: "position", Type: "integer"},
		},
		PrimaryKey: []string{"id"},
		Uniques:    []schema.Unique{{Name: "stops_route_position_key", Columns: []string{"route_id", "position"}}},
		ForeignKeys: []schema.ForeignKey{
			{Name: "stops_route_id_fkey", Columns: []string{"route_id"}, RefTable: "routes", RefColumns: []string{"id"}},
		},
	}
	if deferred {
		stops.PrimaryKeyDeferrable, stops.PrimaryKeyInitiallyDeferred = true, true
		stops.Uniques[0].Deferrable, stops.Uniques[0].InitiallyDeferred = true, true
		stops.ForeignKeys[0].Deferrable = true
	}
	routes := schema.Table{Name: "routes", Columns: []schema.Column{{Name: "id", Type: "bigint"}}, PrimaryKey: []string{"id"}}
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{routes, stops}}
}

func renderDiff(t *testing.T, from, to *schema.Schema) string {
	t.Helper()
	changes, _, err := migrate.Diff(from, to, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.OrderWithViews(changes, from.Views, to.Views)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, s := range renderSQL(t, ordered...) {
		sql = append(sql, s.SQL)
	}
	return strings.Join(sql, "\n")
}

// Catches: CREATE TABLE and the later ALTERs leaving DEFERRABLE out, or
// writing it in an order PostgreSQL rejects, or adding it to a constraint that
// is not deferrable.
func TestRenderDeferrableConstraints(t *testing.T) {
	create := renderDiff(t, &schema.Schema{Dialect: "postgres"}, deferrableTables(true))
	for _, want := range []string{
		`CONSTRAINT "stops_pkey" PRIMARY KEY ("id") DEFERRABLE INITIALLY DEFERRED`,
		`CONSTRAINT "stops_route_position_key" UNIQUE ("route_id", "position") DEFERRABLE INITIALLY DEFERRED`,
		`FOREIGN KEY ("route_id") REFERENCES "routes" ("id") DEFERRABLE`,
	} {
		if !strings.Contains(create, want) {
			t.Errorf("CREATE TABLE lacks %s:\n%s", want, create)
		}
	}
	if n := strings.Count(create, "DEFERRABLE"); n != 3 {
		t.Errorf("%d constraints are deferrable, want 3 (the routes key is not):\n%s", n, create)
	}
	plain := renderDiff(t, &schema.Schema{Dialect: "postgres"}, deferrableTables(false))
	if strings.Contains(plain, "DEFERRABLE") {
		t.Errorf("plain constraints render a deferrability clause:\n%s", plain)
	}
}

// Catches: a change of deferrability that diff drops (no statement), one that
// changes only half of the constraint (the drop without the add), and one that
// adds the constraint back without the clause.
func TestRenderDeferrabilityChange(t *testing.T) {
	forward := renderDiff(t, deferrableTables(false), deferrableTables(true))
	for _, want := range []string{
		`DROP CONSTRAINT "stops_pkey"`,
		`ADD CONSTRAINT "stops_pkey" PRIMARY KEY ("id") DEFERRABLE INITIALLY DEFERRED`,
		`DROP CONSTRAINT "stops_route_position_key"`,
		`UNIQUE USING INDEX "stops_route_position_key" DEFERRABLE INITIALLY DEFERRED`,
		`DROP CONSTRAINT "stops_route_id_fkey"`,
		`FOREIGN KEY ("route_id") REFERENCES "routes" ("id") DEFERRABLE`,
	} {
		if !strings.Contains(forward, want) {
			t.Errorf("the change to deferrable lacks %s:\n%s", want, forward)
		}
	}
	back := renderDiff(t, deferrableTables(true), deferrableTables(false))
	if !strings.Contains(back, `DROP CONSTRAINT "stops_route_position_key"`) || strings.Contains(back, "DEFERRABLE") {
		t.Errorf("the change back to NOT DEFERRABLE:\n%s", back)
	}
	if same := renderDiff(t, deferrableTables(true), deferrableTables(true)); same != "" {
		t.Errorf("an unchanged deferrability renders:\n%s", same)
	}
}

// Catches: DEFERRABLE INITIALLY DEFERRED written twice for a constraint that
// is both flags, and the deferrable-only spelling losing the INITIALLY clause
// in the other direction.
func TestRenderDeferrableClauses(t *testing.T) {
	for _, c := range []struct {
		deferrable, initially bool
		want                  string
	}{
		{false, false, `UNIQUE ("route_id", "position")`},
		{true, false, `UNIQUE ("route_id", "position") DEFERRABLE`},
		{false, true, `UNIQUE ("route_id", "position") DEFERRABLE INITIALLY DEFERRED`},
		{true, true, `UNIQUE ("route_id", "position") DEFERRABLE INITIALLY DEFERRED`},
	} {
		s := deferrableTables(false)
		s.Tables[1].Uniques[0].Deferrable, s.Tables[1].Uniques[0].InitiallyDeferred = c.deferrable, c.initially
		got := renderDiff(t, &schema.Schema{Dialect: "postgres"}, s)
		if !strings.Contains(got, c.want+",") && !strings.Contains(got, c.want+")") {
			t.Errorf("deferrable %v, initially deferred %v: want %s in\n%s", c.deferrable, c.initially, c.want, got)
		}
		if c.want == `UNIQUE ("route_id", "position")` && strings.Contains(got, `UNIQUE ("route_id", "position") DEFERRABLE`) {
			t.Errorf("a plain unique became deferrable:\n%s", got)
		}
	}
}
