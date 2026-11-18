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

// TestPostgresInspectsManyViewsTogether pins that the dependencies and indexes
// of several views, materialized or not, are each attributed to their own view
// and keep their order when read in one query for all of them.
func TestPostgresInspectsManyViewsTogether(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	d := db.Driver()
	mustExec(t, d, `CREATE TABLE a_items (id bigint PRIMARY KEY, owner text NOT NULL, qty int NOT NULL)`)
	mustExec(t, d, `CREATE TABLE b_owners (owner text PRIMARY KEY, region text NOT NULL)`)
	mustExec(t, d, `CREATE VIEW plain_v AS SELECT id FROM a_items`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW m_totals AS SELECT owner, sum(qty) AS total FROM a_items GROUP BY owner`)
	mustExec(t, d, `CREATE UNIQUE INDEX m_totals_owner ON m_totals (owner)`)
	mustExec(t, d, `CREATE INDEX m_totals_a_total ON m_totals (total DESC)`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW m_regions AS SELECT o.region, count(*) AS n FROM b_owners o JOIN a_items i ON i.owner = o.owner GROUP BY o.region`)
	mustExec(t, d, `CREATE INDEX m_regions_region ON m_regions (region)`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW m_bare AS SELECT id FROM a_items`)
	got, err := inspectOnce(ctx, t, db, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	views := map[string]schema.View{}
	for _, v := range got.Views {
		views[v.Name] = v
	}
	if len(views) != 4 {
		t.Fatalf("views: %+v", got.Views)
	}
	names := make([]string, 0, len(got.Views))
	for _, v := range got.Views {
		names = append(names, v.Name)
	}
	if !slices.Equal(names, []string{"m_bare", "m_regions", "m_totals", "plain_v"}) {
		t.Fatalf("view order = %v", names)
	}
	for name, want := range map[string][]string{
		"plain_v":   {"a_items"},
		"m_totals":  {"a_items"},
		"m_regions": {"a_items", "b_owners"},
		"m_bare":    {"a_items"},
	} {
		if !slices.Equal(views[name].Depends, want) {
			t.Errorf("%s depends = %v, want %v", name, views[name].Depends, want)
		}
	}
	idx := func(name string) []string {
		var out []string
		for _, ix := range views[name].Indexes {
			out = append(out, ix.Name)
		}
		return out
	}
	if g := idx("m_totals"); !slices.Equal(g, []string{"m_totals_a_total", "m_totals_owner"}) {
		t.Errorf("m_totals indexes = %v", g)
	}
	if g := idx("m_regions"); !slices.Equal(g, []string{"m_regions_region"}) {
		t.Errorf("m_regions indexes = %v", g)
	}
	if g := idx("m_bare"); len(g) != 0 {
		t.Errorf("m_bare indexes = %v", g)
	}
	if g := idx("plain_v"); len(g) != 0 {
		t.Errorf("plain_v indexes = %v", g)
	}
}
