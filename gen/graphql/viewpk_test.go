// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

// Catches: singleIntPK looking a model up in Graph.Schema by table name, which
// does not hold a view model, so a view model with an integer pk field got no
// by-key query.
func TestSingleIntPKOfViewModel(t *testing.T) {
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	cols := []column{{name: "id", field: "id", sc: scInt64}, {name: "name", field: "name", sc: scString}}
	for _, m := range g.Models {
		if got := singleIntPK(g, m, cols); got != "id" {
			t.Errorf("singleIntPK(%s) = %q, want id", m.GoName, got)
		}
	}
}

// Guards: the GraphQL schema stays read-only for a view model. The generator
// emits queries only, so it has no write operation to withhold; this pins that
// no mutation, create or update input exists for a model that embeds
// momo.View. It passes without the view fix, as there is nothing to fix here.
func TestViewModelHasNoMutationOrInput(t *testing.T) {
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	sdl, ds := SDL(g)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	s := string(sdl)
	if strings.Contains(s, "type Mutation") || strings.Contains(s, "extend type Mutation") {
		t.Error("the schema declares a Mutation")
	}
	for _, in := range []string{"input ActiveAccountInput", "input CreateActiveAccount", "input UpdateActiveAccount"} {
		if strings.Contains(s, in) {
			t.Errorf("the schema has %q for a view model", in)
		}
	}
	if !strings.Contains(s, "ActiveAccount") {
		t.Error("the view model is not in the schema at all")
	}
}
