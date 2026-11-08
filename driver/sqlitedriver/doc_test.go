// SPDX-License-Identifier: Apache-2.0

package sqlitedriver

import (
	"os"
	"strings"
	"testing"
)

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
