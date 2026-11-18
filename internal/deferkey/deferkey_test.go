// SPDX-License-Identifier: Apache-2.0

package deferkey

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Catches: a refusal that fires when a non-deferrable key covers the same
// columns (PostgreSQL uses that key), and one that misses a deferrable key
// whose columns are listed in another order.
func TestCovering(t *testing.T) {
	cases := []struct {
		name string
		tbl  schema.Table
		cols []string
		want string
	}{
		{"plain primary key", schema.Table{PrimaryKey: []string{"id"}}, []string{"id"}, ""},
		{"deferrable primary key", schema.Table{PrimaryKey: []string{"id"}, PrimaryKeyDeferrable: true}, []string{"id"}, "the primary key"},
		{"initially deferred primary key", schema.Table{PrimaryKey: []string{"id"}, PrimaryKeyInitiallyDeferred: true}, []string{"id"}, "the primary key"},
		{"deferrable unique, other order", schema.Table{Uniques: []schema.Unique{{Name: "u", Columns: []string{"a", "b"}, Deferrable: true}}}, []string{"b", "a"}, "unique u"},
		{"other columns", schema.Table{Uniques: []schema.Unique{{Name: "u", Columns: []string{"a"}, Deferrable: true}}}, []string{"b"}, ""},
		{"covered by a plain unique too", schema.Table{Uniques: []schema.Unique{{Name: "u", Columns: []string{"a"}, Deferrable: true}, {Name: "v", Columns: []string{"a"}}}}, []string{"a"}, ""},
		{"covered by a unique index too", schema.Table{
			Uniques: []schema.Unique{{Name: "u", Columns: []string{"a"}, Deferrable: true}},
			Indexes: []schema.Index{{Name: "i", Unique: true, Parts: []schema.IndexPart{{Column: "a"}}}},
		}, []string{"a"}, ""},
		{"partial unique index does not cover", schema.Table{
			Uniques: []schema.Unique{{Name: "u", Columns: []string{"a"}, Deferrable: true}},
			Indexes: []schema.Index{{Name: "i", Unique: true, Where: "a > 0", Parts: []schema.IndexPart{{Column: "a"}}}},
		}, []string{"a"}, "unique u"},
	}
	for _, c := range cases {
		if got := Covering(&c.tbl, c.cols); got != c.want {
			t.Errorf("%s: Covering = %q, want %q", c.name, got, c.want)
		}
	}
}
