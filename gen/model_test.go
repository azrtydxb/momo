// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestModelDiscovery(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./models"}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: err=%v diags=%v", err, diags)
	}
	byName := map[string]*Model{}
	for _, m := range g.Models {
		byName[m.GoName] = m
	}
	doc, ok := byName["Document"]
	if !ok || len(byName) != 2 {
		t.Fatalf("models=%v", keys(byName))
	}
	if doc.Table != "documents" {
		t.Fatalf("table=%q", doc.Table)
	}
	var cols []string
	for _, f := range doc.Fields {
		cols = append(cols, f.Column)
	}
	want := []string{"id", "tenant_id", "title", "note", "created_at"}
	if !equal(cols, want) {
		t.Fatalf("columns=%v want %v", cols, want)
	}
	if doc.Fields[3].GoType != "*string" || !doc.Fields[3].Nullable {
		t.Fatalf("note field: %+v", doc.Fields[3])
	}
	if doc.Fields[4].Access != ReadOnly {
		t.Fatalf("created_at access=%v", doc.Fields[4].Access)
	}
	if doc.Fields[4].GoType != "time.Time" {
		t.Fatalf("created_at type=%q", doc.Fields[4].GoType)
	}
	if doc.Fields[0].Path[0] != 1 || doc.Fields[1].Path[0] != 2 || doc.Fields[0].Access != Writable {
		t.Fatalf("path/access: %+v %+v", doc.Fields[1], doc.Fields[0])
	}
	if doc.PkgName != "models" || doc.Pos.Line == 0 || !strings.HasSuffix(doc.File, "models.go") || doc.PkgDir == "" {
		t.Fatalf("metadata: %+v", doc)
	}
	if byName["Tenant"].Table != "tenants" {
		t.Fatalf("tenant table=%q", byName["Tenant"].Table)
	}
}

func TestModelDiscoveryNoModels(t *testing.T) {
	g, diags, err := Load([]string{"./nomodels"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 || len(g.Models) != 0 {
		t.Fatalf("err=%v diags=%v models=%d", err, diags, len(g.Models))
	}
}

func TestModelDiscoveryDiagnostics(t *testing.T) {
	g, diags, err := Load([]string{"./badtags"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	var all []string
	for _, d := range diags {
		all = append(all, d.String())
	}
	joined := strings.Join(all, "\n")
	for _, w := range []string{
		"momo tags on non-model type NotAModel",
		"momo tag on unexported field hidden",
		"momo.Model must be embedded by value in Ptr",
		"generic type Generic cannot be a model",
		"bogus_option",
	} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing diagnostic %q in:\n%s", w, joined)
		}
	}
	for _, d := range diags {
		if d.Pos.Line == 0 || d.Pos.Filename == "" {
			t.Errorf("unpositioned diagnostic: %v", d)
		}
	}
	// Twice embeds the marker twice: a compile error reported by the type
	// checker; discovery must not turn it into extra models.
	if !strings.Contains(joined, "redeclared") && !strings.Contains(joined, "duplicate field") {
		t.Errorf("no compiler diagnostic for duplicate embed:\n%s", joined)
	}
	byName := map[string]*Model{}
	for _, m := range g.Models {
		byName[m.GoName] = m
	}
	if _, ok := byName["Priv"]; !ok {
		t.Fatalf("Priv missing: %v", keys(byName))
	}
	for _, bad := range []string{"NotAModel", "Ptr", "Generic", "Marker", "AsAlias"} {
		if _, ok := byName[bad]; ok {
			t.Errorf("%s must not be a model", bad)
		}
	}
	if n := countOf(g.Models, "Twice"); n > 1 {
		t.Errorf("Twice discovered %d times", n)
	}
	var privCols []string
	for _, f := range byName["Priv"].Fields {
		privCols = append(privCols, f.Column)
	}
	if !equal(privCols, []string{"id"}) {
		t.Errorf("Priv columns=%v", privCols)
	}
	nd := byName["NamedDefault"]
	if nd == nil || len(nd.Fields) != 1 || nd.Table != "named_defaults" {
		t.Errorf("NamedDefault: %+v", nd)
	}
}

func TestModelDiscoveryAliasEmbed(t *testing.T) {
	g, diags, err := Load([]string{"./aliasembed"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*Model{}
	for _, m := range g.Models {
		byName[m.GoName] = m
	}
	if m := byName["ViaAlias"]; m == nil || len(m.Fields) != 1 || m.Fields[0].GoName != "ID" || m.Table != "via_aliases" {
		t.Errorf("ViaAlias must be a model with one column: %+v (models %v)", m, keys(byName))
	}
	// X is a defined type distinct from momo.Model, so embedding it is not
	// the marker: not a model, and no diagnostic.
	for _, n := range []string{"ViaPtrAlias", "ViaDefined", "X", "M"} {
		if _, ok := byName[n]; ok {
			t.Errorf("%s must not be a model", n)
		}
	}
	var ptr, bogus int
	for _, d := range diags {
		if strings.Contains(d.Msg, "momo.Model must be embedded by value in ViaPtrAlias") {
			ptr++
		}
		if strings.Contains(d.Msg, "ViaDefined") {
			t.Errorf("unexpected diagnostic for ViaDefined: %v", d)
		}
		if strings.Contains(d.Msg, "bogus_option") {
			bogus++
			if !strings.Contains(d.Msg, "field E, F") {
				t.Errorf("tag diagnostic must name the fields: %v", d)
			}
		}
	}
	if ptr != 1 {
		t.Errorf("pointer-to-alias diagnostics=%d want 1: %v", ptr, diags)
	}
	if bogus != 1 {
		t.Errorf("multi-name field tag reported %d times, want once: %v", bogus, diags)
	}
}

func countOf(ms []*Model, name string) int {
	n := 0
	for _, m := range ms {
		if m.GoName == name {
			n++
		}
	}
	return n
}

func keys(m map[string]*Model) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
