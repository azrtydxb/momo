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

const deltaHeader = `goos: linux
goarch: amd64
pkg: github.com/azrtydxb/momo/bench
              │ base │ new │
              │ allocs/op │ allocs/op vs base │
`

func deltaRows(momo, pgx, sql string) string {
	return deltaHeader +
		"Allocs_A/momo-4  10.00 ± 0%  " + momo + "\n" +
		"Allocs_A/pgx-4  5.000 ± 0%  " + pgx + "\n" +
		"Allocs_B/database/sql-4  7.000 ± 0%  " + sql + "\n"
}

func TestComparedFlagsMissingRows(t *testing.T) {
	same := "5.000 ± 0%  ~  (p=1.000 n=6+6)"
	full := deltaRows("10.00 ± 0%  ~  (p=1.000 n=6+6)", same, "7.000 ± 0%  ~  (p=1.000 n=6+6)")
	if err := compared(strings.NewReader(rawBaseline), strings.NewReader(full)); err != nil {
		t.Fatalf("full delta: %v", err)
	}
	// One benchmark absent from the delta must fail and be named.
	partial := deltaHeader + "Allocs_A/momo-4  10.00 ± 0%  10.00 ± 0%  ~  (p=1.000 n=6+6)\n"
	err := compared(strings.NewReader(rawBaseline), strings.NewReader(partial))
	if !errors.Is(err, ErrNotCompared) {
		t.Fatalf("partial delta error = %v, want ErrNotCompared", err)
	}
	for _, name := range []string{"BenchmarkAllocs_A/pgx", "BenchmarkAllocs_B/database/sql"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
	if strings.Contains(err.Error(), "BenchmarkAllocs_A/momo") {
		t.Errorf("error %q names a benchmark that was compared", err)
	}
	// Two tables of one run each carry no delta row at all.
	raw, rerr := os.ReadFile("testdata/split_cpu.txt")
	if rerr != nil {
		t.Fatal(rerr)
	}
	base, rerr := os.ReadFile("../../baseline/allocs.txt")
	if rerr != nil {
		t.Fatal(rerr)
	}
	err = compared(strings.NewReader(string(base)), strings.NewReader(string(raw)))
	if !errors.Is(err, ErrNotCompared) || !strings.Contains(err.Error(), "BenchmarkAllocs_InsertOrder/momo") {
		t.Errorf("split tables error = %v, want ErrNotCompared naming BenchmarkAllocs_InsertOrder/momo", err)
	}
}

func TestStripCPU(t *testing.T) {
	in := "goos: linux\ngoarch: amd64\npkg: p\ncpu: AMD EPYC 7763 64-Core Processor   \nBenchmarkX-4 1 2 ns/op 3 B/op 4 allocs/op\n"
	var out strings.Builder
	if err := stripCPU(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	want := "goos: linux\ngoarch: amd64\npkg: p\nBenchmarkX-4 1 2 ns/op 3 B/op 4 allocs/op\n"
	if out.String() != want {
		t.Errorf("stripCPU = %q, want %q", out.String(), want)
	}
}

// With the cpu: line normalised away, benchstat compares the runs; the gate
// then judges real deltas: +10% fails, +2% passes.
func TestNormalisedComparisonJudgesDeltas(t *testing.T) {
	ok := "~  (p=1.000 n=6+6)"
	cases := []struct {
		name     string
		momo     string
		wantFail bool
	}{
		{"plus ten percent", "11.00 ± 0%  +10.00%  (p=0.002 n=6+6)", true},
		{"plus two percent", "10.20 ± 0%  +2.00%  (p=0.002 n=6+6)", false},
	}
	for _, c := range cases {
		delta := deltaRows(c.momo, "5.000 ± 0%  "+ok, "7.000 ± 0%  "+ok)
		if err := compared(strings.NewReader(rawBaseline), strings.NewReader(delta)); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		failed, err := gate(strings.NewReader(delta), 5)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if failed != c.wantFail {
			t.Errorf("%s: failed = %v, want %v", c.name, failed, c.wantFail)
		}
	}
}

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
