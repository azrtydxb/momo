// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
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
		"_fk=0", "_foreign_keys=0", "_journal=OFF", "_journal_mode=OFF", "_sync=OFF", "_synchronous=OFF",
		"_locking_mode=EXCLUSIVE", "_mutex=no", "_secure_delete=1", "_case_sensitive_like=1",
		"_defer_foreign_keys=1", "_recursive_triggers=1", "_writable_schema=1", "_trusted_schema=1",
		"_query_only=1", "_ignore_check_constraints=1", "_auth=1", "_some_future_key=1", "vfs=x", "mode=bogus", "cache=bogus",
	} {
		if _, err := buildDSN("x.db?"+key, Config{}); err == nil {
			t.Errorf("DSN parameter %s accepted", key)
		}
	}
	for _, key := range []string{"_busy_timeout=7", "_timeout=7", "_txlock=deferred", "_loc=auto", "_cache_size=-2000", "mode=rwc", "cache=shared"} {
		if _, err := buildDSN("file:x.db?"+key, Config{}); err != nil {
			t.Errorf("DSN parameter %s refused: %v", key, err)
		}
	}
}

func TestBuildDSNKeepsSafetiesOn(t *testing.T) {
	got, err := buildDSN("x.db?_timeout=7", Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"_foreign_keys=on", "_journal_mode=WAL", "_synchronous=NORMAL", "_busy_timeout=7"} {
		if !strings.Contains(got, want) {
			t.Errorf("DSN %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "&_timeout=") || strings.Contains(got, "?_timeout=") {
		t.Errorf("DSN %q still carries the alias next to _busy_timeout", got)
	}
}

// mattn hands the query to SQLite only for a "file:" DSN, so mode and cache on
// a plain path would be silently ignored: x.db?mode=ro would open read-write.
func TestBuildDSNRefusesURIParametersOnPlainPaths(t *testing.T) {
	for _, dsn := range []string{
		"x.db?mode=ro", "x.db?mode=memory", "x.db?cache=shared", ":memory:?cache=shared", ":memory:?mode=memory",
	} {
		if _, err := buildDSN(dsn, Config{MaxOpenConns: 4}); err == nil {
			t.Errorf("%q accepted although SQLite never sees its mode or cache parameter", dsn)
		}
	}
	if isMemory("x.db", map[string][]string{"mode": {"memory"}}) {
		t.Error("a plain path with mode=memory counted as in-memory")
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
	if _, err := buildDSN("file::memory:?cache=shared", Config{MaxOpenConns: 4}); err != nil {
		t.Errorf("a shared-cache in-memory database with four connections refused: %v", err)
	}
}

func TestBuildDSN(t *testing.T) {
	got, err := buildDSN("file:x.db?_busy_timeout=9", Config{BusyTimeout: time.Second, Synchronous: "FULL", TxLock: "deferred"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"_busy_timeout=9", "_journal_mode=WAL", "_synchronous=FULL", "_foreign_keys=on", "_txlock=deferred"} {
		if !strings.Contains(got, want) {
			t.Errorf("DSN %q lacks %q", got, want)
		}
	}
	if _, err := buildDSN("x.db", Config{Synchronous: "bogus"}); err == nil {
		t.Error("unknown synchronous mode accepted")
	}
	if _, err := buildDSN("x.db", Config{TxLock: "bogus"}); err == nil {
		t.Error("unknown txlock accepted")
	}
}

// An empty path would make SQLite create a file named after the query string,
// without foreign keys, WAL or the busy timeout.
func TestBuildDSNRefusesEmptyPath(t *testing.T) {
	for _, dsn := range []string{"", "?_txlock=immediate", "?mode=memory"} {
		_, err := buildDSN(dsn, Config{})
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("buildDSN(%q) err = %v, want ErrValidation", dsn, err)
		}
	}
	if isMemory("", nil) {
		t.Error("an empty path counted as in-memory")
	}
}
