// SPDX-License-Identifier: Apache-2.0

// Command momovet reports non-constant SQL passed to Momo's raw-SQL
// functions. Run it standalone (momovet ./...) or as a vet tool:
//
//	go vet -vettool="$(which momovet)" ./...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/azrtydxb/momo/momovet"
)

func main() {
	singlechecker.Main(momovet.Analyzer)
}
