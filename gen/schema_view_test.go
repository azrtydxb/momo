// SPDX-License-Identifier: Apache-2.0

package gen

import (
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
