// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"go/token"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// TestViewModelDeclaresNoTable checks that a model embedding momo.View maps
// an existing view: the schema gets the view its companion declares and no
// table of the same name, so a migration never issues CREATE TABLE for it.
func TestViewModelDeclaresNoTable(t *testing.T) {
	g, diags, err := Load([]string{"./views"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diagnostics: %v", sdiags)
	}
	if _, ok := s.Table("accounts"); !ok {
		t.Fatal("the table model lost its table")
	}
	if _, ok := s.Table("active_accounts"); ok {
		t.Fatal("the view model produced a table named active_accounts")
	}
	if len(s.Tables) != 1 {
		t.Fatalf("tables = %d, want 1", len(s.Tables))
	}
	if len(s.Views) != 1 || s.Views[0].Name != "active_accounts" {
		t.Fatalf("views = %+v, want active_accounts", s.Views)
	}
}

// viewModel is a model that maps the view named table.
func viewModel(goName, table string) *Model {
	m := mdl(goName, table, fld("ID", "int64", "pk"))
	m.IsView = true
	return m
}

// TestViewModelSharingATableNameIsDiagnosed checks that a view model may not
// share its table name with a table model or with another view model, as two
// table models may not.
func TestViewModelSharingATableNameIsDiagnosed(t *testing.T) {
	tbl := mdl("Thing", "things", fld("ID", "int64", "pk"))
	_, diags := build(t, pg(), tbl, viewModel("ThingView", "things"))
	if !strings.Contains(sdiagsString(diags), `table "things" is produced by models Thing and ThingView`) {
		t.Fatalf("view and table: %v", diags)
	}
	_, diags = build(t, pg(), viewModel("AView", "v"), viewModel("BView", "v"))
	if !strings.Contains(sdiagsString(diags), `table "v" is produced by models AView and BView`) {
		t.Fatalf("two views: %v", diags)
	}
	if _, diags = build(t, pg(), tbl, viewModel("ThingView", "thing_view")); len(diags) != 0 {
		t.Fatalf("distinct names must pass: %v", diags)
	}
}

// TestBelongsToViewModelAddsNoForeignKey checks that a belongs_to relation
// whose target is a view model creates no foreign key: a constraint cannot
// reference a view.
func TestBelongsToViewModelAddsNoForeignKey(t *testing.T) {
	doc, author := relModels()
	author.GoName, author.Table = "Author", "authors"
	author.Fields[0].GoType = "int64"
	author.IsView = true
	doc.Relations = []*Relation{{GoName: "Author", Kind: BelongsTo, Target: author, FK: "AuthorID",
		Pos: token.Position{Filename: "m.go", Line: 40, Column: 2}}}
	s, diags := build(t, pg(), doc, author)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	tb, ok := s.Table("documents")
	if !ok {
		t.Fatal("documents table missing")
	}
	if len(tb.ForeignKeys) != 0 {
		t.Fatalf("a belongs_to to a view model must add no foreign key: %+v", tb.ForeignKeys)
	}
}
