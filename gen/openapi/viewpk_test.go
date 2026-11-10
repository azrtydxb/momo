// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

// Catches: propertiesOf looking a model up in Graph.Schema by table name,
// which does not hold a view model, so the primary key of a view model was
// taken for an ordinary required column.
func TestViewModelPrimaryKeyIsNotRequired(t *testing.T) {
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	for _, m := range g.Models {
		props, ds := propertiesOf(g, m)
		if len(ds) != 0 {
			t.Fatalf("%s: %v", m.GoName, ds)
		}
		for _, p := range props {
			if p.column == "id" && p.required {
				t.Errorf("%s: the primary key id is marked required on write", m.GoName)
			}
		}
	}
}
