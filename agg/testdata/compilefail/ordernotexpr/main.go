// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
)

var title = momo.NewColumn[string]("docs", "title", 0)

// An Order is not an Expr, so it cannot be selected or aggregated.
var _ momo.Expr = title.Asc()

func main() {
	_ = agg.JSONB[string](title.Asc())
}
