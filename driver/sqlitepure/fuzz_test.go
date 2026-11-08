// SPDX-License-Identifier: Apache-2.0

package sqlitepure

import (
	"net/url"
	"strings"
	"testing"
)

func FuzzPureParseTime(f *testing.F) {
	for _, s := range []string{"2026-11-07 20:10:12.123", "2026-11-07T20:10:12Z", "2026-11-07", "", "x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_, err := parseTime(s)
		if err != nil && err != errUnparsableTime {
			t.Fatalf("the error is not the fixed one, so it may echo the input: %v", err)
		}
	})
}

func FuzzPureConstraintTarget(f *testing.F) {
	for _, s := range []string{"UNIQUE constraint failed: u.email", "NOT NULL constraint failed: a.b, a.c", "", ": ", "constraint failed: "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		table, _ := constraintTarget(cleanMessage(s))
		if strings.Contains(table, ".") {
			t.Fatalf("table %q holds a dot", table)
		}
	})
}

func FuzzPureBuildDSN(f *testing.F) {
	for _, s := range []string{"x.db", "file:x.db?mode=ro", ":memory:", "x.db?%zz", "x.db?_pragma=foreign_keys(0)", "x.db?_txlock=deferred"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, dsn string) {
		got, err := buildDSN(dsn, Config{})
		if err != nil {
			return
		}
		_, rawQuery, _ := strings.Cut(got, "?")
		q, perr := url.ParseQuery(rawQuery)
		if perr != nil {
			t.Fatalf("built DSN %q does not parse: %v", got, perr)
		}
		for key := range q {
			if !allowedDSNKeys[key] && key != "_pragma" && key != "_time_format" {
				t.Fatalf("built DSN %q carries the unlisted key %q", got, key)
			}
		}
		want := map[string]bool{"journal_mode(WAL)": true, "foreign_keys(1)": true}
		for _, p := range q["_pragma"] {
			delete(want, p)
		}
		if len(want) != 0 {
			t.Fatalf("built DSN %q lacks the safety pragmas %v", got, want)
		}
	})
}
