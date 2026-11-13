// SPDX-License-Identifier: Apache-2.0

package timeop_test

import (
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/timeop"
	"github.com/azrtydxb/momo/types"
)

var tmDue = momo.NewColumn[types.Date]("documents", "due", 0)

var tmDueB = momo.NewColumn[types.Date]("documents", "due_b", 0)

const (
	tmD  = `"documents"."due"`
	tmDB = `"documents"."due_b"`
)

func tmDate(y int, m time.Month, d int) types.Date {
	return types.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// Catches: a wrong operator or function per dialect for date plus days, a
// missing integer cast on the PostgreSQL parameter (PostgreSQL cannot choose
// between date + integer and date + interval for an untyped parameter), a
// missing date cast on a bound date, and a SQLite modifier with the wrong sign.
func TestAddDaysGolden(t *testing.T) {
	bound := momo.Value(int32(5))
	day := momo.Value(tmDate(2028, 2, 28))
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.AddDays(tmDue, 3), `(` + tmD + ` + CAST($1 AS integer))`, []any{int32(3)}},
		{dialect.Postgres, timeop.AddDays(day, -3), `(CAST($1 AS date) + CAST($2 AS integer))`, []any{tmDate(2028, 2, 28), int32(-3)}},
		{dialect.Postgres, timeop.AddDaysExpr(tmDue, bound), `(` + tmD + ` + CAST($1 AS integer))`, []any{int32(5)}},
		{dialect.Postgres, timeop.AddDaysExpr(tmDue, momo.Typed[int32](tmDueB)), `(` + tmD + ` + ` + tmDB + `)`, nil},
		{dialect.MySQL, timeop.AddDays(tmDue, 3), `DATE_ADD(` + tmD + `, INTERVAL ? DAY)`, []any{int32(3)}},
		{dialect.MySQL, timeop.AddDaysExpr(tmDue, bound), `DATE_ADD(` + tmD + `, INTERVAL ? DAY)`, []any{int32(5)}},
		{dialect.SQLite, timeop.AddDays(tmDue, 3), `date(` + tmD + `, '+3 days')`, nil},
		{dialect.SQLite, timeop.AddDays(tmDue, -3), `date(` + tmD + `, '-3 days')`, nil},
		{dialect.SQLite, timeop.AddDays(tmDue, 0), `date(` + tmD + `, '+0 days')`, nil},
		{dialect.SQLite, timeop.AddDaysExpr(tmDue, bound), `date(` + tmD + `, CAST(? AS INTEGER) || ' days')`, []any{int32(5)}},
	})
}

// Catches: a wrong operator or function per dialect for the difference of two
// dates, a missing date cast on a bound operand, and a SQLite cast that does
// not produce an integer.
func TestDaysBetweenGolden(t *testing.T) {
	day := momo.Value(tmDate(2028, 2, 28))
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.DaysBetween(tmDue, tmDueB), `(` + tmD + ` - ` + tmDB + `)`, nil},
		{dialect.Postgres, timeop.DaysBetween(tmDue, day), `(` + tmD + ` - CAST($1 AS date))`, []any{tmDate(2028, 2, 28)}},
		{dialect.MySQL, timeop.DaysBetween(tmDue, tmDueB), `DATEDIFF(` + tmD + `, ` + tmDB + `)`, nil},
		{dialect.SQLite, timeop.DaysBetween(tmDue, tmDueB), `CAST(julianday(` + tmD + `) - julianday(` + tmDB + `) AS INTEGER)`, nil},
	})
}

// Catches: a date arithmetic result that needs momo.Typed to be compared, and a
// nil operand that renders broken SQL instead of failing the build.
func TestDateArithmeticTypes(t *testing.T) {
	// These compile only when the results are Expression[types.Date] and
	// Expression[int32].
	_ = tmDue.LeExpr(timeop.AddDays(tmDueB, 3))
	_ = timeop.DaysBetween(tmDue, tmDueB).Lt(5)
	_ = timeop.AddDays(tmDue, 1).As("next")
	_ = timeop.DaysBetween(tmDue, tmDueB).As("left")

	for _, d := range typedDialects {
		for _, e := range []momo.Expr{
			timeop.AddDays(nil, 1),
			timeop.AddDaysExpr(tmDue, nil),
			timeop.DaysBetween(tmDue, nil),
		} {
			if _, err := selectOne(d, e); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s: nil operand = %v, want ErrValidation", d, err)
			}
		}
	}
}

// Catches: an infinite date bound into SQLite or MySQL date arithmetic, where
// the database would return NULL silently, and a day count outside the int32
// range.
func TestDateArithmeticRejections(t *testing.T) {
	inf := momo.Value(types.Date{Infinity: true})
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if _, err := selectOne(d, timeop.AddDays(inf, 1)); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: AddDays over infinity = %v, want ErrValidation", d, err)
		}
		if _, err := selectOne(d, timeop.DaysBetween(tmDue, inf)); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: DaysBetween over infinity = %v, want ErrValidation", d, err)
		}
	}
	// PostgreSQL has the special dates; the database decides.
	if _, err := selectOne(dialect.Postgres, timeop.AddDays(inf, 1)); err != nil {
		t.Errorf("postgres: AddDays over infinity = %v, want it to build", err)
	}
	for _, d := range typedDialects {
		if _, err := selectOne(d, timeop.AddDays(tmDue, 1<<40)); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: AddDays(1<<40) = %v, want ErrValidation", d, err)
		}
	}
}
