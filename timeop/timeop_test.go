// SPDX-License-Identifier: Apache-2.0

package timeop_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/timeop"
)

var tmCreated = momo.NewColumn[time.Time]("documents", "created_at", 0)

var tmTitle = momo.NewColumn[string]("documents", "title", 0)

const tmC = `"documents"."created_at"`

// selectOne compiles SELECT <e> FROM documents for d.
func selectOne(d dialect.Name, e momo.Expr) (momo.Compiled, error) {
	return momo.Select[exprtest.Tally](exprtest.DB(d)).From(momo.TableRef{Name: "documents"}).Columns(e).SQL()
}

// quoteFor rewrites a PostgreSQL-style golden string for MySQL's backticks.
func quoteFor(d dialect.Name, s string) string {
	if d == dialect.MySQL {
		return strings.ReplaceAll(s, `"`, "`")
	}
	return s
}

var typedDialects = []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL}

type timeGolden struct {
	d    dialect.Name
	e    momo.Expr
	want string
	args []any
}

func runTimeGoldens(t *testing.T, cases []timeGolden) {
	t.Helper()
	for _, c := range cases {
		got, err := selectOne(c.d, c.e)
		if err != nil {
			t.Errorf("%s %s: %v", c.d, c.want, err)
			continue
		}
		want := quoteFor(c.d, "SELECT "+c.want+` FROM "documents"`)
		if got.SQL != want {
			t.Errorf("%s:\n got %s\nwant %s", c.d, got.SQL, want)
		}
		if len(c.args) != len(got.Args) || len(c.args) > 0 && !reflect.DeepEqual(got.Args, c.args) {
			t.Errorf("%s %s: args = %v, want %v", c.d, c.want, got.Args, c.args)
		}
	}
}

// Catches: a wrong keyword, cast or per-dialect spelling of Extract, and an
// EPOCH that loses its fraction through a missing double cast on PostgreSQL.
func TestExtractGolden(t *testing.T) {
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.Extract(timeop.ExtractYear, tmCreated), `CAST(EXTRACT(YEAR FROM ` + tmC + `) AS double precision)`, nil},
		{dialect.Postgres, timeop.Extract(timeop.ExtractEpoch, tmCreated), `CAST(EXTRACT(EPOCH FROM ` + tmC + `) AS double precision)`, nil},
		{dialect.Postgres, timeop.Extract(timeop.ExtractISODow, tmCreated), `CAST(EXTRACT(ISODOW FROM ` + tmC + `) AS double precision)`, nil},
		{dialect.SQLite, timeop.Extract(timeop.ExtractYear, tmCreated), `CAST(strftime('%Y', ` + tmC + `) AS REAL)`, nil},
		{dialect.SQLite, timeop.Extract(timeop.ExtractDoy, tmCreated), `CAST(strftime('%j', ` + tmC + `) AS REAL)`, nil},
		{dialect.SQLite, timeop.Extract(timeop.ExtractEpoch, tmCreated), `CAST(strftime('%s', ` + tmC + `) AS REAL)`, nil},
		{dialect.SQLite, timeop.Extract(timeop.ExtractQuarter, tmCreated), `CAST((CAST(strftime('%m', ` + tmC + `) AS INTEGER) + 2) / 3 AS REAL)`, nil},
		{dialect.SQLite, timeop.Extract(timeop.ExtractISODow, tmCreated), `CAST((CAST(strftime('%w', ` + tmC + `) AS INTEGER) + 6) % 7 + 1 AS REAL)`, nil},
		{dialect.MySQL, timeop.Extract(timeop.ExtractMonth, tmCreated), `(EXTRACT(MONTH FROM ` + tmC + `) + 0e0)`, nil},
		{dialect.MySQL, timeop.Extract(timeop.ExtractDow, tmCreated), `(DAYOFWEEK(` + tmC + `) - 1 + 0e0)`, nil},
		{dialect.MySQL, timeop.Extract(timeop.ExtractISODow, tmCreated), `(WEEKDAY(` + tmC + `) + 1 + 0e0)`, nil},
		{dialect.MySQL, timeop.Extract(timeop.ExtractEpoch, tmCreated), `(UNIX_TIMESTAMP(` + tmC + `) + 0e0)`, nil},
	})
}

