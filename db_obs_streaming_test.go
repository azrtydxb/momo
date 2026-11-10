// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// streamMaxGrowB is the heap growth bound for streaming: 16 MiB.
// docs/PERFORMANCE.md records the same figure.
const streamMaxGrowB = 16 << 20

func TestPerformanceDocRecordsStreamBound(t *testing.T) {
	raw, err := os.ReadFile("docs/PERFORMANCE.md")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\| Streaming heap growth bound \(MiB\) +\| +(\d+) +\|$`).FindSubmatch(raw)
	if m == nil {
		t.Fatal("docs/PERFORMANCE.md has no row '| Streaming heap growth bound (MiB) | N |'")
	}
	got, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	if got != streamMaxGrowB>>20 {
		t.Fatalf("docs/PERFORMANCE.md records %d MiB, constant is %d MiB", got, streamMaxGrowB>>20)
	}
}
