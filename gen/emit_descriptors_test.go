// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"flag"
	"path/filepath"
	"strings"
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

func emitFixtureModel(t *testing.T, pkg, name string) []byte {
	t.Helper()
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./" + pkg}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %v", diags)
	}
	for _, m := range g.Models {
		if m.GoName != name {
			continue
		}
		src, ds := EmitDescriptors(m, g)
		if len(ds) != 0 {
			t.Fatalf("emit diagnostics: %v", ds)
		}
		return src
	}
	t.Fatalf("%s model missing", name)
	return nil
}

// TestEmitDescriptorsVectorColumns pins the descriptor emitted for each of
// the four vector types: a momo.VectorColumn that carries the tagged dim.
func TestEmitDescriptorsVectorColumns(t *testing.T) {
	src := emitFixtureModel(t, "tagvocab", "Article")
	ok, err := golden.Diff("testdata/golden/descriptors/article/descriptors.golden", src, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("descriptors differ from golden; review and rerun with -update")
	}
}

// TestEmitDescriptorsStandInVectorStaysPlain proves a type merely named like
// a vector type, not from the vector package, keeps a plain column.
func TestEmitDescriptorsStandInVectorStaysPlain(t *testing.T) {
	src := string(emitFixtureModel(t, "companion", "Doc"))
	if !strings.Contains(src, "momo.NewColumn[companion.Vector](q, \"embedding\"") || strings.Contains(src, "VectorColumn") {
		t.Fatalf("stand-in Vector must stay a plain column:\n%s", src)
	}
}
