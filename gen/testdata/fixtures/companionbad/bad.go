// SPDX-License-Identifier: Apache-2.0

package companionbad

import "github.com/azrtydxb/momo"

type Bad struct {
	momo.Model `momo:"table:bads"`
	ID         int64
	Name       string
}

const idx = "bads_name_idx"

func (b *Bad) MomoConfig(c *momo.Config) {
	c.Index(idx, &b.Name)
	name := "x"
	c.Check(name, "length(name) > 0")
}
