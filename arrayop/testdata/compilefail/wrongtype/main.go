// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/arrayop"
)

var tags = momo.NewColumn[[]string]("docs", "tags", 0)

func main() {
	_ = arrayop.Contains(tags, 42)
}
