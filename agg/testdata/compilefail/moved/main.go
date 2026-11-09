// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var title = momo.NewColumn[string]("docs", "title", 0)

var (
	_ momo.AggExpr[string]
	_ momo.JSONPair
	_ momo.JSONObjectExpr[string]
	_ momo.ToJSONExpr[string]
)

func main() {
	_ = momo.StringAgg(title, ",")
	_ = momo.ArrayAgg[string](title)
	_ = momo.JSONBAgg[string](title)
	_ = momo.PercentileCont(0.5)
	_ = momo.PercentileDisc[int64](0.5)
	_ = momo.Aggregate[int64]("max", title)
	_ = momo.JSONField("k", title)
	_ = momo.JSONBBuildObject[string]()
	_ = momo.ToJSONB[string](title)
	_ = momo.EmptyJSONArray[string]()
}
