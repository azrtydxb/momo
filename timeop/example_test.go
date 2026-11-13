// SPDX-License-Identifier: Apache-2.0

package timeop_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/timeop"
	"github.com/azrtydxb/momo/types"
)

// Extract reads one part of a timestamp. Every dialect renders its own
// spelling, and the field is checked against an allowlist.
func ExampleExtract() {
	created := momo.NewColumn[time.Time]("docs", "created_at", 0)

	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		b := momo.NewBuilder(dialect.For(d))
		timeop.Extract(timeop.ExtractYear, created).AppendSQL(b)
		fmt.Println(b.String())
	}
	// Output:
	// CAST(EXTRACT(YEAR FROM "docs"."created_at") AS double precision)
	// CAST(strftime('%Y', "docs"."created_at") AS REAL)
}

// Trunc buckets a timestamp, and the unit is written as a literal so the
// select list and GROUP BY render the same text.
func ExampleTrunc() {
	created := momo.NewColumn[time.Time]("docs", "created_at", 0)

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	timeop.Trunc(timeop.TruncDay, created).AppendSQL(b)
	fmt.Println(b.String())
	// Output:
	// date_trunc('day', "docs"."created_at")
}

// AddInterval and SubInterval move a timestamp by an interval. The last month
// day differs between dialects: PostgreSQL and MySQL clamp it, SQLite rolls
// into the next month.
func ExampleAddInterval() {
	created := momo.NewColumn[time.Time]("docs", "created_at", 0)
	week := timeop.IntervalOf(1, timeop.IntervalWeek)

	b := momo.NewBuilder(dialect.For(dialect.MySQL))
	timeop.SubInterval(timeop.Now(), week).AppendSQL(b)
	fmt.Println(b.String())

	b = momo.NewBuilder(dialect.For(dialect.Postgres))
	timeop.AddInterval(created, week).AppendSQL(b)
	fmt.Println(b.String())
	// Output:
	// DATE_SUB(NOW(6), INTERVAL 1 WEEK)
	// ("docs"."created_at" + INTERVAL '1 week')
}

// AtTimeZone converts between wall-clock and zoned timestamps on PostgreSQL.
// Other dialects fail the build with ErrUnsupported.
func ExampleAtTimeZone() {
	created := momo.NewColumn[time.Time]("docs", "created_at", 0)
	expr := timeop.AtTimeZone(created, "Europe/Brussels")

	pg := momo.NewBuilder(dialect.For(dialect.Postgres))
	expr.AppendSQL(pg)
	fmt.Println(pg.String())

	lite := momo.NewBuilder(dialect.For(dialect.SQLite))
	expr.AppendSQL(lite)
	fmt.Println(errors.Is(lite.Err(), momo.ErrUnsupported))
	// Output:
	// ("docs"."created_at" AT TIME ZONE 'Europe/Brussels')
	// true
}

// AddDays and DaysBetween do date arithmetic on a types.Date with typed
// results, so the date is not retyped as an int32 to use a + or a -. This is
// "respond_by <= today + 3" and "respond_by - today" of an appeals dashboard:
//
//	due := respondBy.LeExpr(timeop.AddDays(momo.Value(today), 3))
//	left := timeop.DaysBetween(respondBy, momo.Value(today))
func ExampleAddDays() {
	respondBy := momo.NewColumn[types.Date]("appeals", "respond_by", 0)
	today := momo.Value(types.Date{Time: time.Date(2028, 2, 26, 0, 0, 0, 0, time.UTC)})

	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		b := momo.NewBuilder(dialect.For(d))
		timeop.AddDays(today, 3).AppendSQL(b)
		fmt.Println(b.String())
		b = momo.NewBuilder(dialect.For(d))
		timeop.DaysBetween(respondBy, today).AppendSQL(b)
		fmt.Println(b.String())
	}
	// Output:
	// (CAST($1 AS date) + CAST($2 AS integer))
	// ("appeals"."respond_by" - CAST($1 AS date))
	// date(?, '+3 days')
	// CAST(julianday("appeals"."respond_by") - julianday(?) AS INTEGER)
	// DATE(DATE_ADD(?, INTERVAL ? DAY))
	// DATEDIFF(`appeals`.`respond_by`, ?)
}
