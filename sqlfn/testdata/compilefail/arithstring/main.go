// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/sqlfn"
)

var title = momo.NewColumn[string]("documents", "title", 0)

func main() {
	// Arithmetic is only defined for numeric columns.
	_ = sqlfn.Add(title, "x")
}
