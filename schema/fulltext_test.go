// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// Catches: a FullText field lost by the JSON snapshot, and an old snapshot
// (no "fulltext" key) failing to load.
func TestSchemaFullTextSnapshotCompat(t *testing.T) {
	in := &Schema{FormatVersion: FormatVersion, Dialect: "postgres", Tables: []Table{{
		Name:    "articles",
		Columns: []Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: "text"}, {Name: "body", Type: "text"}},
		Indexes: []Index{{
			Name: "articles_search", Method: "fulltext",
			Parts: []IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &FullText{Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"},
				Tokenizer: "porter unicode61", Parser: "ngram", Expression: true},
		}},
	}}}
	var buf bytes.Buffer
	if err := in.Write(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	got := out.Tables[0].Indexes[0].FullText
	if got == nil || got.Config != "english" || got.Weights[1] != "B" || got.Tokenizer != "porter unicode61" || got.Parser != "ngram" || !got.Expression {
		t.Fatalf("full-text description lost in the round trip: %+v", got)
	}
	old := `{"format_version":1,"dialect":"postgres","tables":[{"name":"t","columns":[{"name":"id","type":"bigint"}],"indexes":[{"name":"i","parts":[{"column":"id"}]}]}]}`
	if _, err := Load(strings.NewReader(old)); err != nil {
		t.Fatalf("snapshot written before this change: %v", err)
	}
	if !json.Valid([]byte(old)) {
		t.Fatal("fixture is not JSON")
	}
}

// Catches: a hostile configuration, tokenizer or parser reaching rendered SQL.
func TestFullTextValidate(t *testing.T) {
	ok := FullText{Columns: []string{"a"}, Config: "english", Tokenizer: "porter unicode61", Parser: "ngram"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid description: %v", err)
	}
	bad := []FullText{
		{},
		{Columns: []string{"a", "a"}},
		{Columns: []string{"a"}, Config: "english'); DROP TABLE x;--"},
		{Columns: []string{"a"}, Tokenizer: "unicode61'); DROP TABLE x;--"}, //nolint:gosec // hostile tokenizer name, not a credential
		{Columns: []string{"a"}, Parser: "ngram; x"},
		{Columns: []string{"a"}, Weights: []string{"A", "B"}},
		{Columns: []string{"a"}, Weights: []string{"E"}},
		{Columns: []string{"a"}, Weights: []string{"A"}, Expression: true},
		{Columns: []string{"a"}, Tokenizer: "ascii remove_diacritics 1"},          //nolint:gosec // invalid tokenizer, not a credential
		{Columns: []string{"a"}, Tokenizer: "porter trigram remove_diacritics 2"}, //nolint:gosec // invalid tokenizer, not a credential
	}
	for i, f := range bad {
		if err := f.Validate(); err == nil {
			t.Errorf("case %d accepted: %+v", i, f)
		}
	}
	if FTS5Table("articles") != "articles_fts" || TSVectorColumn("articles_search") != "articles_search_tsv" {
		t.Fatal("name functions drifted")
	}
}

// Catches: an option a dialect ignores (including the PostgreSQL-only
// Expression) surviving ForDialect, so a change to it would diff forever.
func TestFullTextForDialect(t *testing.T) {
	all := FullText{Columns: []string{"a"}, Config: "english", Weights: []string{"A"},
		Tokenizer: "porter unicode61", Parser: "ngram", Expression: false}
	expr := FullText{Columns: []string{"a"}, Config: "english", Expression: true}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if got := expr.ForDialect(d); got.Expression || got.Config != "" {
			t.Errorf("%s kept PostgreSQL-only options: %+v", d, got)
		}
	}
	if got := expr.ForDialect(dialect.Postgres); !got.Expression || got.Config != "english" {
		t.Errorf("postgres lost its options: %+v", got)
	}
	if got := all.ForDialect(dialect.SQLite); got.Tokenizer != "porter unicode61" || got.Parser != "" || got.Weights != nil {
		t.Errorf("sqlite = %+v", got)
	}
	if got := all.ForDialect(dialect.MySQL); got.Parser != "ngram" || got.Tokenizer != "" || got.Weights != nil {
		t.Errorf("mysql = %+v", got)
	}
	if got := all.ForDialect(dialect.Postgres); got.Tokenizer != "" || got.Parser != "" || got.Config != "english" {
		t.Errorf("postgres = %+v", got)
	}
}
