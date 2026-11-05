// SPDX-License-Identifier: Apache-2.0

package companionbad

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

type Shapes struct {
	momo.Model `momo:"table:shapes"`
	ID         int64
	Name       string
}

func (s *Shapes) MomoConfig(c *momo.Config) {
	f := c.Index
	f("shapes_name_idx", &s.Name)
	v := "UPDATE"
	c.Trigger(schema.Trigger{Name: "t", Events: []string{v}})
	c.RLS(schema.RLS{Enabled: true})
	c.RLS(schema.RLS{Forced: true})
	c.Partition(schema.Partition{Strategy: "range", Key: "RANGE (id)"})
	c.Partition(schema.Partition{Strategy: "hash", Key: "HASH (id)"})
	c.PartitionOf(schema.PartitionOf{Parent: "a", Bound: "x"})
	c.PartitionOf(schema.PartitionOf{Parent: "b", Bound: "y"})
	c.Index("shapes_dup_opt_idx", &s.Name).Where("").Where("x")
}