// Catches: an Extract field or interval or truncation unit reaching the SQL
// text without passing the allowlist, on every dialect.
func TestTimeKeywordsValidated(t *testing.T) {
	bad := `year FROM x); DROP TABLE documents; --`
	exprs := map[string]momo.Expr{
		"extract":   timeop.Extract(timeop.ExtractField(bad), tmCreated),
		"interval":  timeop.AddInterval(tmCreated, timeop.IntervalOf(1, timeop.IntervalUnit(bad))),
		"intervalx": timeop.SubInterval(tmCreated, timeop.IntervalExpr(momo.Value(int64(1)), timeop.IntervalUnit("days"))),
		"trunc":     timeop.Trunc(timeop.TruncUnit(bad), tmCreated),
		"truncCase": timeop.Trunc(timeop.TruncUnit("DAY"), tmCreated),
		"zero":      timeop.AddInterval(tmCreated, timeop.Interval{}),
		"huge":      timeop.AddInterval(tmCreated, timeop.IntervalOf(1<<62, timeop.IntervalWeek)),
		"nil":       timeop.Extract(timeop.ExtractYear, nil),
		"tzEmpty":   timeop.AtTimeZone(tmCreated, ""),
		"tzQuote":   timeop.AtTimeZone(tmCreated, "UTC'); DROP TABLE documents; --"),
		"tzSpace":   timeop.AtTimeZone(tmCreated, "Europe Brussels"),
		"tzNul":     timeop.AtTimeZone(tmCreated, "UTC\x00"),
		"tzLong":    timeop.AtTimeZone(tmCreated, strings.Repeat("a", 65)),
		"ivNilExpr": timeop.AddInterval(tmCreated, timeop.IntervalExpr(nil, timeop.IntervalDay)),
	}
	for _, d := range typedDialects {
		for name, e := range exprs {
			got, err := selectOne(d, e)
			if !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s %s: err = %v sql = %q, want ErrValidation", d, name, err, got.SQL)
			}
		}
	}
}

// Catches: the zone interpolated into SQL instead of bound, and AT TIME ZONE
// rendered on engines without a portable equivalent.
func TestAtTimeZone(t *testing.T) {
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.AtTimeZone(tmCreated, "Europe/Brussels"), `(` + tmC + ` AT TIME ZONE 'Europe/Brussels')`, nil},
		{dialect.Postgres, timeop.AtTimeZone(tmCreated, "Etc/GMT+5"), `(` + tmC + ` AT TIME ZONE 'Etc/GMT+5')`, nil},
		{dialect.Postgres, timeop.Extract(timeop.ExtractYear, momo.Value(time.Unix(0, 0).UTC())), `CAST(EXTRACT(YEAR FROM CAST($1 AS timestamptz)) AS double precision)`, []any{time.Unix(0, 0).UTC()}},
		{dialect.Postgres, timeop.Trunc(timeop.TruncDay, momo.Value(time.Unix(0, 0).UTC())), `date_trunc('day', CAST($1 AS timestamptz))`, []any{time.Unix(0, 0).UTC()}},
		{dialect.Postgres, timeop.AtTimeZoneExpr(tmCreated, tmTitle), `(` + tmC + ` AT TIME ZONE "documents"."title")`, nil},
	})
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if _, err := selectOne(d, timeop.AtTimeZone(tmCreated, "UTC")); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s AtTimeZone err = %v, want ErrUnsupported", d, err)
		}
	}
}

