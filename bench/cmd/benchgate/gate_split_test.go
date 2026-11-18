// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

// TestGateFailsOnSplitTables replays the benchstat output of CI run
// 44161173178: the cpu: line of the baseline and of the runner differed, so
// benchstat printed two tables without a single delta row. InsertOrder/momo
// had gone from 38 to 42 allocs/op and the gate passed because it compared
// nothing. A comparison without delta rows must fail the gate.
func TestGateFailsOnSplitTables(t *testing.T) {
	raw, err := os.ReadFile("testdata/split_cpu.txt")
	if err != nil {
		t.Fatal(err)
	}
	failed, err := gate(strings.NewReader(string(raw)), 5)
	if err == nil && !failed {
		t.Fatal("gate passed on two benchstat tables that compare nothing")
	}
}
