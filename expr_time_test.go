// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

var tmCreated = momo.NewColumn[time.Time]("documents", "created_at", 0)

const tmC = `"documents"."created_at"`

// selectOne compiles SELECT <e> FROM documents for d.
func selectOne(d dialect.Name, e momo.Expr) (momo.Compiled, error) {
	return momo.Select[unitTally](typedDB(d)).From(momo.TableRef{Name: "documents"}).Columns(e).SQL()
}

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
		{dialect.Postgres, momo.Extract(momo.ExtractYear, tmCreated), `CAST(EXTRACT(YEAR FROM ` + tmC + `) AS double precision)`, nil},
		{dialect.Postgres, momo.Extract(momo.ExtractEpoch, tmCreated), `CAST(EXTRACT(EPOCH FROM ` + tmC + `) AS double precision)`, nil},
		{dialect.Postgres, momo.Extract(momo.ExtractISODow, tmCreated), `CAST(EXTRACT(ISODOW FROM ` + tmC + `) AS double precision)`, nil},
		{dialect.SQLite, momo.Extract(momo.ExtractYear, tmCreated), `CAST(strftime('%Y', ` + tmC + `) AS REAL)`, nil},
		{dialect.SQLite, momo.Extract(momo.ExtractDoy, tmCreated), `CAST(strftime('%j', ` + tmC + `) AS REAL)`, nil},
		{dialect.SQLite, momo.Extract(momo.ExtractEpoch, tmCreated), `CAST(strftime('%s', ` + tmC + `) AS REAL)`, nil},
		{dialect.SQLite, momo.Extract(momo.ExtractQuarter, tmCreated), `CAST((CAST(strftime('%m', ` + tmC + `) AS INTEGER) + 2) / 3 AS REAL)`, nil},
		{dialect.SQLite, momo.Extract(momo.ExtractISODow, tmCreated), `CAST((CAST(strftime('%w', ` + tmC + `) AS INTEGER) + 6) % 7 + 1 AS REAL)`, nil},
		{dialect.MySQL, momo.Extract(momo.ExtractMonth, tmCreated), `(EXTRACT(MONTH FROM ` + tmC + `) + 0e0)`, nil},
		{dialect.MySQL, momo.Extract(momo.ExtractDow, tmCreated), `(DAYOFWEEK(` + tmC + `) - 1 + 0e0)`, nil},
		{dialect.MySQL, momo.Extract(momo.ExtractISODow, tmCreated), `(WEEKDAY(` + tmC + `) + 1 + 0e0)`, nil},
		{dialect.MySQL, momo.Extract(momo.ExtractEpoch, tmCreated), `(UNIX_TIMESTAMP(` + tmC + `) + 0e0)`, nil},
	})
}

