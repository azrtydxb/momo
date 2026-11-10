// SPDX-License-Identifier: Apache-2.0

package viz

import (
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

// Catches: keyOf looking a model up in Graph.Schema by table name, which does
// not hold a view model, so a view model had no key columns.
func TestKeyOfViewModel(t *testing.T) {
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	for _, m := range g.Models {
		if got := keyOf(g, m); len(got) != 1 || got[0] != "id" {
			t.Errorf("keyOf(%s) = %v, want [id]", m.GoName, got)
		}
	}
}
