// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"net/url"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/internal/sqlshared"
)

func FuzzConstraintTarget(f *testing.F) {
	for _, s := range []string{"UNIQUE constraint failed: u.email", "NOT NULL constraint failed: a.b, a.c", "", ": ", "constraint failed: "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		table, _ := sqlshared.ConstraintTarget(s)
		if strings.Contains(table, ".") {
			t.Fatalf("table %q holds a dot", table)
		}
	})
}

func FuzzCheckVersion(f *testing.F) {
	for _, s := range []string{"3.35.0", "3.34.9", "4", "", "3.x", "99999999999999999999.1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { _ = checkVersion(s) })
}

func FuzzBuildDSN(f *testing.F) {
	for _, s := range []string{"x.db", "file:x.db?_fk=0", ":memory:", "x.db?%zz", "x.db?_writable_schema=1", "x.db?_foreign_keys=0", "x.db?_busy_timeout=5&_txlock=deferred"} {
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
			if !allowedDSNKeys[key] && key != "_foreign_keys" && key != "_journal_mode" && key != "_synchronous" {
				t.Fatalf("built DSN %q carries the unlisted key %q", got, key)
			}
		}
		if q.Get("_foreign_keys") != "on" || q.Get("_journal_mode") != "WAL" || q.Has("_fk") || q.Has("_journal") || q.Has("_sync") {
			t.Fatalf("built DSN %q does not enforce foreign keys and WAL", got)
		}
	})
}
