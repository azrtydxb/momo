// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// selectNoHooksAllocBudget is allocs/op of BenchmarkSelectNoHooks measured on
// PostgreSQL. docs/PERFORMANCE.md records the same integer.
const selectNoHooksAllocBudget = 0

func TestPerformanceDocRecordsBudget(t *testing.T) {
	raw, err := os.ReadFile("docs/PERFORMANCE.md")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\| BenchmarkSelectNoHooks allocs/op +\| +(\d+) +\|$`).FindSubmatch(raw)
	if m == nil {
		t.Fatal("docs/PERFORMANCE.md has no row '| BenchmarkSelectNoHooks allocs/op | N |'")
	}
	got, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	if got != selectNoHooksAllocBudget {
		t.Fatalf("docs/PERFORMANCE.md records %d, test constant is %d", got, selectNoHooksAllocBudget)
	}
}
