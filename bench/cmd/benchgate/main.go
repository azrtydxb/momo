// SPDX-License-Identifier: Apache-2.0

// Command benchgate reads benchstat text output (stdin or a file argument) and
// exits 1 when any row shows a positive delta above -max-percent with p below
// 0.05. CI feeds it the allocs/op comparison only, so timings are never gated.
//
//	benchgate -max-percent 5 delta.txt
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
)

// A benchstat row with a significant change ends in a signed percentage and
// "(p=0.002 n=6+6)"; an insignificant one has "~" instead of the percentage.
var rowRE = regexp.MustCompile(`([+-][0-9]+(?:\.[0-9]+)?)%\s+\(p=([0-9.]+)`)

const significance = 0.05

// gate reports whether any row regresses by more than maxPct percent with
// p < 0.05. Rows with ~ (no significant change) never fail.
func gate(r io.Reader, maxPct float64) (failed bool, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
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
	return failed, sc.Err()
}

func main() {
	maxPct := flag.Float64("max-percent", 5, "largest tolerated allocs/op increase in percent")
	flag.Parse()
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
