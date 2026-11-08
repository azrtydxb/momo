// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
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

func TestBuildDSNAliasesAndForbiddenKeys(t *testing.T) {
	got, err := buildDSN("x.db?_fk=0&_timeout=7&_journal=DELETE&_sync=OFF", Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, dup := range []string{"_foreign_keys", "_busy_timeout", "_journal_mode", "_synchronous"} {
		if strings.Contains(got, dup+"=") {
			t.Errorf("DSN %q sets %s although its alias was given", got, dup)
		}
	}
	for _, key := range []string{"_writable_schema", "_trusted_schema", "_query_only", "_ignore_check_constraints", "_auth"} {
		if _, err := buildDSN("x.db?"+key+"=1", Config{}); err == nil {
			t.Errorf("DSN key %s accepted", key)
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
