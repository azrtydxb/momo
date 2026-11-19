// SPDX-License-Identifier: Apache-2.0

// Package companionexclude declares an exclusion constraint over a scalar
// column and a range column, and range-typed fields.
package companionexclude

import (
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

type Booking struct {
	momo.Model `momo:"table:bookings"`
	ID         int64 `momo:"pk"`
	RoomID     int64
	During     types.Range[time.Time]
	Local      types.Range[types.Timestamp]
	Days       types.Range[types.Date]
	Qty        types.Range[int32]
	Big        types.Range[int64]
	Price      types.Range[types.Numeric]
}

func (b *Booking) MomoConfig(c *momo.Config) {
	c.Exclude("bookings_no_overlap", &b.RoomID, "=", &b.During, "&&").InitiallyDeferred()
}
