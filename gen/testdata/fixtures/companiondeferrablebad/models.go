// SPDX-License-Identifier: Apache-2.0

// Package companiondeferrablebad declares deferrable constraints the generator
// refuses.
package companiondeferrablebad

import "github.com/azrtydxb/momo"

type A struct {
	momo.Model `momo:"table:a"`
	ID         int64 `momo:"pk"`
	Name       string
}

func (a *A) MomoConfig(c *momo.Config) {
	c.Index("a_name_idx", &a.Name).Deferrable()
	c.Unique("a_name_key", &a.Name).Where("name <> ''").Deferrable()
	c.Unique("a_dup_key", &a.Name).Deferrable().Deferrable()
	c.PrimaryKeyDeferrable()
	c.PrimaryKeyDeferrable()
	c.PrimaryKeyInitiallyDeferred()
}

type C struct {
	momo.Model `momo:"table:c"`
	ID         int64 `momo:"pk"`
}

func (c2 *C) MomoConfig(c *momo.Config) {
	c.PrimaryKeyInitiallyDeferred()
	c.PrimaryKeyDeferrable()
}
