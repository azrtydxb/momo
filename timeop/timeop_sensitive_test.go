// SPDX-License-Identifier: Apache-2.0

package timeop_test

import (
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/timeop"
)

// TestTimeExpressionsMaskComparedArgs requires an argument compared with a
// time expression over a sensitive column to show as RedactedArg in hook
// events, while the driver gets the real value. A plain column stays visible.
// It catches a time wrapper (Extract, AddInterval, DateTrunc) that hides the
// sensitive column from the predicate that compares it.
func TestTimeExpressionsMaskComparedArgs(t *testing.T) {
	when := time.Date(2024, 3, 5, 10, 0, 0, 0, time.UTC)
	build := map[string]func(col momo.Column[time.Time]) momo.Predicate{
		"extract": func(c momo.Column[time.Time]) momo.Predicate { return timeop.Extract(timeop.ExtractYear, c).Eq(2024) },
		"interval": func(c momo.Column[time.Time]) momo.Predicate {
			return timeop.AddInterval(c, timeop.IntervalOf(1, timeop.IntervalDay)).Lt(when)
		},
		"trunc": func(c momo.Column[time.Time]) momo.Predicate { return timeop.Trunc(timeop.TruncDay, c).Eq(when) },
	}
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		for name, mk := range build {
			for _, sensitive := range []bool{true, false} {
				label := "plain"
				if sensitive {
					label = "sensitive"
				}
				t.Run(string(d)+"/"+name+"/"+label, func(t *testing.T) {
					var flags uint16
					if sensitive {
						flags = momo.FlagSensitive
					}
					col := momo.NewColumn[time.Time]("documents", "reviewed_at", flags)
					rec := &obstest.Recorder{}
					drv := fakedriver.New(d)
					db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
					if _, err := momo.Select[suite.Document](db).Where(mk(col)).All(t.Context()); err != nil {
						t.Fatal(err)
					}
					evs, calls := rec.Events(), drv.Calls()
					if len(evs) == 0 || len(calls) == 0 {
						t.Fatal("no statement recorded")
					}
					seen, sent := evs[len(evs)-1].Args, calls[len(calls)-1].Args
					if len(seen) != 1 || len(sent) != 1 {
						t.Fatalf("want one argument, hook saw %v, driver got %v", seen, sent)
					}
					if sent[0] == momo.RedactedArg {
						t.Fatal("the driver received the placeholder")
					}
					if got := seen[0] == momo.RedactedArg; got != sensitive {
						t.Fatalf("hook saw %v, masked = %v, want masked = %v", seen[0], got, sensitive)
					}
				})
			}
		}
	}
}
