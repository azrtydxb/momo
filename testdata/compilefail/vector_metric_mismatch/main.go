// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

var col = momo.NewVectorColumn[vector.Bits]("docs", "bits", 1, 0)

func main() {
	_ = momo.CosineDistance(col, vector.Bits{Len: 1, Data: []byte{0}})
}
