// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

// Guards the primary key of a view model in propertiesOf, which reads it
// through Graph.TableOf. The pk tag on the field marks it too, so this test
// cannot tell the two sources apart and does not fail with the old lookup;
// it pins the behavior that a view model's key is not a required write
// property.
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