// Catches: wrong sign, unit word or placeholder for interval arithmetic, a
// week that is not scaled to days on SQLite, and a bound count that is not
// bound.
func TestIntervalGolden(t *testing.T) {
	const sqliteFmt = `'%Y-%m-%d %H:%M:%f'`
	five := timeop.IntervalOf(5, timeop.IntervalDay)
	bound := timeop.IntervalExpr(momo.Value(int64(3)), timeop.IntervalWeek)
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.AddInterval(tmCreated, five), `(` + tmC + ` + INTERVAL '5 day')`, nil},
		{dialect.Postgres, timeop.SubInterval(tmCreated, five), `(` + tmC + ` - INTERVAL '5 day')`, nil},
		{dialect.Postgres, timeop.AddInterval(tmCreated, bound), `(` + tmC + ` + (CAST($1 AS bigint) * INTERVAL '1 week'))`, []any{int64(3)}},
		{dialect.Postgres, timeop.AddInterval(tmCreated, timeop.IntervalOf(2, timeop.IntervalMonth)), `(` + tmC + ` + INTERVAL '2 month')`, nil},
		{dialect.MySQL, timeop.AddInterval(tmCreated, five), `DATE_ADD(` + tmC + `, INTERVAL 5 DAY)`, nil},
		{dialect.MySQL, timeop.SubInterval(tmCreated, five), `DATE_SUB(` + tmC + `, INTERVAL 5 DAY)`, nil},
		{dialect.MySQL, timeop.SubInterval(tmCreated, bound), `DATE_SUB(` + tmC + `, INTERVAL ? WEEK)`, []any{int64(3)}},
		{dialect.MySQL, timeop.AddInterval(momo.Value(time.Unix(0, 0).UTC()), five), `DATE_ADD(CAST(? AS DATETIME(6)), INTERVAL 5 DAY)`, []any{time.Unix(0, 0).UTC()}},
		{dialect.SQLite, timeop.AddInterval(tmCreated, five), `strftime(` + sqliteFmt + `, ` + tmC + `, '5 days')`, nil},
		{dialect.SQLite, timeop.SubInterval(tmCreated, timeop.IntervalOf(2, timeop.IntervalWeek)), `strftime(` + sqliteFmt + `, ` + tmC + `, '-14 days')`, nil},
		{dialect.SQLite, timeop.AddInterval(tmCreated, timeop.IntervalOf(1, timeop.IntervalYear)), `strftime(` + sqliteFmt + `, ` + tmC + `, '1 years')`, nil},
		{dialect.SQLite, timeop.SubInterval(tmCreated, bound), `strftime(` + sqliteFmt + `, ` + tmC + `, CAST((?) * -7 AS TEXT) || ' days')`, []any{int64(3)}},
	})
}

// Catches: Now and CurrentTimestamp rendering a function the dialect lacks.
func TestNowGolden(t *testing.T) {
	const sqliteNow = `strftime('%Y-%m-%d %H:%M:%f', 'now')`
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.Now(), `now()`, nil},
		{dialect.Postgres, timeop.CurrentTimestamp(), `CURRENT_TIMESTAMP`, nil},
		{dialect.MySQL, timeop.Now(), `NOW(6)`, nil},
		{dialect.MySQL, timeop.CurrentTimestamp(), `CURRENT_TIMESTAMP(6)`, nil},
		{dialect.SQLite, timeop.Now(), sqliteNow, nil},
		{dialect.SQLite, timeop.CurrentTimestamp(), sqliteNow, nil},
	})
}

