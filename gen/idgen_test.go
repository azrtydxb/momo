// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"go/format"
	"strings"
	"testing"
)

func TestEmitMetaIDGens(t *testing.T) {
	g, diags, err := Load([]string{"./idgen"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %v", diags)
	}
	src, ds := EmitMeta(g.Models, g)
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	if _, err := format.Source(src); err != nil {
		t.Fatalf("output is not valid Go: %v", err)
	}
	out := string(src)
	for _, want := range []string{
		`IDGens: []momo.IDGen{{Column: 0, Name: "cuid"}},`,
		`IDGens: []momo.IDGen{{Column: 0, Name: "ulid"}},`,
		`IDGens: []momo.IDGen{{Column: 0, Name: "short_token"}},`,
	} {
		if n := strings.Count(out, want); n != 1 {
			t.Errorf("%q appears %d times, want 1:\n%s", want, n, out)
		}
	}
}

func TestEmitMetaWithoutIDGensOmitsField(t *testing.T) {
	g, diags, err := Load([]string{"./models"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	src, ds := EmitMeta(g.Models, g)
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	if strings.Contains(string(src), "IDGens") {
		t.Fatal("a model without id generators must not emit IDGens")
	}
}

func TestEmitMetaIDGenProblems(t *testing.T) {
	g, diags, err := Load([]string{"./idgenbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %v", diags)
	}
	_, ds := EmitMeta(g.Models, g)
	var msgs []string
	for _, d := range ds {
		msgs = append(msgs, d.Msg)
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{
		"idgen needs a string field, not int64",
		"idgen is declared by both the tag and MomoConfig",
		`idgen "CUID" must be a lowercase name`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing diagnostic %q in:\n%s", want, all)
		}
	}
}
