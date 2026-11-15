// SPDX-License-Identifier: Apache-2.0

package sqlitedriver

import (
	"os"
	"strings"
	"testing"
)

// Catches: the package doc losing the build flag a program needs before FTS5
// full-text search runs on this driver.
func TestDocStatesFTS5Requirement(t *testing.T) {
	b, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sqlite_fts5", "SQLITE_ENABLE_FTS5", "sqlitepure includes FTS5"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("doc.go does not mention %q", want)
		}
	}
}

func TestDocStatesProcessGlobalEffects(t *testing.T) {
	b, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"process-wide", "sqlite_omit_load_extension", "cgo"} {
		if !strings.Contains(s, want) {
			t.Fatalf("doc.go does not mention %q", want)
		}
	}
}
