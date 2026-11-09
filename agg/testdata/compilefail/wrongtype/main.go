// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
)

var count = momo.NewColumn[int64]("docs", "n", 0)

func main() {
	_ = agg.String(count, ",")
}