// Catches: an Extract field or interval or truncation unit reaching the SQL
// text without passing the allowlist, on every dialect.
func TestTimeKeywordsValidated(t *testing.T) {
	bad := `year FROM x); DROP TABLE documents; --`
	exprs := map[string]momo.Expr{
		"extract":   momo.Extract(momo.ExtractField(bad), tmCreated),
		"interval":  momo.AddInterval(tmCreated, momo.IntervalOf(1, momo.IntervalUnit(bad))),
		"intervalx": momo.SubInterval(tmCreated, momo.IntervalExpr(momo.Value(int64(1)), momo.IntervalUnit("days"))),
		"trunc":     momo.DateTrunc(momo.TruncUnit(bad), tmCreated),
		"truncCase": momo.DateTrunc(momo.TruncUnit("DAY"), tmCreated),
		"zero":      momo.AddInterval(tmCreated, momo.Interval{}),
		"huge":      momo.AddInterval(tmCreated, momo.IntervalOf(1<<62, momo.IntervalWeek)),
		"nil":       momo.Extract(momo.ExtractYear, nil),
		"tzEmpty":   momo.AtTimeZone(tmCreated, ""),
		"tzQuote":   momo.AtTimeZone(tmCreated, "UTC'); DROP TABLE documents; --"),
		"tzSpace":   momo.AtTimeZone(tmCreated, "Europe Brussels"),
		"tzNul":     momo.AtTimeZone(tmCreated, "UTC\x00"),
		"tzLong":    momo.AtTimeZone(tmCreated, strings.Repeat("a", 65)),
		"ivNilExpr": momo.AddInterval(tmCreated, momo.IntervalExpr(nil, momo.IntervalDay)),
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
		{dialect.Postgres, momo.AtTimeZone(tmCreated, "Europe/Brussels"), `(` + tmC + ` AT TIME ZONE 'Europe/Brussels')`, nil},
		{dialect.Postgres, momo.AtTimeZone(tmCreated, "Etc/GMT+5"), `(` + tmC + ` AT TIME ZONE 'Etc/GMT+5')`, nil},
		{dialect.Postgres, momo.Extract(momo.ExtractYear, momo.Value(time.Unix(0, 0).UTC())), `CAST(EXTRACT(YEAR FROM CAST($1 AS timestamptz)) AS double precision)`, []any{time.Unix(0, 0).UTC()}},
		{dialect.Postgres, momo.DateTrunc(momo.TruncDay, momo.Value(time.Unix(0, 0).UTC())), `date_trunc('day', CAST($1 AS timestamptz))`, []any{time.Unix(0, 0).UTC()}},
		{dialect.Postgres, momo.AtTimeZoneExpr(tmCreated, DocTitle), `(` + tmC + ` AT TIME ZONE "documents"."title")`, nil},
	})
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if _, err := selectOne(d, momo.AtTimeZone(tmCreated, "UTC")); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s AtTimeZone err = %v, want ErrUnsupported", d, err)
		}
	}
}

// Catches: wrong sign, unit word or placeholder for interval arithmetic, a
// week that is not scaled to days on SQLite, and a bound count that is not
// bound.
func TestIntervalGolden(t *testing.T) {
	const sqliteFmt = `'%Y-%m-%d %H:%M:%f'`
	five := momo.IntervalOf(5, momo.IntervalDay)
	bound := momo.IntervalExpr(momo.Value(int64(3)), momo.IntervalWeek)
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, momo.AddInterval(tmCreated, five), `(` + tmC + ` + INTERVAL '5 day')`, nil},
		{dialect.Postgres, momo.SubInterval(tmCreated, five), `(` + tmC + ` - INTERVAL '5 day')`, nil},
		{dialect.Postgres, momo.AddInterval(tmCreated, bound), `(` + tmC + ` + (CAST($1 AS bigint) * INTERVAL '1 week'))`, []any{int64(3)}},
		{dialect.Postgres, momo.AddInterval(tmCreated, momo.IntervalOf(2, momo.IntervalMonth)), `(` + tmC + ` + INTERVAL '2 month')`, nil},
		{dialect.MySQL, momo.AddInterval(tmCreated, five), `DATE_ADD(` + tmC + `, INTERVAL 5 DAY)`, nil},
		{dialect.MySQL, momo.SubInterval(tmCreated, five), `DATE_SUB(` + tmC + `, INTERVAL 5 DAY)`, nil},
		{dialect.MySQL, momo.SubInterval(tmCreated, bound), `DATE_SUB(` + tmC + `, INTERVAL ? WEEK)`, []any{int64(3)}},
		{dialect.SQLite, momo.AddInterval(tmCreated, five), `strftime(` + sqliteFmt + `, ` + tmC + `, '5 days')`, nil},
		{dialect.SQLite, momo.SubInterval(tmCreated, momo.IntervalOf(2, momo.IntervalWeek)), `strftime(` + sqliteFmt + `, ` + tmC + `, '-14 days')`, nil},
		{dialect.SQLite, momo.AddInterval(tmCreated, momo.IntervalOf(1, momo.IntervalYear)), `strftime(` + sqliteFmt + `, ` + tmC + `, '1 years')`, nil},
		{dialect.SQLite, momo.SubInterval(tmCreated, bound), `strftime(` + sqliteFmt + `, ` + tmC + `, CAST((?) * -7 AS TEXT) || ' days')`, []any{int64(3)}},
	})
}

