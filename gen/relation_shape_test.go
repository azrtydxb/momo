// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/internal/golden"
)

// literalOf returns the text of `var <name> = momo.Relation[...]{...}` in src.
func literalOf(t *testing.T, src []byte, name string) string {
	t.Helper()
	s := string(src)
	start := strings.Index(s, "var "+name+" = momo.Relation[")
	if start < 0 {
		t.Fatalf("no descriptor %s in:\n%s", name, s)
	}
	end := strings.Index(s[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("descriptor %s is not closed", name)
	}
	return s[start : start+end+3]
}

// normalizeSpace collapses every run of blanks to one space, so gofmt's
// column alignment does not matter to the label checks.
func normalizeSpace(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.Join(lines, "\n")
}

// labelKey anchors a descriptor field label at the start of a line, so
// "Name:" does not match the Name inside a TableRef and "ParentKey:" does not
// match inside "SetParentKey:". A statement label (c.X = ...) matches anywhere.
func labelKey(l string) string {
	if strings.HasPrefix(l, "c.") {
		return l
	}
	return "\n" + l
}

func loadGraph(t *testing.T, root, pattern string) *Graph {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{pattern}, Config{Dir: abs})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	return g
}

type shapeCase struct {
	root, pattern, model, relation string
	labels, banned                 []string
}

var commonLabels = []string{
	"Name:", "Parent: momo.TableRef{", "Child: momo.TableRef{", "ParentMeta:", "ChildMeta:",
	"ParentCols:", "ChildCols:", "ChildPK:", "ParentKey:", "ChildKey:",
}
var commonBanned = []string{"Assign:", "ParentColumns:", "ChildColumns:", "JoinSpec"}

func with(extra ...string) []string { return append(append([]string{}, commonLabels...), extra...) }
func ban(extra ...string) []string  { return append(append([]string{}, commonBanned...), extra...) }

func TestRelationDescriptorShape(t *testing.T) {
	cases := []shapeCase{
		{"testdata/fixtures", "./rel", "Author", "Posts",
			with("Kind: momo.HasMany", "SetChildKey:", "GetMany:", "SetMany:"),
			ban("SetParentKey:", "GetOne:", "SetOne:", "Through:", "Discriminator:")},
		{"testdata/fixtures", "./rel", "Post", "Author",
			with("Kind: momo.BelongsTo", "SetParentKey:", "GetOne:", "SetOne:"),
			ban("SetChildKey:", "GetMany:", "SetMany:", "Through:", "Discriminator:")},
		{"../internal/testmodels/example", "./", "Document", "Tags",
			with("Kind: momo.ManyToMany", "Through: &momo.Through{", "GetMany:", "SetMany:"),
			ban("SetChildKey:", "SetParentKey:", "GetOne:", "SetOne:", "Discriminator:")},
		{"testdata/fixtures", "./poly", "Document", "Images",
			with("Kind: momo.HasMany", "SetChildKey:", "GetMany:", "SetMany:",
				"Discriminator: &momo.Discriminator{Column: \"owner_type\", Value: \"document\"},",
				"c.OwnerType = \"document\""),
			ban("SetParentKey:", "GetOne:", "SetOne:", "Through:")},
	}
	for _, c := range cases {
		t.Run(c.model+"."+c.relation, func(t *testing.T) {
			g := loadGraph(t, c.root, c.pattern)
			src, ds := EmitDescriptors(findModel(t, g, c.model), g)
			if len(ds) != 0 {
				t.Fatalf("diagnostics: %v", ds)
			}
			lit := normalizeSpace(literalOf(t, src, c.relation))
			for _, l := range c.labels {
				if n := strings.Count(lit, labelKey(l)); n != 1 {
					t.Fatalf("%q appears %d times, want 1:\n%s", l, n, lit)
				}
			}
			for _, b := range c.banned {
				if strings.Contains(lit, b) {
					t.Fatalf("%q must not appear:\n%s", b, lit)
				}
			}
		})
	}
}

func TestPolymorphicDescriptorGolden(t *testing.T) {
	g := loadGraph(t, "testdata/fixtures", "./poly")
	src, ds := EmitDescriptors(findModel(t, g, "Document"), g)
	if len(ds) != 0 {
		t.Fatalf("diagnostics: %v", ds)
	}
	ok, err := golden.Diff("testdata/golden/poly/document_descriptors.go", src, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("polymorphic descriptors differ from golden; review and rerun with -update")
	}
}

func TestPolymorphicDiagnostics(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	_, diags, err := Load([]string{"./polybad"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(dir, "polybad", "bad.go"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	want := map[string]string{
		"// BAD-1": "polymorphic relation Images needs both polytype and polyvalue",
		"// BAD-2": "polymorphic is only valid for has_one and has_many",
		"// BAD-3": "polymorphic column owner_type is not a field of Note",
	}
	for marker, msg := range want {
		line := 0
		for i, l := range lines {
			if strings.Contains(l, marker) {
				line = i + 1
			}
		}
		if line == 0 {
			t.Fatalf("fixture lacks %s", marker)
		}
		pos := fmt.Sprintf("bad.go:%d:", line)
		found := false
		for _, d := range diags {
			if strings.Contains(d.String(), pos) && strings.Contains(d.String(), msg) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no diagnostic %q at %s; got %v", msg, pos, diags)
		}
	}
}
