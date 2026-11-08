// SPDX-License-Identifier: Apache-2.0

package sqlitepure

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCheckVersion(t *testing.T) {
	for v, ok := range map[string]bool{"3.34.9": false, "3.35.0": true, "3.53.4": true, "2.8.17": false, "4.0.0": true, "": false, "x.y": false} {
		err := checkVersion(v)
		if (err == nil) != ok {
			t.Fatalf("checkVersion(%q) err=%v, want ok=%v", v, err, ok)
		}
	}
}

func TestBuildDSNRefusesUnknownAndWeakeningKeys(t *testing.T) {
	for _, key := range []string{
		"_pragma=foreign_keys(0)", "_pragma=journal_mode(OFF)", "_pragma=busy_timeout(0)", "_fk=0", "_foreign_keys=0",
		"_journal_mode=OFF", "_synchronous=OFF", "_busy_timeout=7", "_time_format=bogus", "vfs=x", "_some_future_key=1",
		"mode=bogus", "cache=bogus", "_txlock=bogus",
	} {
		if _, err := buildDSN("file:x.db?"+key, Config{}); err == nil {
			t.Errorf("DSN parameter %s accepted", key)
		}
	}
	for _, key := range []string{"_txlock=deferred", "mode=rwc", "cache=shared"} {
		if _, err := buildDSN("file:x.db?"+key, Config{}); err != nil {
			t.Errorf("DSN parameter %s refused: %v", key, err)
		}
	}
	if _, err := buildDSN("file:x.db?mode=ro&mode=rw", Config{}); err == nil {
		t.Error("a repeated DSN parameter was accepted")
	}
}

func TestBuildDSNSetsSafetiesFromConfig(t *testing.T) {
	got, err := buildDSN("x.db", Config{BusyTimeout: 7 * time.Millisecond, Synchronous: "FULL", TxLock: "deferred"})
	if err != nil {
		t.Fatal(err)
	}
	_, rawQuery, _ := strings.Cut(got, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"busy_timeout(7)": true, "journal_mode(WAL)": true, "synchronous(FULL)": true, "foreign_keys(1)": true}
	for _, p := range q["_pragma"] {
		delete(want, p)
	}
	if len(want) != 0 {
		t.Errorf("DSN %q lacks pragmas %v", got, want)
	}
	if q.Get("_txlock") != "deferred" || q.Get("_time_format") != "sqlite" {
		t.Errorf("DSN %q: _txlock/_time_format wrong", got)
	}
	def, err := buildDSN("x.db", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(def, url.QueryEscape("busy_timeout(5000)")) || !strings.Contains(def, "_txlock=immediate") {
		t.Errorf("default DSN %q lacks the default busy timeout or txlock", def)
	}
	if _, err := buildDSN("x.db", Config{Synchronous: "bogus"}); err == nil {
		t.Error("unknown synchronous mode accepted")
	}
	if _, err := buildDSN("x.db", Config{TxLock: "bogus"}); err == nil {
		t.Error("unknown txlock accepted")
	}
}

func TestBuildDSNRefusesURIParametersOnPlainPaths(t *testing.T) {
	for _, dsn := range []string{"x.db?mode=ro", "x.db?mode=memory", "x.db?cache=shared", ":memory:?cache=shared"} {
		if _, err := buildDSN(dsn, Config{MaxOpenConns: 4}); err == nil {
			t.Errorf("%q accepted although SQLite never sees its mode or cache parameter", dsn)
		}
	}
	for _, dsn := range []string{"file:x.db?mode=ro", "file:m?mode=memory&cache=shared", "file::memory:?cache=shared"} {
		if _, err := buildDSN(dsn, Config{MaxOpenConns: 4}); err != nil {
			t.Errorf("%q refused: %v", dsn, err)
		}
	}
}

func TestBuildDSNInMemory(t *testing.T) {
	for _, dsn := range []string{":memory:", "file::memory:", "file:m?mode=memory"} {
		if _, err := buildDSN(dsn, Config{MaxOpenConns: 2}); err == nil {
			t.Errorf("%q with two connections accepted: each connection would see its own empty database", dsn)
		}
		if _, err := buildDSN(dsn, Config{}); err != nil {
			t.Errorf("%q with one connection refused: %v", dsn, err)
		}
	}
	if _, err := buildDSN("x.db", Config{MaxOpenConns: 4}); err != nil {
		t.Errorf("a file with four connections refused: %v", err)
	}
}

func TestConstraintTarget(t *testing.T) {
	for msg, want := range map[string][2]string{
		"UNIQUE constraint failed: u.email":    {"u", "email"},
		"NOT NULL constraint failed: a.b, a.c": {"a", "b"},
		"SQL logic error: no such table: x":    {"", ""},
		"FOREIGN KEY constraint failed":        {"", ""},
		"CHECK constraint failed: positive":    {"", ""},
		"UNIQUE constraint failed: onlytable":  {"", ""},
		"constraint failed: ":                  {"", ""},
		"":                                     {"", ""},
	} {
		table, column := constraintTarget(msg)
		if table != want[0] || column != want[1] {
			t.Errorf("constraintTarget(%q) = %q, %q; want %q, %q", msg, table, column, want[0], want[1])
		}
	}
}

func TestCleanMessage(t *testing.T) {
	for in, want := range map[string]string{
		"UNIQUE constraint failed: u.email (2067)": "UNIQUE constraint failed: u.email",
		"no suffix": "no suffix",
		"":          "",
	} {
		if got := cleanMessage(in); got != want {
			t.Errorf("cleanMessage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTime(t *testing.T) {
	for _, s := range []string{"2026-11-07 20:10:12.123", "2026-11-07T20:10:12Z", "2026-11-07 20:10:12.5+02:00", "2026-11-07"} {
		if _, err := parseTime(s); err != nil {
			t.Errorf("parseTime(%q): %v", s, err)
		}
	}
	if _, err := parseTime("secret"); err != errUnparsableTime {
		t.Errorf("err = %v, want the fixed error", err)
	}
}
