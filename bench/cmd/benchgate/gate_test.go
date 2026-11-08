// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

func TestBenchGate(t *testing.T) {
	cases := []struct {
		file     string
		wantFail bool
	}{
		{"testdata/regress.txt", true},
		{"testdata/flat.txt", false},
		{"testdata/improved.txt", false},
		{"testdata/small.txt", false},
	}
	for _, c := range cases {
		raw, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		failed, err := gate(strings.NewReader(string(raw)), 5)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if failed != c.wantFail {
			t.Errorf("%s: failed = %v, want %v", c.file, failed, c.wantFail)
		}
	}
}