// Catches: Now and CurrentTimestamp rendering a function the dialect lacks.
func TestNowGolden(t *testing.T) {
	const sqliteNow = `strftime('%Y-%m-%d %H:%M:%f', 'now')`
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, momo.Now(), `now()`, nil},
		{dialect.Postgres, momo.CurrentTimestamp(), `CURRENT_TIMESTAMP`, nil},
		{dialect.MySQL, momo.Now(), `NOW(6)`, nil},
		{dialect.MySQL, momo.CurrentTimestamp(), `CURRENT_TIMESTAMP(6)`, nil},
		{dialect.SQLite, momo.Now(), sqliteNow, nil},
		{dialect.SQLite, momo.CurrentTimestamp(), sqliteNow, nil},
	})
}

// Catches: a wrong truncation layout, a week that does not start on Monday,
// and a quarter that is not the first month of its quarter.
func TestDateTruncGolden(t *testing.T) {
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, momo.DateTrunc(momo.TruncDay, tmCreated), `date_trunc('day', ` + tmC + `)`, nil},
		{dialect.Postgres, momo.DateTrunc(momo.TruncQuarter, tmCreated), `date_trunc('quarter', ` + tmC + `)`, nil},
		{dialect.SQLite, momo.DateTrunc(momo.TruncDay, tmCreated), `strftime('%Y-%m-%d 00:00:00.000', ` + tmC + `)`, nil},
		{dialect.SQLite, momo.DateTrunc(momo.TruncHour, tmCreated), `strftime('%Y-%m-%d %H:00:00.000', ` + tmC + `)`, nil},
		{dialect.SQLite, momo.DateTrunc(momo.TruncMonth, tmCreated), `strftime('%Y-%m-01 00:00:00.000', ` + tmC + `)`, nil},
		{dialect.SQLite, momo.DateTrunc(momo.TruncWeek, tmCreated), `strftime('%Y-%m-%d 00:00:00.000', ` + tmC + `, 'start of day', '+1 day', 'weekday 1', '-7 days')`, nil},
		{dialect.SQLite, momo.DateTrunc(momo.TruncQuarter, tmCreated), `(strftime('%Y-', ` + tmC + `) || printf('%02d', (CAST(strftime('%m', ` + tmC + `) AS INTEGER) - 1) / 3 * 3 + 1) || '-01 00:00:00.000')`, nil},
		{dialect.MySQL, momo.DateTrunc(momo.TruncMinute, tmCreated), `CAST(DATE_FORMAT(` + tmC + `, '%Y-%m-%d %H:%i:00') AS DATETIME)`, nil},
		{dialect.MySQL, momo.DateTrunc(momo.TruncYear, tmCreated), `CAST(DATE_FORMAT(` + tmC + `, '%Y-01-01') AS DATETIME)`, nil},
		{dialect.MySQL, momo.DateTrunc(momo.TruncWeek, tmCreated), `CAST(DATE_FORMAT(DATE_SUB(` + tmC + `, INTERVAL WEEKDAY(` + tmC + `) DAY), '%Y-%m-%d') AS DATETIME)`, nil},
		{dialect.MySQL, momo.DateTrunc(momo.TruncQuarter, tmCreated), `CAST(CONCAT(YEAR(` + tmC + `), '-', (QUARTER(` + tmC + `) - 1) * 3 + 1, '-01') AS DATETIME)`, nil},
	})
}

