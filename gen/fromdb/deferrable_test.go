// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Catches: the deferrability of an inspected constraint being dropped on the
// way to the models (the init migration is then plain), a deferrable foreign
// key written as a belongs_to tag, which has no way to say it, and a plain
// constraint gaining a clause.
func TestGenerateFromDBDeferrableConstraints(t *testing.T) {
	stops := schema.Table{
		Name: "stops",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "route_id", Type: "bigint"},
			{Name: "position", Type: "integer"},
		},
		PrimaryKey:  []string{"id"},
		Uniques:     []schema.Unique{{Name: "stops_route_position_key", Columns: []string{"route_id", "position"}, Deferrable: true, InitiallyDeferred: true}},
		ForeignKeys: []schema.ForeignKey{{Name: "stops_route_id_fkey", Columns: []string{"route_id"}, RefTable: "routes", RefColumns: []string{"id"}, Deferrable: true}},
	}
	routes := schema.Table{
		Name:                 "routes",
		Columns:              []schema.Column{{Name: "id", Type: "bigint"}},
		PrimaryKey:           []string{"id"},
		PrimaryKeyDeferrable: true,
		Uniques:              []schema.Unique{{Name: "routes_id_key", Columns: []string{"id"}, Deferrable: true}},
	}
	labels := schema.Table{
		Name:                        "labels",
		Columns:                     []schema.Column{{Name: "id", Type: "bigint"}},
		PrimaryKey:                  []string{"id"},
		PrimaryKeyDeferrable:        true,
		PrimaryKeyInitiallyDeferred: true,
		Uniques:                     []schema.Unique{{Name: "labels_id_key", Columns: []string{"id"}}},
	}
	s := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: dialect.Postgres, Tables: []schema.Table{stops, routes, labels}}
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustContain(t, "stops model", files["stops"],
		`c.Unique("stops_route_position_key", &m.RouteID, &m.Position).InitiallyDeferred()`,
		`c.ForeignKey("stops_route_id_fkey", []any{&m.RouteID}, "routes", []string{"id"}).Deferrable()`)
	if strings.Contains(files["stops"], "belongs_to") {
		t.Errorf("a deferrable foreign key is written as a belongs_to tag:\n%s", files["stops"])
	}
	mustContain(t, "routes model", files["routes"], "c.PrimaryKeyDeferrable()", `c.Unique("routes_id_key", &m.ID).Deferrable()`)
	if strings.Contains(files["routes"], "InitiallyDeferred") {
		t.Errorf("a deferrable-only key is written initially deferred:\n%s", files["routes"])
	}
	mustContain(t, "labels model", files["labels"], "c.PrimaryKeyInitiallyDeferred()", `c.Unique("labels_id_key", &m.ID)`)
	if strings.Contains(files["labels"], "Deferrable") || strings.Contains(files["labels"], `"labels_id_key", &m.ID).`) {
		t.Errorf("a plain unique gained a deferrability call:\n%s", files["labels"])
	}
}
