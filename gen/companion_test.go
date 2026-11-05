// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"
)

func diagStrings(diags []Diagnostic) string {
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.String())
	}
	return strings.Join(msgs, "\n")
}

func TestCompanionReaderAccepts(t *testing.T) {
	g, diags, err := Load([]string{"./companion"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	var doc *Model
	for _, m := range g.Models {
		if m.GoName == "Doc" {
			doc = m
		}
	}
	if doc == nil || len(diags) != 0 {
		t.Fatalf("doc=%v diags=%v", doc != nil, diags)
	}
	if len(doc.Config.Indexes) != 3 || len(doc.Config.Uniques) != 1 || len(doc.Config.Enums) != 1 {
		t.Fatalf("companion=%+v", doc.Config)
	}
	c := doc.Config
	first := c.Indexes[0]
	if first.Name != "docs_title_idx" || first.Where != "title <> ''" ||
		len(first.Parts) != 1 || first.Parts[0].Column != "title" || !first.Parts[0].Desc {
		t.Fatalf("first index=%+v", first)
	}
	emb := c.Indexes[2]
	if emb.Method != "hnsw" || emb.Parts[0].OpClass != "vector_cosine_ops" ||
		len(emb.With) != 1 || emb.With[0].Key != "m" || emb.With[0].Value != "16" {
		t.Fatalf("hnsw index=%+v", emb)
	}
	if c.Indexes[1].Parts[0].Expr != "lower(title)" {
		t.Fatalf("expr index=%+v", c.Indexes[1])
	}
	if c.Defaults["title"] != "'untitled', 'x'" || c.Comments["body"] != "markdown, with commas" {
		t.Fatalf("defaults=%v comments=%v", c.Defaults, c.Comments)
	}
	if len(c.Checks) != 1 || c.Checks[0].Expr != "length(title) > 0" || len(c.Enums[0].Values) != 2 {
		t.Fatalf("checks=%v enums=%v", c.Checks, c.Enums)
	}
}

func TestCompanionStructuredArguments(t *testing.T) {
	g, diags, err := Load([]string{"./companionstruct"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 || len(g.Models) != 1 {
		t.Fatalf("err=%v diags=%v models=%d", err, diags, len(g.Models))
	}
	c := g.Models[0].Config
	if len(c.Views) != 1 || !c.Views[0].Materialized || c.Views[0].Depends[0] != "accounts" {
		t.Fatalf("views=%+v", c.Views)
	}
	if len(c.Functions) != 1 || len(c.Triggers) != 1 || c.Triggers[0].Events[0] != "UPDATE" || !c.Triggers[0].ForEachRow {
		t.Fatalf("functions=%+v triggers=%+v", c.Functions, c.Triggers)
	}
	if len(c.Policies) != 1 || c.RLS == nil || !c.RLS.Enabled || c.Partition == nil || c.Partition.Strategy != "range" {
		t.Fatalf("policies=%+v rls=%+v partition=%+v", c.Policies, c.RLS, c.Partition)
	}
}

func TestGenerateDiagnostics(t *testing.T) {
	g, diags, err := Load([]string{"./companionbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	// bad.go carries the licence header, so the plan's lines 14 and 16 are 16 and 18.
	if !strings.Contains(joined, "bad.go:16:") || !strings.Contains(joined, "bad.go:18:") {
		t.Fatalf("missing file:line:col diagnostics:\n%s", joined)
	}
	if len(g.Models) != 0 || len(diags) == 0 {
		t.Fatalf("partial output on invalid input: models=%d", len(g.Models))
	}
}

func TestCompanionHostileShapes(t *testing.T) {
	_, diags, err := Load([]string{"./companionbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	want := []struct{ at, msg string }{
		{"bad.go:16:10:", "unsupported companion expression: idx"},
		{"bad.go:17:2:", `unsupported companion expression: name := "x"`},
		{"bad.go:18:10:", "unsupported companion expression: name"},
		// control flow is never descended into
		{"hostile.go:19:2:", "unsupported companion expression: for "},
		{"hostile.go:22:2:", "unsupported companion expression: if "},
		// variable and concatenated arguments
		{"hostile.go:25:10:", "unsupported companion expression: idxVar"},
		{"hostile.go:26:10:", `unsupported companion expression: "loops_a" + "_check"`},
		// field references that are not persisted fields of this model
		{"hostile.go:38:", "field reference &r.Hidden is not a persisted field of Refs"},
		{"hostile.go:39:", "field reference &r.Model is not a persisted field of Refs"},
		{"hostile.go:40:", "field reference &r.Owner.Name is not a persisted field of Refs"},
		{"hostile.go:41:", "unsupported companion expression: &(&Loop{}).Name"},
		{"hostile.go:42:", "field &r.ID is not a part of index"},
		// unnamed receiver
		{"hostile.go:52:", "unsupported companion expression: &(&Anon{}).Name"},
		// duplicate and clashing names
		{"hostile.go:64:", `index name "dups_name_idx" is already declared at`},
		{"hostile.go:65:", `index name "dups_slug_idx" is already produced by the index tag on field Slug`},
		{"hostile.go:67:", `duplicate default for column "name"`},
		// signature
		{"hostile.go:75:", "MomoConfig must take exactly one *momo.Config parameter"},
		// structured arguments
		{"hostile.go:83:", "unsupported companion expression: name :="},
		{"hostile.go:84:", "unsupported companion expression: name"},
		{"hostile.go:85:", "unsupported companion expression: schema.View{"},
		{"hostile.go:86:", "unsupported companion expression: []schema.Index{}"},
		{"hostile.go:88:", `unsupported companion expression: "p" + "q"`},
	}
	for _, w := range want {
		found := false
		for _, line := range strings.Split(joined, "\n") {
			if strings.Contains(line, w.at) && strings.Contains(line, w.msg) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing diagnostic %s %q in:\n%s", w.at, w.msg, joined)
		}
	}
	for _, bad := range []string{"hostile.go:23:", "hostile.go:63:", "hostile.go:66:", "hostile.go:87:"} {
		if strings.Contains(joined, bad) {
			t.Errorf("unexpected diagnostic %q in:\n%s", bad, joined)
		}
	}
}

func TestCompanionUnknownMethods(t *testing.T) {
	g, diags, err := Load([]string{"./companionunknown"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	for _, w := range []string{
		`unknown.go:14:2: unsupported companion expression: c.Frobnicate("x", &t.Name)`,
		`unknown.go:15:2: unsupported companion expression: c.Index("things_name_idx", &t.Name).Frobnicate()`,
	} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in:\n%s", w, joined)
		}
	}
	if len(g.Models) != 0 {
		t.Fatalf("models=%d", len(g.Models))
	}
}

func TestCompanionMixinFieldPaths(t *testing.T) {
	g, diags, err := Load([]string{"./companionmixin"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("err=%v diags=%s", err, diagStrings(diags))
	}
	byName := map[string]*Model{}
	for _, m := range g.Models {
		byName[m.GoName] = m
	}
	p := byName["Place"]
	if p == nil || len(p.Config.Indexes) != 3 {
		t.Fatalf("place=%+v", p)
	}
	// &p.Lon and &p.Coords.Lon are the same field.
	if a, b := p.Config.Indexes[0].Parts[0].Column, p.Config.Indexes[1].Parts[0].Column; a != "lon" || b != "lon" {
		t.Fatalf("promoted=%q path=%q", a, b)
	}
	if c := p.Config.Indexes[2].Parts[0].Column; c != "home_lat" {
		t.Fatalf("prefixed mixin column=%q", c)
	}
	if p.Config.Defaults["lat"] != "0" {
		t.Fatalf("defaults=%v", p.Config.Defaults)
	}
	// A value receiver is accepted: the plan allows *T or T.
	v := byName["ByValue"]
	if v == nil || len(v.Config.Indexes) != 1 {
		t.Fatalf("by value=%+v", v)
	}
}

func TestCompanionMoreShapes(t *testing.T) {
	_, diags, err := Load([]string{"./companionbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	want := []struct{ at, msg string }{
		{"shapes.go:17:", "unsupported companion expression: f := c.Index"},
		{"shapes.go:18:", `unsupported companion expression: f("shapes_name_idx", &s.Name)`},
		{"shapes.go:20:", "unsupported companion expression: v"},
		{"shapes.go:22:", "duplicate RLS declaration"},
		{"shapes.go:24:", "duplicate Partition declaration"},
		{"shapes.go:26:", "duplicate PartitionOf declaration"},
		{"shapes.go:27:", `duplicate Where on index "shapes_dup_opt_idx"`},
	}
	for _, w := range want {
		found := false
		for _, line := range strings.Split(joined, "\n") {
			if strings.Contains(line, w.at) && strings.Contains(line, w.msg) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing diagnostic %s %q in:\n%s", w.at, w.msg, joined)
		}
	}
	for _, bad := range []string{"shapes.go:21:", "shapes.go:23:", "shapes.go:25:"} {
		if strings.Contains(joined, bad) {
			t.Errorf("unexpected diagnostic %q in:\n%s", bad, joined)
		}
	}
}

func TestCompanionTypeErrorShapes(t *testing.T) {
	g, diags, err := Load([]string{"./companiontypeerr"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	for _, w := range []string{
		"typeerr.go:17:2: unsupported companion expression: c.Index",
		"typeerr.go:18:9: unsupported companion expression: schema.Policy{}",
		"typeerr.go:19:",
		"typeerr.go:20:",
		"unsupported companion expression: []int{}",
	} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in:\n%s", w, joined)
		}
	}
	if len(g.Models) != 0 {
		t.Fatalf("models=%d", len(g.Models))
	}
}
