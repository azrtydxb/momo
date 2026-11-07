// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func loadCompositeFK(t *testing.T) *Graph {
	t.Helper()
	g, diags, err := Load([]string{"./compositefk"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	return g
}

// TestBelongsToOnCompositePKColumn is the schema half of issue 13: the
// foreign key column license_id is part of the composite key of exams, and
// the FK must still be emitted.
func TestBelongsToOnCompositePKColumn(t *testing.T) {
	g := loadCompositeFK(t)
	s, diags := BuildSchema(g.Models, pg())
	if len(diags) != 0 {
		t.Fatalf("schema diagnostics: %v", diags)
	}
	exams, ok := s.Table("exams")
	if !ok {
		t.Fatal("exams missing")
	}
	if !slices.Equal(exams.PrimaryKey, []string{"license_id", "exam_id"}) {
		t.Fatalf("primary key = %v", exams.PrimaryKey)
	}
	if len(exams.ForeignKeys) != 1 {
		t.Fatalf("foreign keys = %+v", exams.ForeignKeys)
	}
	fk := exams.ForeignKeys[0]
	if fk.Name != "exams_license_id_fkey" || fk.RefTable != "licenses" ||
		!slices.Equal(fk.Columns, []string{"license_id"}) || !slices.Equal(fk.RefColumns, []string{"license_id"}) {
		t.Fatalf("fk = %+v", fk)
	}
}

// TestDescriptorsForCompositePKReferencer covers the descriptor half: both the
// belongs_to from the composite-key model and the has_many into it generate.
func TestDescriptorsForCompositePKReferencer(t *testing.T) {
	g := loadCompositeFK(t)
	for _, m := range g.Models {
		src, ds := EmitDescriptors(m, g)
		if len(ds) != 0 {
			t.Fatalf("%s: %v", m.GoName, ds)
		}
		out := strings.Join(strings.Fields(string(src)), " ") // gofmt aligns key values
		var wants []string
		switch m.GoName {
		case "Exam":
			wants = []string{`Kind: momo.BelongsTo`, `ParentCols: []string{"license_id"}`, `ChildCols: []string{"license_id"}`}
		case "License":
			wants = []string{`Kind: momo.HasMany`, `ChildPK: []string{"license_id", "exam_id"}`}
		}
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("%s descriptors lack %q:\n%s", m.GoName, want, out)
			}
		}
	}
}

// TestBelongsToCompositeTargetRefused: a foreign key to a composite target
// key is refused with a message naming the target's columns.
func TestBelongsToCompositeTargetRefused(t *testing.T) {
	src := relHeader + `
type Region struct {
	momo.Model
	Country string ` + "`momo:\"pk\"`" + `
	Code    string ` + "`momo:\"pk\"`" + `
}
type Site struct {
	momo.Model
	ID      int64 ` + "`momo:\"pk\"`" + `
	Country string
	Region  *Region ` + "`momo:\"belongs_to,fk:Country\"`" + `
}
func (*Region) MomoMeta() *momo.Meta[Region] { return &momo.Meta[Region]{} }
func (*Site) MomoMeta() *momo.Meta[Site]     { return &momo.Meta[Site]{} }
`
	dir, pat := scratchModels(t, src)
	_, diags, err := Load([]string{pat}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sdiagsString(diags), "country, code") {
		t.Fatalf("diagnostics must name the target columns: %v", diags)
	}
}

// TestCompositeFKGeneratedCodeBuilds writes the generated meta and every
// generated descriptor package for the compositefk fixture into the module and
// compiles them, so a descriptor that does not build is caught here.
func TestCompositeFKGeneratedCodeBuilds(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join(fixturesDir(t), "compositefk", "models.go"))
	if err != nil {
		t.Fatal(err)
	}
	dir, pat := scratchModels(t, string(fixture))
	g, diags, err := Load([]string{pat}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	w := NewWriter(WriterOptions{ModuleRoot: dir})
	if err := Emit(g, w); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated code for the composite key fixture must build: %v\n%s", err, out)
	}
}