// Catches: a wrong truncation layout, a week that does not start on Monday,
// and a quarter that is not the first month of its quarter.
func TestDateTruncGolden(t *testing.T) {
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.Trunc(timeop.TruncDay, tmCreated), `date_trunc('day', ` + tmC + `)`, nil},
		{dialect.Postgres, timeop.Trunc(timeop.TruncQuarter, tmCreated), `date_trunc('quarter', ` + tmC + `)`, nil},
		{dialect.SQLite, timeop.Trunc(timeop.TruncDay, tmCreated), `strftime('%Y-%m-%d 00:00:00.000', ` + tmC + `)`, nil},
		{dialect.SQLite, timeop.Trunc(timeop.TruncHour, tmCreated), `strftime('%Y-%m-%d %H:00:00.000', ` + tmC + `)`, nil},
		{dialect.SQLite, timeop.Trunc(timeop.TruncMonth, tmCreated), `strftime('%Y-%m-01 00:00:00.000', ` + tmC + `)`, nil},
		{dialect.SQLite, timeop.Trunc(timeop.TruncWeek, tmCreated), `strftime('%Y-%m-%d 00:00:00.000', ` + tmC + `, 'start of day', '+1 day', 'weekday 1', '-7 days')`, nil},
		{dialect.SQLite, timeop.Trunc(timeop.TruncQuarter, tmCreated), `(strftime('%Y-', ` + tmC + `) || printf('%02d', (CAST(strftime('%m', ` + tmC + `) AS INTEGER) - 1) / 3 * 3 + 1) || '-01 00:00:00.000')`, nil},
		{dialect.MySQL, timeop.Trunc(timeop.TruncMinute, tmCreated), `CAST(DATE_FORMAT(` + tmC + `, '%Y-%m-%d %H:%i:00') AS DATETIME)`, nil},
		{dialect.MySQL, timeop.Trunc(timeop.TruncYear, tmCreated), `CAST(DATE_FORMAT(` + tmC + `, '%Y-01-01') AS DATETIME)`, nil},
		{dialect.MySQL, timeop.Trunc(timeop.TruncWeek, tmCreated), `CAST(DATE_FORMAT(DATE_SUB(` + tmC + `, INTERVAL WEEKDAY(` + tmC + `) DAY), '%Y-%m-%d') AS DATETIME)`, nil},
		{dialect.MySQL, timeop.Trunc(timeop.TruncQuarter, tmCreated), `CAST(CONCAT(YEAR(` + tmC + `), '-', (QUARTER(` + tmC + `) - 1) * 3 + 1, '-01') AS DATETIME)`, nil},
	})
}

// Catches: time expressions that cannot be filtered, grouped, ordered or
// aliased like a column, and a GROUP BY expression that renders differently
// from the select-list one (PostgreSQL rejects the grouping).
func TestTimeExprComposition(t *testing.T) {
	db := exprtest.DB(dialect.Postgres)
	day := func() timeop.Expr { return timeop.Trunc(timeop.TruncDay, tmCreated) }
	got, err := momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "documents"}).
		Columns(day().As("d")).
		Where(tmCreated.GtExpr(timeop.SubInterval(timeop.Now(), timeop.IntervalOf(7, timeop.IntervalDay)))).
		GroupBy(day()).OrderBy(day().Desc()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT date_trunc('day', ` + tmC + `) AS "d" FROM "documents" WHERE ` + tmC +
		` > (now() - INTERVAL '7 day') GROUP BY date_trunc('day', ` + tmC + `) ORDER BY date_trunc('day', ` + tmC + `) DESC`
	if got.SQL != want {
		t.Errorf("composition:\n got %s\nwant %s", got.SQL, want)
	}
	// A zone-converted bucket groups with identical text in both clauses.
	local := func() timeop.Expr {
		return timeop.Trunc(timeop.TruncDay, timeop.AtTimeZone(tmCreated, "Europe/Brussels"))
	}
	got, err = momo.Select[exprtest.TextTally](db).From(momo.TableRef{Name: "documents"}).
		Columns(local(), momo.CountAll()).GroupBy(local()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	lw := `date_trunc('day', (` + tmC + ` AT TIME ZONE 'Europe/Brussels'))`
	if want := `SELECT ` + lw + `, COUNT(*) FROM "documents" GROUP BY ` + lw; got.SQL != want || len(got.Args) != 0 {
		t.Errorf("local day:\n got %s %v\nwant %s", got.SQL, got.Args, want)
	}
	if _, err := selectOne(dialect.Postgres, timeop.Extract(timeop.ExtractEpoch, day())); err != nil {
		t.Errorf("extract over date_trunc: %v", err)
	}
}
