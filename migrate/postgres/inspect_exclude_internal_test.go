// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Catches: the text of pg_get_constraintdef read wrongly: a method, a column
// or an operator lost, a quoted column name that holds a comma or a space split
// at the wrong place, the deferral words read as part of the elements, or an
// operator that is not a plain one let into a statement.
func TestExcludeFromDef(t *testing.T) {
	cases := []struct {
		name       string
		def        string
		deferrable bool
		deferred   bool
		want       schema.Exclude
	}{
		{
			name: "gist over a scalar and a range",
			def:  `EXCLUDE USING gist (room_id WITH =, during WITH &&)`,
			want: schema.Exclude{Name: "x", Method: "gist", Elements: []schema.ExcludeElement{{Column: "room_id", Operator: "="}, {Column: "during", Operator: "&&"}}},
		},
		{
			name:       "deferrable and initially deferred come from the flags",
			def:        `EXCLUDE USING gist (during WITH &&) DEFERRABLE INITIALLY DEFERRED`,
			deferrable: true,
			deferred:   true,
			want:       schema.Exclude{Name: "x", Method: "gist", Elements: []schema.ExcludeElement{{Column: "during", Operator: "&&"}}, Deferrable: true, InitiallyDeferred: true},
		},
		{
			name: "another access method and a quoted column",
			def:  `EXCLUDE USING spgist ("Day, 1" WITH =, "say ""hi""" WITH -|-)`,
			want: schema.Exclude{Name: "x", Method: "spgist", Elements: []schema.ExcludeElement{{Column: "Day, 1", Operator: "="}, {Column: `say "hi"`, Operator: "-|-"}}},
		},
		{
			name: "a column named with",
			def:  `EXCLUDE USING gist ("with" WITH =)`,
			want: schema.Exclude{Name: "x", Method: "gist", Elements: []schema.ExcludeElement{{Column: "with", Operator: "="}}},
		},
	}
	for _, c := range cases {
		got, err := excludeFromDef("t", "x", c.def, c.deferrable, c.deferred)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}

// Catches: a constraint momo cannot model being read as one it can: a WHERE
// predicate, an expression, an operator class or ordering, an INCLUDE list or a
// storage parameter dropped silently, so the model, the snapshot and the
// database disagree without a report. Each is refused with the constraint's
// name and the table.
func TestExcludeFromDefRefuses(t *testing.T) {
	for name, def := range map[string]string{
		"where":         `EXCLUDE USING gist (room WITH =, during WITH &&) WHERE ((live))`,
		"expression":    `EXCLUDE USING gist (lower(name) WITH =)`,
		"opclass":       `EXCLUDE USING gist (room gist_int4_ops WITH =)`,
		"ordering":      `EXCLUDE USING btree (room DESC WITH =)`,
		"include":       `EXCLUDE USING gist (room WITH =) INCLUDE (name)`,
		"storage":       `EXCLUDE USING gist (room WITH =) WITH (fillfactor='70')`,
		"operator call": `EXCLUDE USING gist (room WITH OPERATOR(pg_catalog.=))`,
		"no operator":   `EXCLUDE USING gist (room)`,
		"unknown am":    `EXCLUDE USING brin (room WITH =)`,
		"not exclude":   `CHECK ((a > 0))`,
		"no elements":   `EXCLUDE USING gist ()`,
		"unterminated":  `EXCLUDE USING gist (room WITH =`,
	} {
		_, err := excludeFromDef("bookings", "bookings_x", def, false, false)
		if err == nil {
			t.Errorf("%s: %q was accepted", name, def)
			continue
		}
		for _, want := range []string{`"bookings_x"`, `"bookings"`, "exclusion constraint"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q does not mention %s", name, err, want)
			}
		}
	}
}

func TestClosingParenAndSplit(t *testing.T) {
	if got := closingParen(`(a, (b), ")")`); got != len(`(a, (b), ")")`)-1 {
		t.Errorf("closingParen skipped a quoted parenthesis wrongly: %d", got)
	}
	if got := closingParen("a)"); got != -1 {
		t.Errorf("closingParen of text with no opening = %d", got)
	}
	if got := splitTopLevel(`a WITH =, f(x, y) WITH =, "p,q" WITH &&`); len(got) != 3 {
		t.Errorf("splitTopLevel = %q, want 3 parts", got)
	}
}