// Catches: time expressions that cannot be filtered, grouped, ordered or
// aliased like a column, and a GROUP BY expression that renders differently
// from the select-list one (PostgreSQL rejects the grouping).
func TestTimeExprComposition(t *testing.T) {
	db := typedDB(dialect.Postgres)
	day := func() momo.TimeExpr { return momo.DateTrunc(momo.TruncDay, tmCreated) }
	got, err := momo.Select[unitTally](db).From(momo.TableRef{Name: "documents"}).
		Columns(day().As("d")).
		Where(tmCreated.GtExpr(momo.SubInterval(momo.Now(), momo.IntervalOf(7, momo.IntervalDay)))).
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
	local := func() momo.TimeExpr {
		return momo.DateTrunc(momo.TruncDay, momo.AtTimeZone(tmCreated, "Europe/Brussels"))
	}
	got, err = momo.Select[strCount](db).From(momo.TableRef{Name: "documents"}).
		Columns(local(), momo.CountAll()).GroupBy(local()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	lw := `date_trunc('day', (` + tmC + ` AT TIME ZONE 'Europe/Brussels'))`
	if want := `SELECT ` + lw + `, COUNT(*) FROM "documents" GROUP BY ` + lw; got.SQL != want || len(got.Args) != 0 {
		t.Errorf("local day:\n got %s %v\nwant %s", got.SQL, got.Args, want)
	}
	if _, err := selectOne(dialect.Postgres, momo.Extract(momo.ExtractEpoch, day())); err != nil {
		t.Errorf("extract over date_trunc: %v", err)
	}
}

func joinRender(d dialect.Name, e momo.Expr) (momo.Compiled, error) { return selectOne(d, e) }

// Catches: parts that lose their arguments or numbering when joined, lost
// parentheses (an OR part regrouping an AND join), and a comma join that
// wraps its parts.
func TestJoinSQLGolden(t *testing.T) {
	a := momo.SQL("status = ? OR kind = ?", "x", "y")
	b := momo.SQL("owner_id = ?", int64(7))
	got, err := joinRender(dialect.Postgres, momo.JoinSQL(" and ", a, b))
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT ((status = $1 OR kind = $2) AND (owner_id = $3)) FROM "documents"`; got.SQL != want {
		t.Errorf("and:\n got %s\nwant %s", got.SQL, want)
	}
	if !reflect.DeepEqual(got.Args, []any{"x", "y", int64(7)}) {
		t.Errorf("args = %v", got.Args)
	}

	got, err = joinRender(dialect.SQLite, momo.JoinSQL(",", momo.SQL("a"), momo.SQL("b + ?", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT a, b + ? FROM "documents"`; got.SQL != want || len(got.Args) != 1 {
		t.Errorf("comma: got %s %v, want %s", got.SQL, got.Args, want)
	}

	// Inside a larger statement the binds continue the outer numbering.
	q, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
		Columns(momo.CountAll()).
		Where(DocTenantID.Eq(9), momo.JoinSQL(" OR ", momo.SQL("a = ?", 1), momo.SQL("b = ?", 2))).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.SQL, `"documents"."tenant_id" = $1`) || !strings.Contains(q.SQL, `((a = $2) OR (b = $3))`) || len(q.Args) != 3 {
		t.Errorf("where: %s %v", q.SQL, q.Args)
	}

	got, err = joinRender(dialect.MySQL, momo.JoinSQL("||", momo.SQL("a"), momo.SQL("b")))
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT CONCAT((a), (b)) FROM `documents`"; got.SQL != want {
		t.Errorf("mysql concat:\n got %s\nwant %s", got.SQL, want)
	}
	got, err = joinRender(dialect.Postgres, momo.JoinSQL("||", momo.SQL("a"), momo.SQL("b")))
	if err != nil || got.SQL != `SELECT ((a) || (b)) FROM "documents"` {
		t.Errorf("pg concat: %s, %v", got.SQL, err)
	}
	for sep, want := range map[string]string{" AND ": "1 = 1", " or ": "1 = 0"} {
		got, err = joinRender(dialect.Postgres, momo.JoinSQL(sep))
		if err != nil || got.SQL != `SELECT `+want+` FROM "documents"` {
			t.Errorf("empty %q join: %s, %v; want %s", sep, got.SQL, err, want)
		}
	}

	typedJoin := momo.JoinTypedSQL[int64]("+", momo.SQL("a"), momo.SQL("b"))
	if _, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
		Columns(typedJoin).SQL(); err != nil {
		t.Errorf("typed join: %v", err)
	}
}

// Catches: a separator carrying caller SQL, and degenerate joins rendering
// broken SQL instead of failing the build.
func TestJoinSQLValidation(t *testing.T) {
	a := momo.SQL("a")
	for name, e := range map[string]momo.Expr{
		"injection": momo.JoinSQL(" AND 1=1; DROP TABLE documents; -- ", a, a),
		"comment":   momo.JoinSQL("--", a, a),
		"word":      momo.JoinSQL("UNION", a, a),
		"empty":     momo.JoinSQL(","),
		"nil":       momo.JoinSQL(",", a, nil),
	} {
		if _, err := joinRender(dialect.Postgres, e); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}
