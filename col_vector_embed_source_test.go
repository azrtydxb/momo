// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql"
	"testing"
)

type embedTitle string

// Catches: a source field type that the generator accepts being read as NULL
// or as an error, which would hash every row alike or fail the whole run.
func TestEmbeddingSourceValue(t *testing.T) {
	text := "text"
	var nilText *string
	named := embedTitle("named")
	cases := []struct {
		name string
		in   any
		want sql.NullString
	}{
		{"nil", nil, sql.NullString{}},
		{"string", "text", sql.NullString{String: "text", Valid: true}},
		{"empty", "", sql.NullString{Valid: true}},
		{"pointer", &text, sql.NullString{String: "text", Valid: true}},
		{"nil pointer", nilText, sql.NullString{}},
		{"named", named, sql.NullString{String: "named", Valid: true}},
		{"null string", sql.NullString{String: "x", Valid: true}, sql.NullString{String: "x", Valid: true}},
		{"null null", sql.NullString{}, sql.NullString{}},
		{"generic null", sql.Null[string]{V: "g", Valid: true}, sql.NullString{String: "g", Valid: true}},
	}
	for _, c := range cases {
		got, err := sourceValue(c.in)
		if err != nil || got != c.want {
			t.Errorf("%s: sourceValue(%v) = %+v, %v; want %+v", c.name, c.in, got, err, c.want)
		}
	}
	for _, bad := range []any{42, 1.5, []string{"a"}, sql.NullInt64{Int64: 1, Valid: true}} {
		if _, err := sourceValue(bad); err == nil {
			t.Errorf("sourceValue(%T) succeeded, want an error", bad)
		}
	}
}
