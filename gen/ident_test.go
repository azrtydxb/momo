// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateNameCollisions(t *testing.T) {
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
