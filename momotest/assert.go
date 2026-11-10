// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"strconv"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
)

// AssertSQL fails the test unless the compiled statement got has the SQL text
// want, ignoring differences in whitespace. Build got with a query's SQL
// method: nothing is executed, so the assertion needs no database.
//
// On a mismatch the message holds both texts, the number of bound arguments
// and got.String(). It never holds an argument value: every bound argument is
// listed as [redacted], so a value bound from a sensitive column cannot reach
// the test log.
func AssertSQL(tb testing.TB, got momo.Compiled, want string) {
	tb.Helper()
	g, w := normalizeSQL(got.SQL), normalizeSQL(want)
	if g == w {
		return
	}
	tb.Errorf("momotest.AssertSQL: SQL mismatch\n got: %s\nwant: %s\nargs: %d bound (%s)\ndump: %s",
		g, w, len(got.Args), redactedList(len(got.Args)), got.String())
}

// normalizeSQL collapses every run of whitespace to one space.
func normalizeSQL(s string) string { return strings.Join(strings.Fields(s), " ") }

// redactedList names the n bound arguments without their values.
func redactedList(n int) string {
	if n == 0 {
		return "none"
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "$" + strconv.Itoa(i+1) + "=" + momo.RedactedArg
	}
	return strings.Join(parts, ", ")
}
