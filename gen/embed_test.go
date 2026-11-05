// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedNestedAliasAndSkip(t *testing.T) {
	g, diags, err := Load([]string{"./embedmore"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: err=%v diags=%v", err, diags)
	}
	if len(g.Models) != 1 {
		t.Fatalf("models=%d", len(g.Models))
	}
	m := g.Models[0]
	got := map[string][]int{}
	var cols []string
	for _, f := range m.Fields {
		cols = append(cols, f.Column)
		got[f.Column] = f.Path
	}
	want := []string{"id", "name", "geo_lat", "geo_lon", "o_name", "o_geo_lat", "o_geo_lon"}
	if !equal(cols, want) {
		t.Fatalf("columns=%v want %v", cols, want)
	}
	if p := got["o_geo_lat"]; len(p) != 3 || p[0] != 3 || p[1] != 1 || p[2] != 0 {
		t.Fatalf("nested path=%v", p)
	}
	if p := got["geo_lat"]; len(p) != 3 || p[0] != 2 {
		t.Fatalf("aliased mixin path=%v", p)
	}
	for _, f := range m.Fields {
		if f.Column == "o_geo_lat" && (f.Base != "o_geo_" || f.GoName != "Lat") {
			t.Fatalf("base/goname: %+v", f)
		}
	}
	if len(m.Indexes) != 2 || !m.Indexes[0].Unique || m.Indexes[0].Name != "geo_lon_u" || m.Indexes[0].Parts[0].Column != "geo_lon" {
		t.Fatalf("indexes=%+v", m.Indexes)
	}
	if m.Indexes[1].Name != "geo_lon_u" || m.Indexes[1].Parts[0].Column != "o_geo_lon" {
		t.Fatalf("second index=%+v", m.Indexes[1])
	}
}

func TestEmbeddedDiagnostics(t *testing.T) {
	_, diags, err := Load([]string{"./embedbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Msg)
	}
	for _, sub := range []string{
		`column "billing_street" is produced by fields Street and BillingStreet`,
		"embedded pointer field Address is not supported",
		"field Count is tagged embedded but is not a struct",
		"prefix on field Billing requires the embedded option",
	} {
		found := false
		for _, m := range msgs {
			if strings.Contains(m, sub) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing diagnostic %q in %q", sub, msgs)
		}
	}
	if len(diags) != 4 {
		t.Errorf("want 4 diagnostics, got %d: %q", len(diags), msgs)
	}
}

func TestEmbeddedAndMixins(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./mixins"}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: err=%v diags=%v", err, diags)
	}
	if len(g.Models) != 1 {
		t.Fatalf("models=%d", len(g.Models))
	}
	m := g.Models[0]
	var cols []string
	for _, f := range m.Fields {
		cols = append(cols, f.Column)
	}
	want := []string{"id", "created_at", "updated_at", "billing_street", "billing_city", "shipping_street", "shipping_city"}
	if !equal(cols, want) {
		t.Fatalf("columns=%v want %v", cols, want)
	}
	if !hasIndex(m, "articles_updated_at_idx") {
		t.Fatalf("mixin index missing; indexes=%v", indexNames(m))
	}
	if m.Fields[3].Path == nil || len(m.Fields[3].Path) != 2 {
		t.Fatalf("embedded path not recorded: %+v", m.Fields[3])
	}
}

func hasIndex(m *Model, name string) bool {
	for _, n := range indexNames(m) {
		if n == name {
			return true
		}
	}
	return false
}

func indexNames(m *Model) []string {
	var out []string
	for _, ix := range m.Indexes {
		out = append(out, ix.Name)
	}
	return out
}
