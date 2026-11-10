// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"
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
