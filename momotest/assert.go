// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
)

// AssertSQL fails the test unless the compiled statement got has the SQL text
// want, ignoring differences in whitespace. Build got with a query's SQL
// method: nothing is executed, so the assertion needs no database.
//
// On a mismatch the message holds both texts and got.String(), which names the
// bound arguments by placeholder and never prints their values, so a value
// bound from a sensitive column cannot reach the test log.
func AssertSQL(tb testing.TB, got momo.Compiled, want string) {
	tb.Helper()
	g, w := normalizeSQL(got.SQL), normalizeSQL(want)
	if g == w {
		return
	}
	tb.Errorf("momotest.AssertSQL: SQL mismatch\n got: %s\nwant: %s\ndump: %s", g, w, got.String())
}

// normalizeSQL collapses every run of whitespace to one space.
func normalizeSQL(s string) string { return strings.Join(strings.Fields(s), " ") }
