// SPDX-License-Identifier: Apache-2.0

// Package companiondeferrablenopk makes the primary key of a model deferrable
// when the model has none.
package companiondeferrablenopk

import "github.com/azrtydxb/momo"

type B struct {
	momo.Model `momo:"table:b"`
	Name       string
}

func (b *B) MomoConfig(c *momo.Config) {
	c.PrimaryKeyDeferrable()
}
