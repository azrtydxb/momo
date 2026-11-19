// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Catches: an inspected exclusion constraint dropped on the way to the models
// (the init migration then accepts overlapping rows), a range column written as
// a string or given a type tag when its Go type already maps to it, a method
// other than gist, or a deferral not carried.
func TestGenerateFromDBExclusionConstraintsAndRanges(t *testing.T) {
	bookings := schema.Table{
		Name: "bookings",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "room_id", Type: "integer"},
			{Name: "during", Type: "tstzrange"},
			{Name: "local", Type: "tsrange"},
			{Name: "days", Type: "daterange", Nullable: true},
			{Name: "qty", Type: "int4range"},
			{Name: "big", Type: "int8range"},
			{Name: "price", Type: "numrange"},
		},
		PrimaryKey: []string{"id"},
		Excludes: []schema.Exclude{
			{Name: "bookings_no_overlap", Method: "gist", Elements: []schema.ExcludeElement{{Column: "room_id", Operator: "="}, {Column: "during", Operator: "&&"}}, Deferrable: true, InitiallyDeferred: true},
			{Name: "bookings_days_excl", Method: "spgist", Elements: []schema.ExcludeElement{{Column: "days", Operator: "&&"}}},
		},
	}
	s := &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Extensions:    []schema.Extension{{Name: "btree_gist"}},
		Tables:        []schema.Table{bookings},
	}
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustContain(t, "bookings model", files["bookings"],
		`c.Exclude("bookings_no_overlap", &m.RoomID, "=", &m.During, "&&").InitiallyDeferred()`,
		`c.Exclude("bookings_days_excl", &m.Days, "&&").Using("spgist")`,
	)
	mustMatch(t, "bookings model", files["bookings"],
		`During\s+types\.Range\[time\.Time\]`,
		`Local\s+types\.Range\[types\.Timestamp\]`,
		`Qty\s+types\.Range\[int32\]`,
		`Big\s+types\.Range\[int64\]`,
		`Price\s+types\.Range\[types\.Numeric\]`,
	)
	if strings.Contains(files["bookings"], "type:") {
		t.Errorf("a range column was given a type tag:\n%s", files["bookings"])
	}
	if strings.Contains(files["bookings"], `Using("gist")`) {
		t.Errorf("the default access method was written out:\n%s", files["bookings"])
	}
}
