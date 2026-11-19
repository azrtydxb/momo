// SPDX-License-Identifier: Apache-2.0

// Package companionexcludebad declares exclusion constraints the generator
// refuses.
package companionexcludebad

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

type A struct {
	momo.Model `momo:"table:a"`
	ID         int64 `momo:"pk"`
	Slot       types.Range[int32]
}

func (a *A) MomoConfig(c *momo.Config) {
	// An operator that is not an operator, so it cannot reach the DDL.
	c.Exclude("a_bad_op", &a.Slot, "&&; DROP TABLE a")
	// A field with no operator.
	c.Exclude("a_odd", &a.Slot)
	// An access method PostgreSQL does not accept for an exclusion constraint.
	c.Exclude("a_bad_method", &a.Slot, "&&").Using("brin")
	// A repeated option.
	c.Exclude("a_dup", &a.Slot, "&&").Using("gist").Using("gist")
}
