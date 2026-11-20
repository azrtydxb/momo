// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests exercise the deprecated names

package timeop_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/timeop"
)

// Catches: the deprecated IntervalExpr drifting from IntervalOfExpr, so a
// caller that has not migrated yet builds different SQL.
func TestDeprecatedIntervalExprMatchesIntervalOfExpr(t *testing.T) {
	count := momo.Value(int64(3))
	for _, d := range typedDialects {
		old, err := selectOne(d, timeop.AddInterval(tmCreated, timeop.IntervalExpr(count, timeop.IntervalWeek)))
		if err != nil {
			t.Fatalf("%s IntervalExpr: %v", d, err)
		}
		now, err := selectOne(d, timeop.AddInterval(tmCreated, timeop.IntervalOfExpr(count, timeop.IntervalWeek)))
		if err != nil {
			t.Fatalf("%s IntervalOfExpr: %v", d, err)
		}
		if old.SQL != now.SQL || len(old.Args) != len(now.Args) {
			t.Errorf("%s: IntervalExpr = %s %v, IntervalOfExpr = %s %v", d, old.SQL, old.Args, now.SQL, now.Args)
		}
	}
}
