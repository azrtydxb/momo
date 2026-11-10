// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Catches: fk_name or FKName being accepted where no constraint is created, a
// name PostgreSQL would truncate, or two foreign keys of a table sharing a name.
func TestForeignKeyNameDiagnostics(t *testing.T) {
	newPair := func() (*Model, *Model) {
		doc, author := relModels()
		author.GoName, author.Table = "Author", "authors"
		return doc, author
	}
	t.Run("has_many", func(t *testing.T) {
		doc, author := newPair()
		doc.Relations = []*Relation{{GoName: "Authors", Kind: HasMany, Target: author, FK: "AuthorID", FKName: "x_fkey"}}
		_, diags := build(t, pg(), doc, author)
		if !strings.Contains(sdiagsString(diags), "fk_name applies to belongs_to only") {
			t.Fatalf("diags: %v", diags)
		}
	})
	t.Run("config without belongs_to", func(t *testing.T) {
		doc, author := newPair()
		doc.Config.FKNames = map[string]string{"author_id": "x_fkey"}
		_, diags := build(t, pg(), doc, author)
		if !strings.Contains(sdiagsString(diags), `FKName on column "author_id"`) {
			t.Fatalf("diags: %v", diags)
		}
	})
	t.Run("too long", func(t *testing.T) {
		doc, author := newPair()
		doc.Relations = []*Relation{{GoName: "Author", Kind: BelongsTo, Target: author, FK: "AuthorID", FKName: strings.Repeat("a", 64)}}
		_, diags := build(t, pg(), doc, author)
		if !strings.Contains(sdiagsString(diags), "is longer than 63 bytes") {
			t.Fatalf("diags: %v", diags)
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		doc, author := newPair()
		doc.Relations = []*Relation{
			{GoName: "Author", Kind: BelongsTo, Target: author, FK: "AuthorID", FKName: "same_fkey"},
			{GoName: "Editor", Kind: BelongsTo, Target: author, FK: "AuthorID", FKName: "same_fkey"},
		}
		_, diags := build(t, pg(), doc, author)
		if !strings.Contains(sdiagsString(diags), "already used by another foreign key") {
			t.Fatalf("diags: %v", diags)
		}
	})
}

// Catches: a composite Config.ForeignKey sharing a name with a belongs_to
// fk_name, or exceeding 63 bytes, being accepted; PostgreSQL would reject or
// truncate the second constraint.
func TestCompositeForeignKeyNameClashes(t *testing.T) {
	newPair := func() (*Model, *Model) {
		doc, author := relModels()
		author.GoName, author.Table = "Author", "authors"
		doc.Relations = []*Relation{{GoName: "Author", Kind: BelongsTo, Target: author, FK: "AuthorID", FKName: "clash_fkey"}}
		return doc, author
	}
	t.Run("clash", func(t *testing.T) {
		doc, author := newPair()
		doc.Config.ForeignKeys = []schema.ForeignKey{{Name: "clash_fkey", Columns: []string{"author_id"}, RefTable: "authors", RefColumns: []string{"id"}}}
		_, diags := build(t, pg(), doc, author)
		if !strings.Contains(sdiagsString(diags), `foreign key "clash_fkey": the name is already used by another foreign key`) {
			t.Fatalf("diags: %v", diags)
		}
	})
	t.Run("too long", func(t *testing.T) {
		doc, author := newPair()
		doc.Config.ForeignKeys = []schema.ForeignKey{{Name: strings.Repeat("b", 64), Columns: []string{"author_id"}, RefTable: "authors", RefColumns: []string{"id"}}}
		_, diags := build(t, pg(), doc, author)
		if !strings.Contains(sdiagsString(diags), "the name is longer than 63 bytes") {
			t.Fatalf("diags: %v", diags)
		}
	})
}
