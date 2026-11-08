// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
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

func TestGateNoRowsIsAnError(t *testing.T) {
	for _, in := range []string{"", "goos: linux\ngoarch: amd64\n│ allocs/op │\n"} {
		failed, err := gate(strings.NewReader(in), 5)
		if !errors.Is(err, ErrNoRows) {
			t.Errorf("gate(%q) error = %v, want ErrNoRows", in, err)
		}
		if failed {
			t.Errorf("gate(%q) reported a regression on empty input", in)
		}
	}
}

const rawBaseline = `goos: linux
BenchmarkAllocs_A/momo-4   100   1000 ns/op   64 B/op   10 allocs/op
BenchmarkAllocs_A/pgx-4    100   1000 ns/op   64 B/op   5 allocs/op
BenchmarkAllocs_B/database/sql-4   100   1000 ns/op   64 B/op   7 allocs/op
`

func TestCoverage(t *testing.T) {
	full := "BenchmarkAllocs_A/momo-8 1 2 ns/op 3 B/op 4 allocs/op\n" +
		"BenchmarkAllocs_A/pgx-8 1 2 ns/op 3 B/op 4 allocs/op\n" +
		"BenchmarkAllocs_B/database/sql-8 1 2 ns/op 3 B/op 4 allocs/op\n"
	if err := coverage(strings.NewReader(rawBaseline), strings.NewReader(full)); err != nil {
		t.Fatalf("full run: %v", err)
	}
	// The benchmarks never ran: only build output, no result rows.
	err := coverage(strings.NewReader(rawBaseline), strings.NewReader("FAIL\tgithub.com/azrtydxb/momo/bench [build failed]\n"))
	if !errors.Is(err, ErrNoRows) {
		t.Errorf("empty run error = %v, want ErrNoRows", err)
	}
	partial := "BenchmarkAllocs_A/momo-8 1 2 ns/op 3 B/op 4 allocs/op\n"
	err = coverage(strings.NewReader(rawBaseline), strings.NewReader(partial))
	if !errors.Is(err, ErrMissingBenchmarks) {
		t.Fatalf("partial run error = %v, want ErrMissingBenchmarks", err)
	}
	for _, name := range []string{"BenchmarkAllocs_A/pgx", "BenchmarkAllocs_B/database/sql"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}
