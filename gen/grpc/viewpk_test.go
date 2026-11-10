// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

// Catches: a plugin that looks a model up in Graph.Schema by table name and so
// misses a view model, which the schema does not hold as a table. A view model
// with a pk field must keep its key: Get and Delete address it by that key.
func TestViewModelKeepsItsKey(t *testing.T) {
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	_, ds := Proto(g, &Lock{}, "view.v1")
	for _, d := range ds {
		if strings.Contains(d.Msg, "primary key") {
			t.Errorf("unexpected diagnostic: %v", d)
		}
	}
}
