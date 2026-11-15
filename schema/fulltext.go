// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

// FullText describes a full-text search declaration on an index whose Method
// is "fulltext". The snapshot records only the declaration; every dialect
// derives its own structures (generated tsvector column, FTS5 table and
// triggers, FULLTEXT index) from it.
type FullText struct {
	Columns    []string `json:"columns"`              // source columns, in order
	Config     string   `json:"config,omitempty"`     // PostgreSQL regconfig
	Weights    []string `json:"weights,omitempty"`    // "A".."D", one per column
	Tokenizer  string   `json:"tokenizer,omitempty"`  // SQLite FTS5 tokenize clause
	Parser     string   `json:"parser,omitempty"`     // MySQL WITH PARSER
	Expression bool     `json:"expression,omitempty"` // PostgreSQL: expression index, no generated column
}

var (
	ftsConfigRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	ftsTokenizerRE = regexp.MustCompile(`^(porter )?(unicode61|ascii|trigram)( remove_diacritics [0-2])?$`)
)

// FTS5Table is the name of the FTS5 virtual table that indexes table.
func FTS5Table(table string) string { return table + "_fts" }

// TSVectorColumn is the name of the generated tsvector column of the
// full-text index named index.
func TSVectorColumn(index string) string { return index + "_tsv" }

// Validate checks the description against the allowlists that keep a
// configuration, tokenizer or parser name from reaching rendered SQL.
func (f FullText) Validate() error {
	if len(f.Columns) == 0 {
		return errors.New("a full-text declaration needs at least one column")
	}
	seen := map[string]bool{}
	for _, c := range f.Columns {
		if c == "" {
			return errors.New("a full-text declaration has an empty column name")
		}
		if seen[c] {
			return fmt.Errorf("full-text column %q is repeated", c)
		}
		seen[c] = true
	}
	if f.Config != "" && !ftsConfigRE.MatchString(f.Config) {
		return fmt.Errorf("full-text config %q is not a plain identifier", f.Config)
	}
	if len(f.Weights) > 0 {
		if len(f.Weights) != len(f.Columns) {
			return fmt.Errorf("full-text weights: got %d, want one per column (%d)", len(f.Weights), len(f.Columns))
		}
		for _, w := range f.Weights {
			if !slices.Contains([]string{"A", "B", "C", "D"}, w) {
				return fmt.Errorf("full-text weight %q is not one of A, B, C, D", w)
			}
		}
		if f.Expression {
			return errors.New("full-text weights cannot be combined with an expression index")
		}
	}
	if f.Tokenizer != "" && !ftsTokenizerRE.MatchString(f.Tokenizer) {
		return fmt.Errorf("full-text tokenizer %q is not supported", f.Tokenizer)
	}
	if f.Parser != "" && f.Parser != "ngram" && f.Parser != "mecab" {
		return fmt.Errorf("full-text parser %q is not ngram or mecab", f.Parser)
	}
	return nil
}

// ForDialect returns f with the options the dialect ignores cleared, so a
// change to an option that has no effect on d is no migration there.
func (f FullText) ForDialect(d dialect.Name) FullText {
	switch d {
	case dialect.SQLite:
		f.Config, f.Parser, f.Weights = "", "", nil
	case dialect.MySQL:
		f.Config, f.Tokenizer, f.Weights = "", "", nil
	case dialect.Postgres:
		f.Tokenizer, f.Parser = "", ""
	}
	return f
}

// FullTextIndex returns the table's full-text index, if it declares one.
func (t *Table) FullTextIndex() (*Index, bool) {
	for i := range t.Indexes {
		if t.Indexes[i].Method == "fulltext" && t.Indexes[i].FullText != nil {
			return &t.Indexes[i], true
		}
	}
	return nil, false
}
