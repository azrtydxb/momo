// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"
)

// Catches: a declared foreign key name being replaced by the derived one, so
// the partition's _fkey1 constraint would show as a change on baseline.
func TestBelongsToForeignKeyName(t *testing.T) {
	doc, author := relModels()
	author.GoName, author.Table = "Author", "authors"
	doc.Relations = []*Relation{{GoName: "Author", Kind: BelongsTo, Target: author, FK: "AuthorID", FKName: "documents_author_id_fkey1"}}
	s, diags := build(t, pg(), doc, author)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	if tb, _ := s.Table("documents"); tb.ForeignKeys[0].Name != "documents_author_id_fkey1" {
		t.Fatalf("tag name: %+v", tb.ForeignKeys)
	}
	doc.Relations[0].FKName = ""
	doc.Config.FKNames = map[string]string{"author_id": "cfg_fkey"}
	s, _ = build(t, pg(), doc, author)
	if tb, _ := s.Table("documents"); tb.ForeignKeys[0].Name != "cfg_fkey" {
		t.Fatalf("config name: %+v", tb.ForeignKeys)
	}
	doc.Relations[0].FKName = "tag_fkey"
	_, diags = build(t, pg(), doc, author)
	if !strings.Contains(sdiagsString(diags), "declared by both the fk_name tag and MomoConfig") {
		t.Fatalf("both ways must be refused: %v", diags)
	}
}
