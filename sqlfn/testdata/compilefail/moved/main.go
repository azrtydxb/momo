// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var (
	title = momo.NewColumn[string]("docs", "title", 0)
	hits  = momo.NewColumn[int64]("docs", "hits", 0)
)

func main() {
	_ = momo.Substr(title, 1, 3)
	_ = momo.SubstrFrom(title, 2)
	_ = momo.Lower(title)
	_ = momo.Upper(title)
	_ = momo.Length(title)
	_ = momo.CurrentSchema()
	_ = momo.Case[string]()
	_ = momo.Add(hits, 1)
	_ = momo.AddExpr(hits, hits)
	_ = momo.Sub(hits, 1)
	_ = momo.SubExpr(hits, hits)
	_ = momo.Mul(hits, 2)
	_ = momo.MulExpr(hits, hits)
	_ = momo.CoalesceExpr(title, title)
	_ = momo.Cast[int64](title, "bigint")
	_ = momo.Concat(title, title)
	var _ momo.CaseExpr[string]
	var _ momo.ArithExpr[int64]
	var _ momo.CastExpr[int64]
	var _ momo.LengthExpr
	var _ momo.ConcatExpr
	var _ momo.CurrentSchemaExpr
	var _ momo.Numeric
}
