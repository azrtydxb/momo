// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
