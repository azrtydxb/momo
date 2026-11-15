// SPDX-License-Identifier: Apache-2.0

package schema

import "testing"

func legacyFullText(ft *FullText, parts ...IndexPart) *Schema {
	return &Schema{Dialect: "mysql", Tables: []Table{{
		Name:    "t",
		Columns: []Column{{Name: "a", Type: "text"}, {Name: "b", Type: "text"}},
		Indexes: []Index{{Name: "t_ft", Method: "fulltext", Parts: parts, FullText: ft}},
	}}}
}

// Catches: a FULLTEXT index declared before the description existed comparing
// unequal to the same index with its description, which is what inspection
// reports, and an index with an expression part being given one.
func TestLegacyFullTextEqualsDescribed(t *testing.T) {
	parts := []IndexPart{{Column: "a"}, {Column: "b"}}
	old := legacyFullText(nil, parts...)
	described := legacyFullText(&FullText{Columns: []string{"a", "b"}}, parts...)
	if !Equal(old, described) {
		t.Fatal("a legacy FULLTEXT declaration differs from its described form")
	}
	other := legacyFullText(&FullText{Columns: []string{"b", "a"}}, parts...)
	if Equal(old, other) {
		t.Fatal("a different column order compared equal")
	}
	expr := legacyFullText(nil, IndexPart{Expr: "lower(a)"})
	if got := Normalized(expr).Tables[0].Indexes[0].FullText; got != nil {
		t.Fatalf("an expression part was described: %+v", got)
	}
}
