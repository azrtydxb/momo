// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/internal/golden"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestEmitDescriptorsGolden(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./models"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %v", diags)
	}
	var doc *Model
	for _, m := range g.Models {
		if m.GoName == "Document" {
			doc = m
		}
	}
	if doc == nil {
		t.Fatal("Document model missing")
	}
	src, ds := EmitDescriptors(doc, g)
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	ok, err := golden.Diff("testdata/golden/descriptors/document/momo_gen.go", src, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("descriptors differ from golden; review and rerun with -update")
	}
}
