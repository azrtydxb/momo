// SPDX-License-Identifier: Apache-2.0

package companiontypeerr

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

type Wrong struct {
	momo.Model `momo:"table:wrongs"`
	ID         int64
	Name       string
}

func (w *Wrong) MomoConfig(c *momo.Config) {
	c.Index
	c.View(schema.Policy{})
	c.View(schema.View{Name: 5})
	c.View(schema.View{Materialized: "x"})
	c.Trigger(schema.Trigger{Events: []int{}})
}
