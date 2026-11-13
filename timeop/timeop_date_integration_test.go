// SPDX-License-Identifier: Apache-2.0

//go:build integration

package timeop_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/timeop"
	"github.com/azrtydxb/momo/types"
)

var (
	tdID = momo.NewColumn[int64]("timeop_days", "id", 0)
	tdD  = momo.NewColumn[types.Date]("timeop_days", "d", 0)
	tdN  = momo.NewColumn[int32]("timeop_days", "n", 0)
)

type tdDate struct{ D types.Date }

var tdDateMeta = momo.Meta[tdDate]{
	Columns:  []momo.ColumnMeta{{Name: "d"}},
	Ptr:      func(m *tdDate, _ int) any { return &m.D },
	Value:    func(m *tdDate, _ int) any { return m.D },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*tdDate) MomoMeta() *momo.Meta[tdDate] { return &tdDateMeta }

type tdInt struct{ N int32 }

var tdIntMeta = momo.Meta[tdInt]{
	Columns:  []momo.ColumnMeta{{Name: "n"}},
	Ptr:      func(m *tdInt, _ int) any { return &m.N },
	Value:    func(m *tdInt, _ int) any { return m.N },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*tdInt) MomoMeta() *momo.Meta[tdInt] { return &tdIntMeta }

// eachDateDB runs fn on PostgreSQL, SQLite, MySQL 8.4, MySQL 9.7 and MariaDB
// 11.8, one subtest each. testopen.Each covers only the first two, so the MySQL
// servers are opened by name; a server whose URL is unset is skipped, and
// failed under MOMO_REQUIRE_INTEGRATION=1 for the dialects the job requires.
func eachDateDB(t *testing.T, fn func(t *testing.T, db *momo.DB, d dialect.Name)) {
	t.Helper()
	for _, s := range []struct {
		name string
		d    dialect.Name
		open func(*testing.T, ...momo.Option) *momo.DB
	}{
		{"postgres", dialect.Postgres, testopen.Postgres},
		{"sqlite", dialect.SQLite, testopen.SQLite},
		{"mysql84", dialect.MySQL, testopen.MySQL84},
		{"mysql97", dialect.MySQL, testopen.MySQL97},
		{"mariadb118", dialect.MySQL, testopen.MariaDB},
	} {
		t.Run(s.name, func(t *testing.T) {
			fn(t, s.open(t), s.d)
		})
	}
}

// seedDays creates timeop_days: row 1 is 2028-02-28 (a leap year), row 2 is
// 2028-01-31 (a month end) and row 3 is 2028-12-31.
func seedDays(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Helper()
	typ := "TEXT"
	if d == dialect.Postgres {
		typ = "date"
	}
	for _, s := range []string{
		"DROP TABLE IF EXISTS timeop_days",
		"CREATE TABLE timeop_days (id bigint primary key, d " + typ + " not null, n integer not null)",
		"INSERT INTO timeop_days VALUES (1, '2028-02-28', 1)",
		"INSERT INTO timeop_days VALUES (2, '2028-01-31', 2)",
		"INSERT INTO timeop_days VALUES (3, '2028-12-31', 3)",
	} {
		if _, err := db.Driver().Exec(context.Background(), s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func dateAt(db *momo.DB, id int64, e momo.Expr) (types.Date, error) {
	r, err := momo.Select[tdDate](db).From(momo.TableRef{Name: "timeop_days"}).Columns(e).
		Where(tdID.Eq(id)).One(context.Background())
	if err != nil {
		return types.Date{}, err
	}
	return r.D, nil
}

func daysAt(db *momo.DB, id int64, e momo.Expr) (int32, error) {
	r, err := momo.Select[tdInt](db).From(momo.TableRef{Name: "timeop_days"}).Columns(e).
		Where(tdID.Eq(id)).One(context.Background())
	if err != nil {
		return 0, err
	}
	return r.N, nil
}

func wantDate(t *testing.T, label string, got types.Date, err error, want types.Date) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	const layout = "2006-01-02"
	if !got.IsFinite() || got.Time.Format(layout) != want.Time.Format(layout) {
		t.Errorf("%s = %v, want %v", label, got.Time.Format(layout), want.Time.Format(layout))
	}
}

// Catches: AddDays rendering SQL a database rejects (PostgreSQL typing the
// parameter as an interval, MySQL or SQLite choking on the text date), a wrong
// sign for a negative count, and a month end or leap day that rolls wrongly.
func TestAddDaysIntegration(t *testing.T) {
	eachDateDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedDays(t, db, d)
		for _, c := range []struct {
			id   int64
			n    int
			want types.Date
		}{
			{1, 1, tmDate(2028, 2, 29)},
			{1, 3, tmDate(2028, 3, 2)},
			{1, -3, tmDate(2028, 2, 25)},
			{1, 0, tmDate(2028, 2, 28)},
			{2, 1, tmDate(2028, 2, 1)},
			{2, 3, tmDate(2028, 2, 3)},
			{2, -3, tmDate(2028, 1, 28)},
			{3, 1, tmDate(2029, 1, 1)},
			{3, -366, tmDate(2027, 12, 31)},
		} {
			got, err := dateAt(db, c.id, timeop.AddDays(tdD, c.n))
			wantDate(t, "AddDays", got, err, c.want)
		}
		// A bound date operand, with no column.
		got, err := dateAt(db, 1, timeop.AddDays(momo.Value(tmDate(2028, 2, 28)), 1))
		wantDate(t, "AddDays over a bound date", got, err, tmDate(2028, 2, 29))
	})
}

