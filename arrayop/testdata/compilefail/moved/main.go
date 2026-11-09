// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var tags = momo.NewColumn[[]string]("docs", "tags", 0)

func main() {
	_ = momo.ArrayContains(tags, "a")
	_ = momo.ArrayContainedBy(tags, "a")
	_ = momo.ArrayOverlaps(tags, "a")
	_ = momo.AnyOf(tags, []string{"a"})
}
