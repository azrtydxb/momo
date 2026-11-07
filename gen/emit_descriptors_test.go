// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bytes"
	"flag"
	"os/exec"
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

// TestEmitDescriptorsVectorPermissions pins the descriptor type of a vector
// field under each write permission, the dimension read from an explicit type
// tag, and an alias to a pointer vector type.
func TestEmitDescriptorsVectorPermissions(t *testing.T) {
	src := emitFixtureModel(t, "vectorperm", "Chunk")
	ok, err := golden.Diff("testdata/golden/descriptors/chunk/descriptors.golden", src, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("descriptors differ from golden; review and rerun with -update")
	}
	for _, want := range []string{
		`Generated  momo.ReadOnlyVectorColumn[vector.Vector]`,
		`GenNull    momo.ReadOnlyVectorColumn[vector.Vector]`,
		`Once       momo.CreateOnlyVectorColumn[vector.Vector]`,
		`Frozen     momo.ImmutableVectorColumn[vector.Vector]`,
		`Plain      momo.VectorColumn[vector.Vector]`,
		`Maybe      momo.NullVectorColumn[vector.Vector]`,
		`ViaAlias   momo.NullVectorColumn[vector.Vector]`,
		`momo.NewReadOnlyVectorColumn[vector.Vector](q, "generated", 3,`,
		`momo.NewCreateOnlyVectorColumn[vector.Vector](q, "once", 3,`,
		`momo.NewImmutableVectorColumn[vector.Vector](q, "frozen", 3,`,
		`momo.NewVectorColumn[vector.Vector](q, "type_vec", 3, 0)`,
		`momo.NewVectorColumn[vector.HalfVector](q, "type_half", 4, 0)`,
		`momo.NewVectorColumn[vector.SparseVector](q, "type_sparse", 5, 0)`,
		`momo.NewVectorColumn[vector.Bits](q, "type_bits", 8, 0)`,
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("descriptors miss %q:\n%s", want, src)
		}
	}
}

// emitAndBuild loads src, lets mutate edit the loaded graph, emits every
// model's descriptors into the scratch module and builds it. It returns the
// emitted sources by model name.
func emitAndBuild(t *testing.T, src string, mutate func(*Graph)) map[string]string {
	t.Helper()
	dir, pat := scratchModels(t, src)
	g, diags, err := Load([]string{pat}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	if mutate != nil {
		mutate(g)
	}
	out := map[string]string{}
	w := NewWriter(WriterOptions{ModuleRoot: dir})
	for _, m := range g.Models {
		b, ds := EmitDescriptors(m, g)
		if len(ds) != 0 {
			t.Fatalf("%s: %v", m.GoName, ds)
		}
		out[m.GoName] = string(b)
		if err := w.File(filepath.Join(dir, "models", m.SubPkg, "momo_gen.go"), b); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated descriptors must build: %v\n%s", err, o)
	}
	return out
}

func TestNullableTagOnNonPointerEmitsNullColumn(t *testing.T) {
	out := emitAndBuild(t, relHeader+`
type Item struct {
	momo.Model `+"`momo:\"table:items\"`"+`
	ID    int64  `+"`momo:\"pk\"`"+`
	Title string `+"`momo:\"nullable\"`"+`
}
`, nil)["Item"]
	for _, want := range []string{
		"Title momo.NullColumn[string]",
		`momo.NewNullColumn[string](q, "title", momo.FlagNullable)`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestEmitDeterministic(t *testing.T) {
	dir, pat := scratchModels(t, compileSrc)
	g, diags, err := Load([]string{pat}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	for _, m := range g.Models {
		a, d1 := EmitDescriptors(m, g)
		b, d2 := EmitDescriptors(m, g)
		if len(d1) != 0 || len(d2) != 0 {
			t.Fatalf("%s: %v %v", m.GoName, d1, d2)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("%s: two emits differ", m.GoName)
		}
	}
}

func TestReservedIdentifierFieldNames(t *testing.T) {
	out := emitAndBuild(t, relHeader+`
type Odd struct {
	momo.Model `+"`momo:\"table:odds\"`"+`
	ID      int64  `+"`momo:\"pk\"`"+`
	Table   string
	As      string
	Columns string
}
`, nil)["Odd"]
	for _, want := range []string{"Table_ ", "As_ ", "Columns_ "} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestKeywordLikeUnexportedTaggedFieldIsDiagnosed(t *testing.T) {
	dir, pat := scratchModels(t, relHeader+`
type Kw struct {
	momo.Model `+"`momo:\"table:kws\"`"+`
	ID   int64  `+"`momo:\"pk\"`"+`
	type_ string `+"`momo:\"column:type\"`"+`
}
`)
	_, diags, err := Load([]string{pat}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diags {
		if strings.Contains(d.Msg, "momo tag on unexported field type_") && d.Pos.Line > 0 {
			return
		}
	}
	t.Fatalf("want positioned diagnostic, got %v", diags)
}

// TestImportAliasClashWithIdentifier renames a field to the alias of an
// imported package. The generated package must still compile.
func TestImportAliasClashWithIdentifier(t *testing.T) {
	out := emitAndBuild(t, `// SPDX-License-Identifier: Apache-2.0

package models

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

type Clash struct {
	momo.Model `+"`momo:\"table:clashes\"`"+`
	ID   int64        `+"`momo:\"pk\"`"+`
	Kind dialect.Name `+"`momo:\"column:kind\"`"+`
}
`, func(g *Graph) {
		for _, m := range g.Models {
			for _, f := range m.Fields {
				if f.GoName == "Kind" {
					f.GoName = "dialect"
				}
			}
		}
	})["Clash"]
	if !strings.Contains(out, "dialect_ ") {
		t.Fatalf("a field named like an import alias must be renamed:\n%s", out)
	}
}
