// SPDX-License-Identifier: Apache-2.0

package viz

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/schema"
)

var update = flag.Bool("update", false, "rewrite golden files")

func loadExample(t *testing.T) *gen.Graph {
	t.Helper()
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	return g
}

// Catches: a relation missing from the edge model, drawn from the wrong table
// to the wrong table, or given the wrong cardinality.
func TestVizEdges(t *testing.T) {
	g := loadExample(t)
	edges := EdgesFromGraph(g)
	want := map[string]string{
		"comments->documents": OneToMany,
		"documents->tags":     ManyToMany,
		"documents->tenants":  OneToMany,
	}
	got := map[string]string{}
	for _, e := range edges {
		got[e.From+"->"+e.To] = e.Cardinality
	}
	if len(got) != len(want) {
		t.Errorf("edges = %v, want exactly %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("edge %s: got %q want %q (all: %v)", k, got[k], v, got)
		}
	}
	for _, e := range edges {
		switch e.From + "->" + e.To {
		case "comments->documents":
			if strings.Join(e.FromColumns, ",") != "document_id" || strings.Join(e.ToColumns, ",") != "id" {
				t.Errorf("comments->documents columns = %v -> %v", e.FromColumns, e.ToColumns)
			}
		case "documents->tags":
			if e.Label != "document_tags" {
				t.Errorf("m2m label = %q, want the join table", e.Label)
			}
		}
	}
	for i := 1; i < len(edges); i++ {
		a, b := edges[i-1], edges[i]
		if a.From > b.From || (a.From == b.From && a.To > b.To) {
			t.Errorf("edges not sorted: %v before %v", a, b)
		}
	}
	names := map[string]bool{}
	for _, tb := range Tables(g) {
		names[tb.Name] = true
	}
	for _, n := range []string{"comments", "document_tags", "documents", "tags", "tenants"} {
		if !names[n] {
			t.Errorf("table %s missing from Tables: %v", n, names)
		}
	}
}

// Catches: snapshot edges built from the wrong side of a foreign key, or
// from something other than the snapshot.
func TestVizEdgesFromSchema(t *testing.T) {
	s := &schema.Schema{Tables: []schema.Table{
		{Name: "b", ForeignKeys: []schema.ForeignKey{{Name: "b_a_fk", Columns: []string{"a_id"}, RefTable: "a", RefColumns: []string{"id"}}}},
		{Name: "a"},
	}}
	edges := EdgesFromSchema(s)
	if len(edges) != 1 {
		t.Fatalf("edges = %v", edges)
	}
	e := edges[0]
	if e.From != "b" || e.To != "a" || e.Cardinality != OneToMany || strings.Join(e.FromColumns, ",") != "a_id" || strings.Join(e.ToColumns, ",") != "id" {
		t.Fatalf("edge = %+v", e)
	}
	tbs := TablesFromSchema(s)
	if len(tbs) != 2 || tbs[0].Name != "a" || tbs[1].Name != "b" {
		t.Fatalf("tables not sorted by name: %v", tbs)
	}
}
