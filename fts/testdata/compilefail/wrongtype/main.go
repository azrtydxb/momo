// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/fts"
)

var title = momo.NewColumn[string]("docs", "title", 0)

func main() {
	// A text column is not a tsvector: Match takes Expression[types.TSVector].
	_ = fts.Match(title, fts.WebsearchToQuery("english", "x"))
}
