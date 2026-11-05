// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql"
	"testing"
)

// TestHasNilPartMatchesKeyStringNull pins that every component keyString
// encodes as NULL ("n:") also counts as no key, so such a parent never adds a
// never-matching tuple to the IN list.
func TestHasNilPartMatchesKeyStringNull(t *testing.T) {
	var nilPtr *int64
	one := int64(1)
	var nilValuer *sql.NullInt64
	cases := []struct {
		name string
		part any
		null bool
	}{
		{"untyped nil", nil, true},
		{"nil pointer", nilPtr, true},
		{"invalid NullInt64", sql.NullInt64{}, true},
		{"nil *NullInt64", nilValuer, true},
		{"pointer to invalid NullInt64", &sql.NullInt64{}, true},
		{"valid NullInt64", sql.NullInt64{Int64: 1, Valid: true}, false},
		{"pointer to value", &one, false},
		{"zero int", 0, false},
		{"empty string", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasNilPart([]any{int64(7), c.part}); got != c.null {
				t.Fatalf("hasNilPart(%#v) = %v, want %v", c.part, got, c.null)
			}
			if c.null != (keyString([]any{c.part}) == "n:") {
				t.Fatalf("keyString(%#v) = %q disagrees with null=%v", c.part, keyString([]any{c.part}), c.null)
			}
		})
	}
}
