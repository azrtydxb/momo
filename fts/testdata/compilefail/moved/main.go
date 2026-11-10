// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

var (
	vec = momo.NewColumn[types.TSVector]("docs", "tsv", 0)
	txt = momo.NewColumn[string]("docs", "body", 0)
)

func main() {
	q := momo.WebsearchToTSQuery("english", "x")
	_ = momo.PlainToTSQuery("english", "x")
	_ = momo.PhraseToTSQuery("english", "x")
	_ = momo.ToTSQuery("english", "x")
	_ = momo.ToTSVector("english", txt)
	_ = momo.TSMatch(vec, q)
	_ = momo.TSRank(vec, q)
	_ = momo.TSRankCD(vec, q)
	_ = momo.TSHeadline("english", txt, q, "")
	var _ momo.TSExpr[float64]
}
