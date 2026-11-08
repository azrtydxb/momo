// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

type tmFloat struct{ F float64 }

var tmFloatMeta = momo.Meta[tmFloat]{
	Columns:  []momo.ColumnMeta{{Name: "f"}},
	Ptr:      func(m *tmFloat, _ int) any { return &m.F },
	Value:    func(m *tmFloat, _ int) any { return m.F },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*tmFloat) MomoMeta() *momo.Meta[tmFloat] { return &tmFloatMeta }

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
	if db.Dialect().Name == dialect.SQLite {
		// The seeds use a PostgreSQL timestamp literal and the expectations are
		// PostgreSQL's extract and date_trunc semantics; SQLite strftime support
		// for these expressions is not implemented yet.
		t.Skip("time expressions are not implemented for SQLite yet")
	}
	ctx := context.Background()
	suite.CreateDocuments(t, db, suite.CapsOf(db))
	for _, s := range []string{
		"INSERT INTO documents (tenant_id, title, created_at) VALUES (1, 'old', '2024-03-15 13:45:30+00')",
		"INSERT INTO documents (tenant_id, title) VALUES (2, 'new')",
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
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		at := time.Date(2024, 3, 15, 13, 45, 30, 0, time.UTC)
		// Read the wall clock in UTC explicitly, so the result does not
		// depend on the session time zone.
		src := momo.Expression[time.Time](suite.DocCreatedAt)
		if d == dialect.Postgres {
			src = momo.AtTimeZone(suite.DocCreatedAt, "UTC")
		}
		for _, c := range []struct {
			field momo.ExtractField
			want  float64
		}{
			{momo.ExtractYear, 2024}, {momo.ExtractMonth, 3}, {momo.ExtractDay, 15},
			{momo.ExtractQuarter, 1}, {momo.ExtractDow, 5}, {momo.ExtractISODow, 5},
			{momo.ExtractDoy, 75}, {momo.ExtractEpoch, float64(at.Unix())},
		} {
			if got := floatOf(t, db, 1, momo.Extract(c.field, src)); math.Abs(got-c.want) > 1e-6 {
				t.Errorf("Extract %s = %v, want %v", c.field, got, c.want)
			}
		}
		if d == dialect.Postgres {
			if got := floatOf(t, db, 1, momo.Extract(momo.ExtractHour, src)); got != 13 {
				t.Errorf("hour in UTC = %v, want 13", got)
			}
			if got := floatOf(t, db, 1, momo.Extract(momo.ExtractHour, momo.AtTimeZone(suite.DocCreatedAt, "Asia/Tokyo"))); got != 22 {
				t.Errorf("hour in Tokyo = %v, want 22", got)
			}
		}
	})
}

