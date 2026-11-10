// SPDX-License-Identifier: Apache-2.0

package companionschemabad

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

type Bad struct {
	momo.Model `momo:"table:bads"`
	ID         int64 `momo:"pk"`
}

var strategy = "range"

func (b *Bad) MomoConfig(c *momo.Config) {
	rls := schema.RLS{Enabled: true}
	c.RLS(rls)                                                           // BAD-1
	c.Partition(schema.Partition{Strategy: strategy, Key: "RANGE (id)"}) // BAD-2
	var v schema.View
	c.View(v) // BAD-3
}