// Catches: AddDaysExpr with a bound int32 that PostgreSQL cannot type, a
// SQLite modifier built from a negative expression, and a column count.
func TestAddDaysExprIntegration(t *testing.T) {
	eachDateDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedDays(t, db, d)
		for _, c := range []struct {
			n    int32
			want types.Date
		}{
			{1, tmDate(2028, 2, 29)}, {-3, tmDate(2028, 2, 25)}, {0, tmDate(2028, 2, 28)}, {3, tmDate(2028, 3, 2)},
		} {
			got, err := dateAt(db, 1, timeop.AddDaysExpr(tdD, momo.Value(c.n)))
			wantDate(t, "AddDaysExpr", got, err, c.want)
		}
		// A count read from a column: row 1's id is 1 day.
		got, err := dateAt(db, 1, timeop.AddDaysExpr(tdD, tdN))
		wantDate(t, "AddDaysExpr over a column", got, err, tmDate(2028, 2, 29))
	})
}

// Catches: DaysBetween with the operands swapped (wrong sign), a SQLite or
// MySQL result that is not a whole number of days, and a bound date PostgreSQL
// cannot type.
func TestDaysBetweenIntegration(t *testing.T) {
	eachDateDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedDays(t, db, d)
		for _, c := range []struct {
			label string
			id    int64
			e     momo.Expr
			want  int32
		}{
			{"row minus earlier", 1, timeop.DaysBetween(tdD, momo.Value(tmDate(2028, 2, 25))), 3},
			{"earlier minus row", 1, timeop.DaysBetween(momo.Value(tmDate(2028, 2, 25)), tdD), -3},
			{"same day", 1, timeop.DaysBetween(tdD, momo.Value(tmDate(2028, 2, 28))), 0},
			{"across the leap day", 3, timeop.DaysBetween(tdD, momo.Value(tmDate(2028, 2, 28))), 307},
			{"across the leap day, negative", 1, timeop.DaysBetween(momo.Value(tmDate(2028, 2, 28)), momo.Value(tmDate(2028, 12, 31))), -307},
			{"two bound dates", 1, timeop.DaysBetween(momo.Value(tmDate(2028, 3, 1)), momo.Value(tmDate(2028, 2, 28))), 2},
		} {
			got, err := daysAt(db, c.id, c.e)
			if err != nil || got != c.want {
				t.Errorf("%s = %d, %v; want %d", c.label, got, err, c.want)
			}
		}
	})
}

// Catches: a date comparison against date arithmetic that needs momo.Typed or
// compares text with a date wrongly.
func TestAddDaysWhereIntegration(t *testing.T) {
	eachDateDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedDays(t, db, d)
		today := momo.Value(tmDate(2028, 2, 26))
		// d <= today + 3 (2028-02-29): rows 1 and 2.
		r, err := momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "timeop_days"}).
			Columns(momo.CountAll()).Where(tdD.LeExpr(timeop.AddDays(today, 3))).One(context.Background())
		if err != nil || r.N != 2 {
			t.Errorf("d <= today + 3 = %d, %v; want 2", r.N, err)
		}
		// d - today <= 3 on the same rows.
		r, err = momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "timeop_days"}).
			Columns(momo.CountAll()).Where(timeop.DaysBetween(tdD, today).Le(3)).One(context.Background())
		if err != nil || r.N != 2 {
			t.Errorf("d - today <= 3 = %d, %v; want 2", r.N, err)
		}
	})
}

// Catches: infinity reaching date arithmetic unchecked. PostgreSQL keeps
// infinity + n as infinity and refuses infinity - date; SQLite and MySQL
// cannot store infinity and refuse a bound one at build time.
func TestDateArithmeticInfinityIntegration(t *testing.T) {
	eachDateDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedDays(t, db, d)
		inf := momo.Value(types.Date{Infinity: true})
		if d != dialect.Postgres {
			if _, err := dateAt(db, 1, timeop.AddDays(inf, 1)); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("AddDays over infinity = %v, want ErrValidation", err)
			}
			return
		}
		got, err := dateAt(db, 1, timeop.AddDays(inf, 1))
		if err != nil || !got.Infinity {
			t.Errorf("infinity + 1 = %+v, %v; want infinity", got, err)
		}
		if _, err := daysAt(db, 1, timeop.DaysBetween(inf, tdD)); err == nil {
			t.Error("infinity - date succeeded, want a database error")
		}
	})
}