// Catches: a bound Go time that PostgreSQL cannot type inside EXTRACT,
// date_trunc or AT TIME ZONE.
func TestBoundTimeOperandIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("bound timestamp operands are checked on PostgreSQL")
		}
		seedTimeDocs(t, db)
		at := time.Date(2024, 3, 15, 13, 45, 30, 0, time.UTC)
		if got := floatOf(t, db, 1, momo.Extract(momo.ExtractYear, momo.Value(at))); got != 2024 {
			t.Errorf("Extract over a bound time = %v, want 2024", got)
		}
		if got := timeOf(t, db, 1, momo.DateTrunc(momo.TruncDay, momo.AtTimeZone(momo.Value(at), "UTC"))); got.Day() != 15 || got.Hour() != 0 {
			t.Errorf("DateTrunc over a bound time = %v", got)
		}
		if got := timeOf(t, db, 1, momo.AddInterval(momo.Value(at), momo.IntervalOf(1, momo.IntervalHour))); !got.Equal(at.Add(time.Hour)) {
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
		local := func() momo.TimeExpr {
			return momo.DateTrunc(momo.TruncDay, momo.AtTimeZone(suite.DocCreatedAt, "Europe/Brussels"))
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
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		src := momo.Expression[time.Time](suite.DocCreatedAt)
		if d == dialect.Postgres {
			src = momo.AtTimeZone(suite.DocCreatedAt, "UTC")
		}
		for _, c := range []struct {
			unit momo.TruncUnit
			want time.Time
		}{
			{momo.TruncHour, time.Date(2024, 3, 15, 13, 0, 0, 0, time.UTC)},
			{momo.TruncDay, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
			{momo.TruncWeek, time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)},
			{momo.TruncMonth, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
			{momo.TruncQuarter, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			{momo.TruncYear, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		} {
			got := timeOf(t, db, 1, momo.DateTrunc(c.unit, src))
			if got.Year() != c.want.Year() || got.Month() != c.want.Month() || got.Day() != c.want.Day() || got.Hour() != c.want.Hour() || got.Minute() != 0 {
				t.Errorf("DateTrunc %s = %v, want %v", c.unit, got, c.want)
			}
		}
		day := func() momo.TimeExpr { return momo.DateTrunc(momo.TruncDay, suite.DocCreatedAt) }
		rows, err := momo.Select[unitTally](db).From(momo.TableRef{Name: "documents"}).
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
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		ctx := context.Background()
		at := time.Date(2024, 3, 15, 13, 45, 30, 0, time.UTC)

		if got := timeOf(t, db, 1, momo.AddInterval(suite.DocCreatedAt, momo.IntervalOf(36, momo.IntervalHour))); !got.Equal(at.Add(36 * time.Hour)) {
			t.Errorf("+36 hours = %v", got)
		}
		if got := timeOf(t, db, 1, momo.SubInterval(suite.DocCreatedAt, momo.IntervalOf(2, momo.IntervalWeek))); !got.Equal(at.Add(-14 * 24 * time.Hour)) {
			t.Errorf("-2 weeks = %v", got)
		}
		bound := momo.IntervalExpr(momo.Value(int64(90)), momo.IntervalMinute)
		if got := timeOf(t, db, 1, momo.AddInterval(suite.DocCreatedAt, bound)); !got.Equal(at.Add(90 * time.Minute)) {
			t.Errorf("+90 bound minutes = %v", got)
		}

		if d != dialect.Postgres {
			return
		}
		now := timeOf(t, db, 1, momo.Now())
		if diff := time.Since(now); diff < -time.Minute || diff > time.Minute {
			t.Errorf("Now() = %v, far from the wall clock", now)
		}
		if cur := timeOf(t, db, 1, momo.CurrentTimestamp()); cur.Before(now.Add(-time.Minute)) {
			t.Errorf("CurrentTimestamp() = %v, behind Now() = %v", cur, now)
		}

		recent := func(p momo.Predicate) int64 {
			r, err := momo.Select[unitTally](db).From(momo.TableRef{Name: "documents"}).
				Columns(momo.CountAll()).Where(p).One(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return r.N
		}
		if n := recent(suite.DocCreatedAt.GtExpr(momo.SubInterval(momo.Now(), momo.IntervalOf(1, momo.IntervalDay)))); n != 1 {
			t.Errorf("created in the last day = %d, want 1", n)
		}
		if n := recent(suite.DocCreatedAt.LtExpr(momo.Now())); n != 2 {
			t.Errorf("created before now = %d, want 2", n)
		}
	})
}

// Catches: JoinSQL fragments losing their binds or placeholder numbering when
// executed, and a typed join not usable as a value.
func TestJoinSQLIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTimeDocs(t, db)
		ctx := context.Background()
		var conds []momo.Expr
		for _, tenant := range []int64{1, 99} {
			conds = append(conds, momo.SQL("tenant_id = ?", tenant))
		}
		r, err := momo.Select[unitTally](db).From(momo.TableRef{Name: "documents"}).
			Columns(momo.CountAll()).
			Where(suite.DocTitle.Ne("zzz"), momo.JoinSQL(" OR ", conds...)).One(ctx)
		if err != nil || r.N != 1 {
			t.Fatalf("OR join count = %+v, %v; want 1", r, err)
		}
		sum := momo.JoinTypedSQL[int64]("+", momo.SQL("tenant_id"), momo.SQL("tenant_id"))
		r, err = momo.Select[unitTally](db).From(momo.TableRef{Name: "documents"}).
			Columns(sum).Where(suite.DocTenantID.Eq(2)).One(ctx)
		if err != nil || r.N != 4 {
			t.Fatalf("typed + join = %+v, %v; want 4", r, err)
		}
	})
}
