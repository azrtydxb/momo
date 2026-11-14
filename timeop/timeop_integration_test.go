// SPDX-License-Identifier: Apache-2.0

//go:build integration

package timeop_test

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/timeop"
)

type tmFloat struct{ F float64 }

var tmFloatMeta = momo.Meta[tmFloat]{
	Columns:  []momo.ColumnMeta{{Name: "f"}},
	Ptr:      func(m *tmFloat, _ int) any { return &m.F },
	Value:    func(m *tmFloat, _ int) any { return m.F },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*tmFloat) MomoMeta() *momo.Meta[tmFloat] { return &tmFloatMeta }

type tmText struct{ S string }

var tmTextMeta = momo.Meta[tmText]{
	Columns:  []momo.ColumnMeta{{Name: "s"}},
	Ptr:      func(m *tmText, _ int) any { return &m.S },
	Value:    func(m *tmText, _ int) any { return m.S },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*tmText) MomoMeta() *momo.Meta[tmText] { return &tmTextMeta }

// sqliteStamp is momo's stored SQLite timestamp text: millisecond resolution.
var sqliteStamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}$`)

type tmTime struct{ T time.Time }

var tmTimeMeta = momo.Meta[tmTime]{
	Columns:  []momo.ColumnMeta{{Name: "t"}},
	Ptr:      func(m *tmTime, _ int) any { return &m.T },
	Value:    func(m *tmTime, _ int) any { return m.T },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*tmTime) MomoMeta() *momo.Meta[tmTime] { return &tmTimeMeta }

// seedTimeDocs inserts one document at a fixed instant (tenant 1) and one at
// the database's now (tenant 2).
func seedTimeDocs(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	suite.CreateDocuments(t, db, suite.CapsOf(db))
	// SQLite stores momo's own text format, which strftime parses.
	old := "'2024-03-15 13:45:30+00'"
	// The "new" row takes the database's now. SQLite timestamps have millisecond
	// resolution, so it is seeded a second in the past: "created before now" then
	// cannot tie with a Now() read in the same millisecond.
	newCol, newVal := "", ""
	if db.Dialect().Name == dialect.SQLite {
		newCol, newVal = ", created_at", ", strftime('%Y-%m-%d %H:%M:%f', 'now', '-1 seconds')"
	}
	if db.Dialect().Name == dialect.SQLite {
		old = "'2024-03-15 13:45:30.000'"
	}
	if db.Dialect().Name == dialect.MySQL {
		old = "'2024-03-15 13:45:30'" // MariaDB has no UTC offset in a datetime literal
	}
	for _, s := range []string{
		"INSERT INTO documents (tenant_id, title, created_at) VALUES (1, 'old', " + old + ")",
		"INSERT INTO documents (tenant_id, title" + newCol + ") VALUES (2, 'new'" + newVal + ")",
	} {
		if _, err := momo.Exec(ctx, db, s); err != nil {
			t.Fatal(err)
		}
	}
}

func timeOf(t *testing.T, db *momo.DB, tenant int64, e momo.Expr) time.Time {
	t.Helper()
	r, err := momo.Select[tmTime](db).From(momo.TableRef{Name: "documents"}).Columns(e).
		Where(suite.DocTenantID.Eq(tenant)).One(context.Background())
	if err != nil {
		t.Fatalf("select time: %v", err)
	}
	return r.T
}

func floatOf(t *testing.T, db *momo.DB, tenant int64, e momo.Expr) float64 {
	t.Helper()
	r, err := momo.Select[tmFloat](db).From(momo.TableRef{Name: "documents"}).Columns(e).
		Where(suite.DocTenantID.Eq(tenant)).One(context.Background())
	if err != nil {
		t.Fatalf("select float: %v", err)
	}
	return r.F
}

// Catches: Extract rendering SQL PostgreSQL rejects, numeric results the
// driver cannot scan into float64, and a wrong field or EPOCH value.
func TestExtractIntegration(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		at := time.Date(2024, 3, 15, 13, 45, 30, 0, time.UTC)
		// Read the wall clock in UTC explicitly, so the result does not
		// depend on the session time zone.
		src := momo.Expression[time.Time](suite.DocCreatedAt)
		if d == dialect.Postgres {
			src = timeop.AtTimeZone(suite.DocCreatedAt, "UTC")
		}
		for _, c := range []struct {
			field timeop.ExtractField
			want  float64
		}{
			{timeop.ExtractYear, 2024}, {timeop.ExtractMonth, 3}, {timeop.ExtractDay, 15},
			{timeop.ExtractHour, 13}, {timeop.ExtractMinute, 45}, {timeop.ExtractSecond, 30},
			{timeop.ExtractQuarter, 1}, {timeop.ExtractDow, 5}, {timeop.ExtractISODow, 5},
			{timeop.ExtractDoy, 75}, {timeop.ExtractEpoch, float64(at.Unix())},
		} {
			if got := floatOf(t, db, 1, timeop.Extract(c.field, src)); math.Abs(got-c.want) > 1e-6 {
				t.Errorf("Extract %s = %v, want %v", c.field, got, c.want)
			}
		}
		if d != dialect.Postgres {
			_, err := momo.Select[tmFloat](db).From(momo.TableRef{Name: "documents"}).
				Columns(timeop.Extract(timeop.ExtractHour, timeop.AtTimeZone(suite.DocCreatedAt, "Asia/Tokyo"))).
				Where(suite.DocTenantID.Eq(1)).One(context.Background())
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("AtTimeZone on %s = %v, want ErrUnsupported", d, err)
			}
		}
		if d == dialect.Postgres {
			if got := floatOf(t, db, 1, timeop.Extract(timeop.ExtractHour, src)); got != 13 {
				t.Errorf("hour in UTC = %v, want 13", got)
			}
			if got := floatOf(t, db, 1, timeop.Extract(timeop.ExtractHour, timeop.AtTimeZone(suite.DocCreatedAt, "Asia/Tokyo"))); got != 22 {
				t.Errorf("hour in Tokyo = %v, want 22", got)
			}
		}
	})
}

// Catches: a bound Go time that PostgreSQL cannot type inside EXTRACT,
// date_trunc or AT TIME ZONE.
func TestBoundTimeOperandIntegration(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		at := time.Date(2024, 3, 15, 13, 45, 30, 0, time.UTC)
		if got := floatOf(t, db, 1, timeop.Extract(timeop.ExtractYear, momo.Value(at))); got != 2024 {
			t.Errorf("Extract over a bound time = %v, want 2024", got)
		}
		trunc := momo.Expression[time.Time](momo.Value(at))
		if d == dialect.Postgres {
			trunc = timeop.AtTimeZone(momo.Value(at), "UTC")
		}
		if got := timeOf(t, db, 1, timeop.Trunc(timeop.TruncDay, trunc)); got.Day() != 15 || got.Hour() != 0 {
			t.Errorf("DateTrunc over a bound time = %v", got)
		}
		if got := timeOf(t, db, 1, timeop.AddInterval(momo.Value(at), timeop.IntervalOf(1, timeop.IntervalHour))); !got.Equal(at.Add(time.Hour)) {
			t.Errorf("AddInterval over a bound time = %v", got)
		}
	})
}

// Catches: a zone-converted day bucket whose SELECT and GROUP BY texts differ,
// which PostgreSQL rejects.
func TestGroupByLocalDayIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("AtTimeZone is PostgreSQL only")
		}
		seedTimeDocs(t, db)
		// 23:30 UTC on March 15 is already March 16 in Brussels (UTC+1).
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title, created_at) VALUES (3, 'late', '2024-03-15 23:30:00+00')"); err != nil {
			t.Fatal(err)
		}
		local := func() timeop.Expr {
			return timeop.Trunc(timeop.TruncDay, timeop.AtTimeZone(suite.DocCreatedAt, "Europe/Brussels"))
		}
		got, err := momo.Select[tmTime](db).From(momo.TableRef{Name: "documents"}).
			Columns(local()).Where(suite.DocTenantID.Eq(3)).GroupBy(local()).One(context.Background())
		if err != nil || got.T.Day() != 16 {
			t.Fatalf("local day of 23:30 UTC = %+v, %v; want the 16th", got, err)
		}
	})
}

// Catches: DateTrunc units mapped wrongly (a week not starting on Monday, a
// quarter not on its first month) and the grouped expression differing from
// the select-list one.
func TestDateTruncIntegration(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		src := momo.Expression[time.Time](suite.DocCreatedAt)
		if d == dialect.Postgres {
			src = timeop.AtTimeZone(suite.DocCreatedAt, "UTC")
		}
		for _, c := range []struct {
			unit timeop.TruncUnit
			want time.Time
		}{
			{timeop.TruncSecond, time.Date(2024, 3, 15, 13, 45, 30, 0, time.UTC)},
			{timeop.TruncMinute, time.Date(2024, 3, 15, 13, 45, 0, 0, time.UTC)},
			{timeop.TruncHour, time.Date(2024, 3, 15, 13, 0, 0, 0, time.UTC)},
			{timeop.TruncDay, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
			{timeop.TruncWeek, time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)},
			{timeop.TruncMonth, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
			{timeop.TruncQuarter, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			{timeop.TruncYear, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		} {
			got := timeOf(t, db, 1, timeop.Trunc(c.unit, src))
			if got.Year() != c.want.Year() || got.Month() != c.want.Month() || got.Day() != c.want.Day() || got.Hour() != c.want.Hour() || got.Minute() != c.want.Minute() || got.Second() != c.want.Second() {
				t.Errorf("DateTrunc %s = %v, want %v", c.unit, got, c.want)
			}
		}
		day := func() timeop.Expr { return timeop.Trunc(timeop.TruncDay, suite.DocCreatedAt) }
		rows, err := momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "documents"}).
			Columns(momo.CountAll()).GroupBy(day()).OrderBy(day().Asc()).All(context.Background())
		if err != nil || len(rows) != 2 {
			t.Fatalf("group by day = %+v, %v; want 2 groups", rows, err)
		}
	})
}

// Catches: interval arithmetic with the wrong sign or unit, a bound count
// that PostgreSQL cannot type, and Now/CurrentTimestamp not usable in a
// comparison with a stored timestamp.
func TestIntervalAndNowIntegration(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		ctx := context.Background()
		at := time.Date(2024, 3, 15, 13, 45, 30, 0, time.UTC)

		if got := timeOf(t, db, 1, timeop.AddInterval(suite.DocCreatedAt, timeop.IntervalOf(36, timeop.IntervalHour))); !got.Equal(at.Add(36 * time.Hour)) {
			t.Errorf("+36 hours = %v", got)
		}
		if got := timeOf(t, db, 1, timeop.SubInterval(suite.DocCreatedAt, timeop.IntervalOf(2, timeop.IntervalWeek))); !got.Equal(at.Add(-14 * 24 * time.Hour)) {
			t.Errorf("-2 weeks = %v", got)
		}
		bound := timeop.IntervalExpr(momo.Value(int64(90)), timeop.IntervalMinute)
		if got := timeOf(t, db, 1, timeop.AddInterval(suite.DocCreatedAt, bound)); !got.Equal(at.Add(90 * time.Minute)) {
			t.Errorf("+90 bound minutes = %v", got)
		}

		// Month arithmetic at a month end: Feb 28 plus a month is Mar 28
		// everywhere; Jan 31 plus a month is clamped to Feb 28 on PostgreSQL
		// and normalised into Mar 3 on SQLite (2023 is not a leap year).
		stamp := func(tenant int64, wall string) {
			t.Helper()
			lit := "'" + wall + "+00'"
			if d == dialect.SQLite {
				lit = "'" + wall + ".000'"
			}
			if d == dialect.MySQL {
				lit = "'" + wall + "'"
			}
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title, created_at) VALUES ("+strconv.FormatInt(tenant, 10)+", 'eom', "+lit+")"); err != nil {
				t.Fatal(err)
			}
		}
		stamp(10, "2023-02-28 10:00:00")
		stamp(11, "2023-01-31 10:00:00")
		month := timeop.IntervalOf(1, timeop.IntervalMonth)
		if got, want := timeOf(t, db, 10, timeop.AddInterval(suite.DocCreatedAt, month)), time.Date(2023, 3, 28, 10, 0, 0, 0, time.UTC); !got.Equal(want) {
			t.Errorf("Feb 28 + 1 month = %v, want %v", got, want)
		}
		janWant := time.Date(2023, 2, 28, 10, 0, 0, 0, time.UTC)
		if d == dialect.SQLite {
			janWant = time.Date(2023, 3, 3, 10, 0, 0, 0, time.UTC)
		}
		if got := timeOf(t, db, 11, timeop.AddInterval(suite.DocCreatedAt, month)); !got.Equal(janWant) {
			t.Errorf("Jan 31 + 1 month = %v, want %v", got, janWant)
		}
		if got, want := timeOf(t, db, 11, timeop.SubInterval(suite.DocCreatedAt, timeop.IntervalOf(1, timeop.IntervalYear))), time.Date(2022, 1, 31, 10, 0, 0, 0, time.UTC); !got.Equal(want) {
			t.Errorf("Jan 31 - 1 year = %v, want %v", got, want)
		}

		if d == dialect.SQLite {
			// Now is text in the stored timestamp format, so it sorts with it.
			r, err := momo.Select[tmText](db).From(momo.TableRef{Name: "documents"}).Columns(timeop.Now()).
				Where(suite.DocTenantID.Eq(1)).One(ctx)
			if err != nil || !sqliteStamp.MatchString(r.S) {
				t.Errorf("Now() on SQLite = %q, %v; want %s", r.S, err, sqliteStamp)
			}
		}

		now := timeOf(t, db, 1, timeop.Now())
		if diff := time.Since(now); diff < -time.Minute || diff > time.Minute {
			t.Errorf("Now() = %v, far from the wall clock", now)
		}
		if cur := timeOf(t, db, 1, timeop.CurrentTimestamp()); cur.Before(now.Add(-time.Minute)) {
			t.Errorf("CurrentTimestamp() = %v, behind Now() = %v", cur, now)
		}

		recent := func(p momo.Predicate) int64 {
			r, err := momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "documents"}).
				Columns(momo.CountAll()).Where(p).One(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return r.N
		}
		if n := recent(suite.DocCreatedAt.GtExpr(timeop.SubInterval(timeop.Now(), timeop.IntervalOf(1, timeop.IntervalDay)))); n != 1 {
			t.Errorf("created in the last day = %d, want 1", n)
		}
		if n := recent(suite.DocCreatedAt.LtExpr(timeop.Now())); n != 4 {
			t.Errorf("created before now = %d, want 4", n)
		}
	})
}
