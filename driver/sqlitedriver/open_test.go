// SPDX-License-Identifier: Apache-2.0

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
