// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/sqlfn"
	"github.com/azrtydxb/momo/types"
)

var day = momo.NewColumn[types.Date]("documents", "day", 0)

func main() {
	// types.Numeric joined the constraint by exact type; no other types struct
	// did, so a Date column still has no arithmetic.
	_ = sqlfn.Add(day, types.Date{})
}
