// SPDX-License-Identifier: Apache-2.0

package main

import (
	"time"

	"github.com/azrtydxb/momo"
)

var created = momo.NewColumn[time.Time]("docs", "created_at", 0)

func main() {
	_ = momo.Extract(momo.ExtractYear, created)
	_ = momo.AtTimeZone(created, "UTC")
	_ = momo.DateTrunc(momo.TruncDay, created)
	_ = momo.AddInterval(created, momo.IntervalOf(1, momo.IntervalDay))
	_ = momo.SubInterval(created, momo.IntervalOf(1, momo.IntervalDay))
	_ = momo.Now()
	_ = momo.CurrentTimestamp()
	var _ momo.TimeExpr
}
