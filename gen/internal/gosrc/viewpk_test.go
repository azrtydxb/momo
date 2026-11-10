// SPDX-License-Identifier: Apache-2.0

package gosrc

import (
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

// Catches: SinglePK looking a model up in Graph.Schema by table name, which
// does not hold a view model, so a view model with a pk field had no key.
func TestSinglePKOfViewModel(t *testing.T) {
	dir, err := filepath.Abs("../../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	for _, m := range g.Models {
		f := SinglePK(g, m)
		if f == nil || f.GoName != "ID" {
			t.Errorf("SinglePK(%s) = %v, want field ID", m.GoName, f)
		}
	}
}
