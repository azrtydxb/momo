// SPDX-License-Identifier: Apache-2.0

// Command benchgate reads benchstat text output (stdin or a file argument) and
// exits 1 when any row shows a positive delta above -max-percent with p below
// 0.05. CI feeds it the allocs/op comparison only, so timings are never gated.
//
// It exits 2, never 0, when the gate could not judge anything: the comparison
// has no data rows, the new run has no allocs/op rows, or a benchmark in the
// baseline is missing from the new run. A benchmark run that failed or never
// ran therefore cannot pass.
//
//	benchgate -max-percent 5 -baseline bench/baseline/allocs.txt -new new.txt delta.txt
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ErrNoRows reports input the gate cannot judge: no comparison rows, or a raw
// benchmark run without allocs/op results.
var ErrNoRows = errors.New("benchgate: no allocs/op rows in the input")

// ErrMissingBenchmarks reports baseline benchmarks that the new run lacks.
var ErrMissingBenchmarks = errors.New("benchgate: baseline benchmarks missing from the new run")

// rawRowRE matches a go test -benchmem result line and captures the benchmark
// name; the trailing -N is the GOMAXPROCS suffix.
var rawRowRE = regexp.MustCompile(`^(Benchmark\S+?)(?:-[0-9]+)?\s+[0-9]+\s+.*\sallocs/op`)

// benchmarkNames returns the set of benchmark names with allocs/op results in
// a raw go test output.
func benchmarkNames(r io.Reader) (map[string]bool, error) {
	names := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if m := rawRowRE.FindStringSubmatch(sc.Text()); m != nil {
			names[m[1]] = true
		}
	}
	return names, sc.Err()
}

// coverage checks that the new run produced allocs/op rows and covers every
// benchmark of the baseline.
func coverage(baseline, newRun io.Reader) error {
	have, err := benchmarkNames(newRun)
	if err != nil {
		return err
	}
	if len(have) == 0 {
		return fmt.Errorf("%w: the new run has no allocs/op results", ErrNoRows)
	}
	want, err := benchmarkNames(baseline)
	if err != nil {
		return err
	}
	var missing []string
	for n := range want {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%w: %s", ErrMissingBenchmarks, strings.Join(missing, ", "))
	}
	return nil
}

// A benchstat row with a significant change ends in a signed percentage and
// "(p=0.002 n=6+6)"; an insignificant one has "~" instead of the percentage.
var rowRE = regexp.MustCompile(`([+-][0-9]+(?:\.[0-9]+)?)%\s+\(p=([0-9.]+)`)

const significance = 0.05

// gate reports whether any row regresses by more than maxPct percent with
// p < 0.05. Rows with ~ (no significant change) never fail.
func gate(r io.Reader, maxPct float64) (failed bool, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	rows := 0
	for sc.Scan() {
		if strings.Contains(sc.Text(), "±") {
			rows++
		}
		m := rowRE.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		delta, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return false, err
		}
		p, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return false, err
		}
		if delta > maxPct && p < significance {
			fmt.Fprintf(os.Stderr, "allocation regression: %s\n", sc.Text())
			failed = true
		}
	}
	if err := sc.Err(); err != nil {
		return failed, err
	}
	if rows == 0 {
		return false, ErrNoRows
	}
	return failed, nil
}

func main() {
	maxPct := flag.Float64("max-percent", 5, "largest tolerated allocs/op increase in percent")
	baselineFile := flag.String("baseline", "", "raw baseline run; with -new, every benchmark in it must appear in the new run")
	newFile := flag.String("new", "", "raw new run; must contain allocs/op rows")
	flag.Parse()
	if (*baselineFile == "") != (*newFile == "") {
		fmt.Fprintln(os.Stderr, "benchgate: -baseline and -new go together")
		os.Exit(2)
	}
	if *newFile != "" {
		if err := checkFiles(*baselineFile, *newFile); err != nil {
			fmt.Fprintln(os.Stderr, "benchgate:", err)
			os.Exit(2)
		}
	}
	var in io.Reader = os.Stdin
	if flag.NArg() > 0 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, "benchgate:", err)
			os.Exit(2)
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	failed, err := gate(in, *maxPct)
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchgate:", err)
		os.Exit(2)
	}
	if failed {
		os.Exit(1)
	}
}

func checkFiles(baselinePath, newPath string) error {
	b, err := os.Open(baselinePath) // #nosec G304 -- operator-supplied path
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	n, err := os.Open(newPath) // #nosec G304 -- operator-supplied path
	if err != nil {
		return err
	}
	defer func() { _ = n.Close() }()
	return coverage(b, n)
}
