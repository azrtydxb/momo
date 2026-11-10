// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// TestCompanionForeignKey is issue 54: Config.ForeignKey declares two- and
// three-column keys into a unique constraint, with referential actions.
func TestCompanionForeignKey(t *testing.T) {
	g, diags, err := Load([]string{"./companionfk"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, diags := BuildSchema(g.Models, pg())
	if len(diags) != 0 {
		t.Fatalf("schema diagnostics: %v", diags)
	}
	note, _ := s.Table("note")
	want := schema.ForeignKey{
		Name: "note_document_fk", Columns: []string{"company_id", "document_id"}, RefTable: "document",
		RefColumns: []string{"company_id", "id"}, OnDelete: "CASCADE", OnUpdate: "RESTRICT",
	}
	if len(note.ForeignKeys) != 1 || !fkEqual(note.ForeignKeys[0], want) {
		t.Fatalf("note foreign keys = %+v", note.ForeignKeys)
	}
	rev, _ := s.Table("revision")
	if len(rev.ForeignKeys) != 1 || !slices.Equal(rev.ForeignKeys[0].Columns, []string{"company_id", "template_id", "version_no"}) ||
		!slices.Equal(rev.ForeignKeys[0].RefColumns, []string{"company_id", "template_id", "version_no"}) ||
		rev.ForeignKeys[0].OnDelete != "" {
		t.Fatalf("revision foreign keys = %+v", rev.ForeignKeys)
	}
}

func fkEqual(a, b schema.ForeignKey) bool {
	return a.Name == b.Name && slices.Equal(a.Columns, b.Columns) && a.RefTable == b.RefTable &&
		slices.Equal(a.RefColumns, b.RefColumns) && a.OnDelete == b.OnDelete && a.OnUpdate == b.OnUpdate
}

// TestCompanionForeignKeyRefusals: a key that does not reference a unique
// key, an unknown table or an unknown column is a diagnostic, not a schema.
func TestCompanionForeignKeyRefusals(t *testing.T) {
	// Load builds the schema to validate the companion, so the refusals are
	// its diagnostics and no model survives.
	g, diags, err := Load([]string{"./companionfkbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	for _, want := range []string{
		`foreign key "child_not_unique_fk": (company_id, id) is neither the primary key of parent`,
		`foreign key "child_ghost_fk": the referenced table "ghost" is not the table of any model`,
		`foreign key "child_col_fk": column "nope" is not a column of table parent`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing diagnostic %q in:\n%s", want, joined)
		}
	}
	if len(g.Models) != 0 {
		t.Errorf("refused keys must not produce models: %d", len(g.Models))
	}
}
