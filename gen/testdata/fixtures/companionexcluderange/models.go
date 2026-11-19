// SPDX-License-Identifier: Apache-2.0

// Package companionexcluderange declares an exclusion constraint over range
// columns only, which needs no btree_gist extension.
package companionexcluderange

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

type Shift struct {
	momo.Model `momo:"table:shifts"`
	ID         int64 `momo:"pk"`
	Slot       types.Range[int32]
	Days       types.Range[types.Date]
}

func (s *Shift) MomoConfig(c *momo.Config) {
	c.Exclude("shifts_slot_excl", &s.Slot, "&&", &s.Days, "=").Using("gist").Deferrable()
}
