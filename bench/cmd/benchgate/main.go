// SPDX-License-Identifier: Apache-2.0

// Command benchgate reads benchstat text output (stdin or a file argument) and
// exits 1 when any row shows a positive delta above -max-percent with p below
// 0.05. CI feeds it the allocs/op comparison only, so timings are never gated.
//
// It exits 2, never 0, when the gate could not judge anything: the comparison
// has no data rows, the new run has no allocs/op rows, a benchmark in the
// baseline is missing from the new run, or a baseline benchmark has no
// comparison row in the delta (benchstat splits its tables when a config line
// differs). A benchmark run that failed or never ran therefore cannot pass.
//
// With -strip-cpu it instead copies its input without the "cpu:" line; the
// Makefile runs both benchstat inputs through it, because allocs/op does not
// depend on the CPU model.
//
//	benchgate -strip-cpu new.txt > new-nocpu.txt
//	benchgate -max-percent 5 -baseline bench/baseline/allocs.txt -new new.txt delta.txt
package main

import (
	"bufio"
	"bytes"
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

// ErrNotCompared reports baseline benchmarks that have no comparison row in
// the benchstat output, for instance because benchstat split baseline and new
// into two tables after a config line (cpu, goos, goarch, pkg) differed.
var ErrNotCompared = errors.New("benchgate: baseline benchmarks without a comparison row in the delta")

// deltaRowRE matches a benchstat row that compares two runs: a name, two
// "value ± spread" columns and a "(p=... n=...)" verdict. A row of a table
// holding one run has a single "±" column and never matches.
var deltaRowRE = regexp.MustCompile(`^(\S+)\s.*±.*±.*\(p=[0-9.]+\s+n=`)

// gomaxprocsRE is the -N suffix go test adds to a benchmark name.
var gomaxprocsRE = regexp.MustCompile(`-[0-9]+$`)

// comparedNames returns the benchmark names (in go test spelling, without the
// GOMAXPROCS suffix) that have a comparison row in benchstat output.
func comparedNames(r io.Reader) (map[string]bool, error) {
	names := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if m := deltaRowRE.FindStringSubmatch(sc.Text()); m != nil {
			names["Benchmark"+gomaxprocsRE.ReplaceAllString(m[1], "")] = true
		}
	}
	return names, sc.Err()
}

// compared fails unless every benchmark of the raw baseline has a comparison
// row in the benchstat delta, so a gate that compared nothing cannot pass.
func compared(baseline, delta io.Reader) error {
	want, err := benchmarkNames(baseline)
	if err != nil {
		return err
	}
	if len(want) == 0 {
		return fmt.Errorf("%w: the baseline has no allocs/op results", ErrNoRows)
	}
	have, err := comparedNames(delta)
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
		return fmt.Errorf("%w: %s", ErrNotCompared, strings.Join(missing, ", "))
	}
	return nil
}

// stripCPU copies a raw benchmark run or benchstat input, dropping the "cpu:"
// config line. allocs/op does not depend on the CPU model, but benchstat
// splits its tables whenever that line differs between the two inputs.
func stripCPU(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" && !strings.HasPrefix(line, "cpu:") {
			if _, werr := io.WriteString(w, line); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
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
	var uncompared []string
	for sc.Scan() {
		if strings.Contains(sc.Text(), "±") {
			rows++
			if !deltaRowRE.MatchString(sc.Text()) {
				uncompared = append(uncompared, strings.Fields(sc.Text())[0])
			}
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
	if len(uncompared) > 0 {
		return failed, fmt.Errorf("%w: %s", ErrNotCompared, strings.Join(uncompared, ", "))
	}
	return failed, nil
}

func main() {
	maxPct := flag.Float64("max-percent", 5, "largest tolerated allocs/op increase in percent")
	baselineFile := flag.String("baseline", "", "raw baseline run; with -new, every benchmark in it must appear in the new run")
	newFile := flag.String("new", "", "raw new run; must contain allocs/op rows")
	strip := flag.Bool("strip-cpu", false, "copy the file argument (or stdin) to stdout without its cpu: line, then exit")
	flag.Parse()
	if *strip {
		if err := stripCPUMain(); err != nil {
			fmt.Fprintln(os.Stderr, "benchgate:", err)
			os.Exit(2)
		}
		return
	}
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
	raw, err := readInput()
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchgate:", err)
		os.Exit(2)
	}
	failed, err := gate(bytes.NewReader(raw), *maxPct)
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchgate:", err)
		os.Exit(2)
	}
	if *baselineFile != "" {
		if err := comparedFiles(*baselineFile, raw); err != nil {
			fmt.Fprintln(os.Stderr, "benchgate:", err)
			os.Exit(2)
		}
	}
	if failed {
		os.Exit(1)
	}
}

// readInput returns the file argument, or stdin when there is none.
func readInput() ([]byte, error) {
	if flag.NArg() == 0 {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(flag.Arg(0))
}

func stripCPUMain() error {
	raw, err := readInput()
	if err != nil {
		return err
	}
	return stripCPU(bytes.NewReader(raw), os.Stdout)
}

func comparedFiles(baselinePath string, delta []byte) error {
	b, err := os.Open(baselinePath) // #nosec G304 -- operator-supplied path
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	return compared(b, bytes.NewReader(delta))
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
