// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/timeop"
)

var title = momo.NewColumn[string]("docs", "title", 0)

func main() {
	_ = timeop.Extract(timeop.ExtractYear, title)
}
