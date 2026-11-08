// SPDX-License-Identifier: Apache-2.0

package sqlshared

import (
	"database/sql"
	"errors"
	"testing"
)

// testHooks classifies nothing and recognises no driver error.
var testHooks = &Hooks{
	Classify:    func(_ string, err error) error { return err },
	IsDriverErr: func(error) bool { return false },
}

func FuzzParseTime(f *testing.F) {
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

func FuzzScanErr(f *testing.F) {
	for _, s := range []string{"", "converting driver.Value type string", "Scan error on column index 0", "sql: no rows"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		in := errors.New(s)
		got := testHooks.scanErr(in)
		if got == nil || (got != in && !errors.Is(got, in)) {
			t.Fatalf("scanErr(%q) = %v; the cause must stay reachable", s, got)
		}
		if errors.Is(got, sql.ErrNoRows) {
			t.Fatal("an unrelated error became ErrNoRows")
		}
	})
}
