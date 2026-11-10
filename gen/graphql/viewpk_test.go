// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"path/filepath"
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
