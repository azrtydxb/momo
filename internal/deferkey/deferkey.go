// SPDX-License-Identifier: Apache-2.0

// Package deferkey finds the deferrable key a foreign key would reference.
// PostgreSQL refuses a deferrable primary key or unique constraint as a foreign
// key target ("cannot use a deferrable unique constraint for referenced
// table"), so the generator and migrate.Diff both refuse such a foreign key
// with the one rule kept here.
package deferkey

import (
	"slices"

	"github.com/azrtydxb/momo/schema"
)

// Covering describes the deferrable primary key or unique constraint of t that
// covers exactly cols, compared as sets. It returns "" when no deferrable key
// covers cols, or when a non-deferrable primary key, unique constraint or
// non-partial unique index over plain columns does too: PostgreSQL uses that
// key instead.
func Covering(t *schema.Table, cols []string) string {
	same := func(key []string) bool {
		if len(key) != len(cols) {
			return false
		}
		a, c := slices.Clone(key), slices.Clone(cols)
		slices.Sort(a)
		slices.Sort(c)
		return slices.Equal(a, c)
	}
	what := ""
	if same(t.PrimaryKey) {
		if !t.PrimaryKeyDeferrable && !t.PrimaryKeyInitiallyDeferred {
			return ""
		}
		what = "the primary key"
	}
	for _, u := range t.Uniques {
		if !same(u.Columns) {
			continue
		}
		if !u.Deferrable && !u.InitiallyDeferred {
			return ""
		}
		if what == "" {
			what = "unique " + u.Name
		}
	}
	for _, ix := range t.Indexes {
		if !ix.Unique || ix.Where != "" {
			continue
		}
		key := make([]string, 0, len(ix.Parts))
		for _, p := range ix.Parts {
			if p.Column == "" {
				key = nil
				break
			}
			key = append(key, p.Column)
		}
		if key != nil && same(key) {
			return ""
		}
	}
	return what
}
