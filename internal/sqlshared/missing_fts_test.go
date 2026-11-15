// SPDX-License-Identifier: Apache-2.0

package sqlshared

import "testing"

// Catches: a missing FTS5 table not recognised, and an ordinary missing table
// or an unrelated message reported as a missing search structure.
func TestMissingFTSTable(t *testing.T) {
	cases := []struct {
		msg  string
		name string
		ok   bool
	}{
		{"no such table: articles_fts", "articles_fts", true},
		{"no such table: main.articles_fts", "articles_fts", true},
		{"no such table: articles", "", false},
		{"no such column: articles_fts", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		name, ok := MissingFTSTable(c.msg)
		if name != c.name || ok != c.ok {
			t.Errorf("MissingFTSTable(%q) = %q, %v; want %q, %v", c.msg, name, ok, c.name, c.ok)
		}
	}
}
