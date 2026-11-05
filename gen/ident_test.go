// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateNameCollisions(t *testing.T) {
	if got := Identifier("Table", DescriptorReserved()); got != "Table_" {
		t.Fatalf("Table -> %q", got)
	}
	if got := Identifier("Name", DescriptorReserved()); got != "Name" {
		t.Fatalf("Name -> %q", got)
	}
	if got := SubPkgName("Type", "models"); got != "type_" {
		t.Fatalf("Type subpackage -> %q", got)
	}
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./collide"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Models) == 0 {
		t.Fatal("collide fixture produced no models")
	}
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.String() + "\n")
	}
	if !strings.Contains(b.String(), `column "user_id" is produced by fields UserId and UserID`) {
		t.Fatalf("column collision not reported:\n%s", b.String())
	}
}

func TestIdentifierReservedWords(t *testing.T) {
	keywords := []string{"break", "default", "func", "interface", "select", "case", "defer", "go", "map", "struct",
		"chan", "else", "goto", "package", "switch", "const", "fallthrough", "if", "range", "type",
		"continue", "for", "import", "return", "var"}
	if len(keywords) != 25 {
		t.Fatalf("keyword list has %d entries", len(keywords))
	}
	r := DescriptorReserved()
	for _, k := range keywords {
		if !r[k] {
			t.Errorf("keyword %q not reserved", k)
		}
		if got := Identifier(k, r); got != k+"_" {
			t.Errorf("Identifier(%q) = %q", k, got)
		}
		if got := SubPkgName(strings.ToUpper(k[:1])+k[1:], "models"); got != k+"_" {
			t.Errorf("SubPkgName(%q) = %q", k, got)
		}
	}
	for _, n := range []string{"any", "error", "string", "len", "int", "nil", "true", "iota", "append", "comparable",
		"Table", "As", "Columns", "ColumnByName", "ColumnNames", "base", "fmt", "momo", "time", "vector", "types"} {
		if !r[n] {
			t.Errorf("%q not reserved", n)
		}
		if got := Identifier(n, r); got != n+"_" {
			t.Errorf("Identifier(%q) = %q", n, got)
		}
	}
	for _, n := range []string{"any", "error", "string", "len", "nil"} {
		if got := SubPkgName(n, "models"); got != n+"_" {
			t.Errorf("SubPkgName(%q) = %q", n, got)
		}
	}
}

func TestIdentifierEdgeCases(t *testing.T) {
	// Case differences are distinct Go identifiers.
	r := map[string]bool{"Name": true}
	if got := Identifier("name", r); got != "name" {
		t.Errorf("case-different name -> %q", got)
	}
	r = map[string]bool{"X": true, "X_": true, "X__": true}
	if got := Identifier("X", r); got != "X___" {
		t.Errorf("chained -> %q", got)
	}
	// Names identical after naming conversion: the caller records each result.
	set := map[string]bool{}
	var out []string
	for _, n := range []string{"UserID", "UserID", "UserID"} {
		id := Identifier(n, set)
		set[id] = true
		out = append(out, id)
	}
	if strings.Join(out, ",") != "UserID,UserID_,UserID__" {
		t.Errorf("sequence -> %v", out)
	}
	if got := Identifier("Größe", DescriptorReserved()); got != "Größe" {
		t.Errorf("unicode -> %q", got)
	}
	if got := SubPkgName("Ünï", "models"); got != "ünï" {
		t.Errorf("unicode subpkg -> %q", got)
	}
	if got := Identifier("", DescriptorReserved()); got != "" {
		t.Errorf("empty -> %q", got)
	}
	if got := Identifier("a", nil); got != "a" {
		t.Errorf("nil set -> %q", got)
	}
	if got := SubPkgName("Models", "models"); got != "models_" {
		t.Errorf("same as model pkg -> %q", got)
	}
	if got := SubPkgName("User", "models"); got != "user" {
		t.Errorf("plain -> %q", got)
	}
	if got := SubPkgName("String", "models"); got != "string_" {
		t.Errorf("predeclared -> %q", got)
	}
	if got := SubPkgName("", "models"); got != "" {
		t.Errorf("empty subpkg -> %q", got)
	}
}

func TestModelIdentifierCollisions(t *testing.T) {
	pos := func(l int) token.Position { return token.Position{Filename: "m.go", Line: l, Column: 6} }
	models := []*Model{
		{GoName: "User", PkgPath: "p", PkgName: "p", Pos: pos(1)},
		{GoName: "user", PkgPath: "p", PkgName: "p", Pos: pos(2)},
		{GoName: "User", PkgPath: "q", PkgName: "q", Pos: pos(3)},
		{GoName: "Other", PkgPath: "p", PkgName: "p", Pos: pos(4)},
	}
	diags := checkModelIdentifiers(models)
	if len(diags) != 1 {
		t.Fatalf("diags = %v", diags)
	}
	if diags[0].Msg != "model identifiers collide in package p: User, user" || diags[0].Pos.Line != 2 {
		t.Fatalf("diag = %v", diags[0])
	}
	if got := checkModelIdentifiers(nil); len(got) != 0 {
		t.Fatalf("nil -> %v", got)
	}
}

func TestColumnCollisions(t *testing.T) {
	pos := func(l int) token.Position { return token.Position{Filename: "m.go", Line: l, Column: 2} }
	cols := []columnName{
		{Field: "UserID", Column: "user_id", Pos: pos(1)},
		{Field: "Name", Column: "name", Pos: pos(2)},
		{Field: "UserId", Column: "user_id", Pos: pos(3)},
	}
	diags := checkColumnCollisions(cols)
	if len(diags) != 1 || diags[0].Msg != `column "user_id" is produced by fields UserId and UserID` || diags[0].Pos.Line != 3 {
		t.Fatalf("diags = %v", diags)
	}
	if got := checkColumnCollisions(cols[:2]); len(got) != 0 {
		t.Fatalf("unexpected %v", got)
	}
}
