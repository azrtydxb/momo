// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

var (
	emb  = momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	bits = momo.NewVectorColumn[vector.Bits]("docs", "bits", 8, 0)
)

func main() {
	q := vector.Vector{1, 2, 3}
	b := vector.Bits{Len: 8, Data: []byte{0}}
	_ = momo.L2Distance(emb, q)
	_ = momo.InnerProductDistance(emb, q)
	_ = momo.CosineDistance(emb, q)
	_ = momo.L1Distance(emb, q)
	_ = momo.HammingDistance(bits, b)
	_ = momo.JaccardDistance(bits, b)
	var _ momo.DistanceExpr
}
