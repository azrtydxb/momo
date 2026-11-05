// SPDX-License-Identifier: Apache-2.0

package companionunknown

import "github.com/azrtydxb/momo"

type Thing struct {
	momo.Model `momo:"table:things"`
	ID         int64
	Name       string
}

func (t *Thing) MomoConfig(c *momo.Config) {
	c.Frobnicate("x", &t.Name)
	c.Index("things_name_idx", &t.Name).Frobnicate()
}
