// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scratchModels writes src as package models into a fresh directory below
// testdata (inside the module, so it can import momo) and returns the
// directory and its load pattern.
func scratchModels(t *testing.T, src string) (dir, pattern string) {
	t.Helper()
	base, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	dir, err = os.MkdirTemp(base, "scratch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.MkdirAll(filepath.Join(dir, "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models", "models.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, "./models"
}

const relHeader = `// SPDX-License-Identifier: Apache-2.0

package models

import "github.com/azrtydxb/momo"

`

func TestRelationDiagnostics(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"target not a model", `
type Plain struct{ ID int64 }
type A struct {
	momo.Model
	ID int64 ` + "`momo:\"pk\"`" + `
	P  []Plain ` + "`momo:\"has_many,fk:ID\"`" + `
}`, "relation target Plain is not a momo model"},
		{"polytype without polyvalue", `
type C struct {
	momo.Model
	ID int64 ` + "`momo:\"pk\"`" + `
	OwnerType string
	OwnerID int64
}
type A struct {
	momo.Model
	ID int64 ` + "`momo:\"pk\"`" + `
	Cs []C ` + "`momo:\"has_many,fk:OwnerID,polytype:owner_type\"`" + `
}`, "polymorphic relation Cs needs both polytype and polyvalue"},
		{"polymorphic belongs_to", `
type B struct {
	momo.Model
	ID int64 ` + "`momo:\"pk\"`" + `
}
type A struct {
	momo.Model
	ID int64 ` + "`momo:\"pk\"`" + `
	BID int64
	B *B ` + "`momo:\"belongs_to,fk:BID,polytype:t,polyvalue:v\"`" + `
}`, "polymorphic is only valid for has_one and has_many"},
		{"polymorphic column missing", `
type C struct {
	momo.Model
	ID int64 ` + "`momo:\"pk\"`" + `
	OwnerID int64
}
type A struct {
	momo.Model
	ID int64 ` + "`momo:\"pk\"`" + `
	Cs []C ` + "`momo:\"has_many,fk:OwnerID,polytype:owner_type,polyvalue:a\"`" + `
}`, "polymorphic column owner_type is not a field of C"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, pat := scratchModels(t, relHeader+tc.src)
			g, diags, err := Load([]string{pat}, Config{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range diags {
				if strings.Contains(d.Msg, tc.want) && d.Pos.Line > 0 {
					found = true
				}
			}
			if !found {
				t.Fatalf("want positioned diagnostic %q, got %v", tc.want, diags)
			}
			if len(g.Models) != 0 {
				t.Fatalf("an invalid package must yield no models, got %d", len(g.Models))
			}
		})
	}
}

const compileSrc = relHeader + `
type Status string

type Kind string

type Tag struct {
	momo.Model ` + "`momo:\"table:tags\"`" + `
	ID   int64 ` + "`momo:\"pk\"`" + `
	Name string
}

type Profile struct {
	momo.Model ` + "`momo:\"table:profiles\"`" + `
	ID    Status ` + "`momo:\"pk\"`" + `
	DocID *int64 ` + "`momo:\"column:doc_id\"`" + `
	Note  string
}

type Comment struct {
	momo.Model ` + "`momo:\"table:comments\"`" + `
	ID        int64 ` + "`momo:\"pk\"`" + `
	OwnerID   int64
	OwnerType Kind
}

type Doc struct {
	momo.Model ` + "`momo:\"table:docs\"`" + `
	ID       int64    ` + "`momo:\"pk,identity\"`" + `
	Owner    string   ` + "`momo:\"create_only\"`" + `
	Serial   string   ` + "`momo:\"immutable\"`" + `
	Slug     *string  ` + "`momo:\"read_only\"`" + `
	Tags     []Tag    ` + "`momo:\"many2many,join:doc_tags\"`" + `
	Profile  *Profile ` + "`momo:\"has_one,fk:DocID\"`" + `
	Comments []Comment ` + "`momo:\"has_many,fk:OwnerID,polytype:owner_type,polyvalue:doc\"`" + `
	Cover    *Comment  ` + "`momo:\"has_one,fk:OwnerID,polytype:owner_type,polyvalue:cover\"`" + `
}

type Reply struct {
	momo.Model ` + "`momo:\"table:replies\"`" + `
	ID     Status   ` + "`momo:\"pk\"`" + `
	DocID  int64
	Doc    *Doc     ` + "`momo:\"belongs_to,fk:DocID\"`" + `
	ProfID *Status
	Prof   *Profile ` + "`momo:\"belongs_to,fk:ProfID\"`" + `
}

func (*Tag) MomoMeta() *momo.Meta[Tag]         { return &momo.Meta[Tag]{} }
func (*Profile) MomoMeta() *momo.Meta[Profile] { return &momo.Meta[Profile]{} }
func (*Comment) MomoMeta() *momo.Meta[Comment] { return &momo.Meta[Comment]{} }
func (*Doc) MomoMeta() *momo.Meta[Doc]         { return &momo.Meta[Doc]{} }
func (*Reply) MomoMeta() *momo.Meta[Reply]     { return &momo.Meta[Reply]{} }
`

// TestDescriptorsCompile generates descriptors for every model of a fixture
// with every relation kind into the module and builds the result.
func TestDescriptorsCompile(t *testing.T) {
	dir, pat := scratchModels(t, compileSrc)
	g, diags, err := Load([]string{pat}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	w := NewWriter(WriterOptions{ModuleRoot: dir})
	for _, m := range g.Models {
		src, ds := EmitDescriptors(m, g)
		if len(ds) != 0 {
			t.Fatalf("%s: %v", m.GoName, ds)
		}
		if err := w.File(filepath.Join(dir, "models", m.SubPkg, "momo_gen.go"), src); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated descriptors must build: %v\n%s", err, out)
	}

	// Run a program that sets a child through each polymorphic relation and
	// checks the discriminator value on the child.
	imp := "github.com/azrtydxb/momo/gen/testdata/" + filepath.Base(dir) + "/models"
	prog := `package main

import (
	"fmt"
	"os"

	models "` + imp + `"
	doc "` + imp + `/doc"
)

func main() {
	var c models.Comment
	doc.Comments.SetChildKey(&c, []any{int64(7)})
	var v models.Comment
	doc.Cover.SetChildKey(&v, []any{int64(9)})
	if c.OwnerID != 7 || c.OwnerType != "doc" || v.OwnerID != 9 || v.OwnerType != "cover" {
		fmt.Printf("has_many %+v has_one %+v\n", c, v)
		os.Exit(1)
	}
}
`
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "main.go"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	run := exec.Command("go", "run", "./cmd")
	run.Dir = dir
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("polymorphic SetChildKey must set the discriminator: %v\n%s", err, out)
	}
}
