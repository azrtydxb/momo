// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// mwWin is a title with three nullable window results.
type mwWin struct {
	Title   string
	A, B, C *int64
}

var mwWinMeta = anMeta([]string{"title", "a", "b", "c"}, func(m *mwWin) []any {
	return []any{&m.Title, &m.A, &m.B, &m.C}
})

func (*mwWin) MomoMeta() *momo.Meta[mwWin] { return mwWinMeta }

// mwInt is a nullable int64 for comparisons in table-driven checks.
func mwInt(v int64) *int64 { return &v }

func mwEq(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Catches: window functions that render for MySQL and MariaDB but compute the
// wrong frame or value on the server: LEAD, FIRST_VALUE and LAST_VALUE over a
// full frame, a ROWS moving sum, a RANGE sum that must include peer rows and a
// COUNT over a partition, plus the build errors the docs promise (GROUPS is
// ErrUnsupported on MySQL, a ranking function without ORDER BY and a frame on
// a ranking function are ErrValidation).
func TestWindowFunctionsOnMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		seedAnalytics(t, db)
		ctx := context.Background()
		asc := momo.Over().PartitionBy(suite.DocTenantID).OrderBy(suite.DocVersion.Asc(), suite.DocID.Asc())
		full := asc.Rows(momo.UnboundedPreceding(), momo.UnboundedFollowing())

		rows, err := momo.Select[mwWin](db).From(anDocs).
			Columns(
				suite.DocTitle,
				momo.Lead(suite.DocVersion, 1).Over(asc),
				momo.FirstValue(suite.DocVersion).Over(full),
				momo.LastValue(suite.DocVersion).Over(full),
			).
			Where(suite.DocTenantID.Eq(1)).OrderBy(suite.DocTitle.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		wants := []mwWin{
			{"a", mwInt(20), mwInt(10), mwInt(30)},
			{"b", mwInt(30), mwInt(10), mwInt(30)},
			{"c", nil, mwInt(10), mwInt(30)},
		}
		if len(rows) != len(wants) {
			t.Fatalf("rows = %+v", rows)
		}
		for i, w := range wants {
			r := rows[i]
			if r.Title != w.Title || !mwEq(r.A, w.A) || !mwEq(r.B, w.B) || !mwEq(r.C, w.C) {
				t.Errorf("lead/first/last row %d = %s %v %v %v, want %s %v %v %v", i, r.Title, mwStr(r.A), mwStr(r.B), mwStr(r.C), w.Title, mwStr(w.A), mwStr(w.B), mwStr(w.C))
			}
		}

		// A ROWS moving sum, a RANGE running sum (peers share one value) and a
		// COUNT over the partition, for every tenant at once.
		moving := momo.Sum(suite.DocVersion).Over(asc.Rows(momo.Preceding(1), momo.CurrentRow()))
		byVersion := momo.Over().PartitionBy(suite.DocTenantID).OrderBy(suite.DocVersion.Asc())
		running := momo.Sum(suite.DocVersion).Over(byVersion.Range(momo.UnboundedPreceding(), momo.CurrentRow()))
		size := momo.CountAll().Over(momo.Over().PartitionBy(suite.DocTenantID))
		sums, err := momo.Select[mwWin](db).From(anDocs).
			Columns(suite.DocTitle, moving, running, size).
			OrderBy(suite.DocTitle.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		sumWants := []mwWin{
			{"a", mwInt(10), mwInt(10), mwInt(3)},
			{"b", mwInt(30), mwInt(30), mwInt(3)},
			{"c", mwInt(50), mwInt(60), mwInt(3)},
			{"d", mwInt(5), mwInt(5), mwInt(1)},
			{"e", mwInt(7), mwInt(14), mwInt(3)},
			{"f", mwInt(14), mwInt(14), mwInt(3)},
			{"g", mwInt(16), mwInt(23), mwInt(3)},
		}
		if len(sums) != len(sumWants) {
			t.Fatalf("sums = %+v", sums)
		}
		for i, w := range sumWants {
			r := sums[i]
			if r.Title != w.Title || !mwEq(r.A, w.A) || !mwEq(r.B, w.B) || !mwEq(r.C, w.C) {
				t.Errorf("sum row %d = %s moving %v range %v count %v, want %s %v %v %v", i, r.Title, mwStr(r.A), mwStr(r.B), mwStr(r.C), w.Title, mwStr(w.A), mwStr(w.B), mwStr(w.C))
			}
		}

		// Build errors.
		cols := func(e ...momo.Expr) error {
			_, err := momo.Select[mwWin](db).From(anDocs).Columns(append([]momo.Expr{suite.DocTitle}, e...)...).All(ctx)
			return err
		}
		pad := func(e momo.Expr) []momo.Expr { return []momo.Expr{e, e, e} }
		groups := momo.Sum(suite.DocVersion).Over(byVersion.Groups(momo.UnboundedPreceding(), momo.CurrentRow()))
		if err := cols(pad(groups)...); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("GROUPS frame = %v, want ErrUnsupported", err)
		}
		if err := cols(pad(momo.Rank().Over(momo.Over()))...); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("RANK without ORDER BY = %v, want ErrValidation", err)
		}
		framed := momo.RowNumber().Over(asc.Rows(momo.UnboundedPreceding(), momo.CurrentRow()))
		if err := cols(pad(framed)...); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("frame on ROW_NUMBER = %v, want ErrValidation", err)
		}
	})
}

func mwStr(v *int64) string {
	if v == nil {
		return "NULL"
	}
	return fmt.Sprint(*v)
}

// Catches: the aggregate FILTER clause on MySQL and MariaDB, which have no
// FILTER and get agg(CASE WHEN p THEN x END), counting or summing the wrong
// rows, ignoring the filter in HAVING or in a window, or turning COUNT(*) into
// a count of every row.
func TestAggregateFilterOnMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		seedAnalytics(t, db)
		ctx := context.Background()
		over10 := momo.CountAll().Filter(suite.DocVersion.Gt(10))
		rows, err := momo.Select[anCounts](db).From(anDocs).
			Columns(suite.DocTenantID, momo.CountAll(), over10).
			GroupBy(suite.DocTenantID).OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []anCounts{{1, 3, 2}, {2, 1, 0}, {3, 3, 0}}
		if !slices.Equal(rows, want) {
			t.Fatalf("rows = %+v, want %+v", rows, want)
		}

		kept, err := momo.Select[anCounts](db).From(anDocs).
			Columns(suite.DocTenantID, momo.CountAll(), over10).
			GroupBy(suite.DocTenantID).Having(over10.Gt(0)).All(ctx)
		if err != nil || len(kept) != 1 || kept[0].K != 1 {
			t.Fatalf("having = %+v, %v", kept, err)
		}

		// A filtered aggregate that skips NULLs, as a window function: tenant 1
		// sums its versions above 10 (20 + 30).
		sum := momo.Sum(suite.DocVersion).Filter(suite.DocVersion.Gt(10)).Over(momo.Over().PartitionBy(suite.DocTenantID))
		w, err := momo.Select[anWin](db).From(anDocs).
			Columns(suite.DocTitle, momo.RowNumber().Over(momo.Over().OrderBy(suite.DocID.Asc())), momo.Lag(suite.DocVersion, 1).Over(momo.Over().OrderBy(suite.DocID.Asc())), sum).
			Where(suite.DocTenantID.Eq(1)).OrderBy(suite.DocTitle.Asc()).All(ctx)
		if err != nil || len(w) != 3 || w[0].Running == nil || *w[0].Running != 50 {
			t.Fatalf("filtered window = %+v, %v", w, err)
		}
	})
}

// Catches: LATERAL joins that render but run wrong on MySQL 8, or that render
// for MariaDB, which has no LATERAL, instead of ErrUnsupported.
func TestLateralOnMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		seedAnalytics(t, db)
		ctx := context.Background()
		for _, s := range []string{
			"DROP TABLE IF EXISTS mw_tenants",
			"CREATE TABLE mw_tenants (id bigint PRIMARY KEY)",
			"INSERT INTO mw_tenants (id) VALUES (1), (2), (4)",
		} {
			if _, err := momo.Exec(ctx, db, s); err != nil {
				t.Fatal(err)
			}
		}
		tid := momo.ColumnOf[int64]("t", "id")
		latest := momo.Select[anKT](db).
			From(momo.TableRef{Name: "documents", Alias: "d"}).
			Columns(momo.ColumnOf[int64]("d", "id"), momo.ColumnOf[string]("d", "title")).
			Where(momo.ColumnOf[int64]("d", "tenant_id").EqExpr(tid)).
			OrderBy(momo.ColumnOf[int64]("d", "version").Desc()).
			Limit(1)
		title := momo.ColumnOf[string]("lo", "title")
		src := momo.TableRef{Name: "mw_tenants", Alias: "t"}

		inner, err := momo.Select[anKT](db).From(src).Columns(tid, title).
			CrossJoin(momo.Lateral(latest, "lo")).OrderBy(tid.Asc()).All(ctx)
		if db.Dialect().Flavor == dialect.FlavorMariaDB {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("LATERAL on MariaDB = %v, want ErrUnsupported", err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(inner) != 2 || inner[0] != (anKT{1, "c"}) || inner[1] != (anKT{2, "d"}) {
			t.Fatalf("cross join lateral = %+v", inner)
		}
		outer, err := momo.Select[anKNT](db).From(src).Columns(tid, title).
			LeftJoin(momo.Lateral(latest, "lo"), momo.SQL("true")).OrderBy(tid.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(outer) != 3 || outer[2].K != 4 || outer[2].T != nil || outer[0].T == nil || *outer[0].T != "c" {
			t.Fatalf("left join lateral = %+v", outer)
		}
	})
}

// mwSeedKeyset creates documents with ties on every ordering key: the titles
// differ only by case in places, which the MySQL default collations treat as
// equal, so the primary-key tie-breaker has to split them.
func mwSeedKeyset(t *testing.T, db *momo.DB) {
	t.Helper()
	suite.CreateDocuments(t, db, suite.CapsOf(db))
	for i, title := range []string{"a", "B", "b", "c", "C", "c", "d", "A", "b", "e"} {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title, version) VALUES (?, ?, ?)", int64(i%3), title, int64(i%4)); err != nil {
			t.Fatal(err)
		}
	}
}

// mwWalk pages through q with After, size rows at a time, and returns the ids
// in the order the pages came; cursor names the After values of a page's last
// row.
func mwWalk(t *testing.T, q momo.SelectQuery[suite.Document], size int, cursor func(suite.Document) []any) []int64 {
	t.Helper()
	ctx := context.Background()
	var out []int64
	page := q.Limit(int64(size))
	for i := 0; ; i++ {
		if i > 20 {
			t.Fatal("keyset walk did not terminate")
		}
		rows, err := page.All(ctx)
		if err != nil {
			t.Fatalf("page %d: %v", i+1, err)
		}
		if len(rows) == 0 {
			return out
		}
		if len(rows) > size {
			t.Fatalf("page %d has %d rows, want at most %d", i+1, len(rows), size)
		}
		for _, r := range rows {
			out = append(out, r.ID)
		}
		page = q.Limit(int64(size)).After(cursor(rows[len(rows)-1])...)
	}
}

// Catches: keyset pagination that skips or repeats rows on a MySQL-family
// server: a string key whose collation ties values that differ by case, an
// all-descending key list, a mixed one, and a cursor of the wrong length.
func TestKeysetPaginationOnMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		mwSeedKeyset(t, db)
		ctx := context.Background()
		cases := []struct {
			name   string
			q      momo.SelectQuery[suite.Document]
			cursor func(suite.Document) []any
		}{
			{"title asc", momo.Select[suite.Document](db).OrderBy(suite.DocTitle.Asc(), suite.DocID.Asc()),
				func(d suite.Document) []any { return []any{d.Title, d.ID} }},
			{"title desc", momo.Select[suite.Document](db).OrderBy(suite.DocTitle.Desc(), suite.DocID.Asc()),
				func(d suite.Document) []any { return []any{d.Title, d.ID} }},
			{"version and id desc", momo.Select[suite.Document](db).OrderBy(suite.DocVersion.Desc(), suite.DocID.Desc()),
				func(d suite.Document) []any { return []any{d.Version, d.ID} }},
			{"mixed", momo.Select[suite.Document](db).OrderBy(suite.DocVersion.Asc(), suite.DocTitle.Desc(), suite.DocID.Asc()),
				func(d suite.Document) []any { return []any{d.Version, d.Title, d.ID} }},
		}
		for _, c := range cases {
			all, err := c.q.All(ctx)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			want := make([]int64, len(all))
			for i, d := range all {
				want[i] = d.ID
			}
			if len(want) != 10 {
				t.Fatalf("%s: %d rows, want 10", c.name, len(want))
			}
			for _, size := range []int{1, 3, 4} {
				if got := mwWalk(t, c.q, size, c.cursor); !slices.Equal(got, want) {
					t.Errorf("%s, pages of %d: walk = %v, want %v", c.name, size, got, want)
				}
			}
		}

		_, err := momo.Select[suite.Document](db).OrderBy(suite.DocTitle.Asc()).After("a").All(ctx)
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("cursor with one value for two keys = %v, want ErrValidation", err)
		}
	})
}

// mwSeedBulk creates documents and fills it with n rows, 400 per statement.
func mwSeedBulk(t *testing.T, db *momo.DB, n int) {
	t.Helper()
	suite.CreateDocuments(t, db, suite.CapsOf(db))
	for lo := 0; lo < n; lo += 400 {
		hi := min(lo+400, n)
		var sb strings.Builder
		var args []any
		sb.WriteString("INSERT INTO documents (tenant_id, title, version) VALUES ")
		for i := lo; i < hi; i++ {
			if i > lo {
				sb.WriteString(", ")
			}
			sb.WriteString("(?, ?, ?)")
			args = append(args, int64(i%5), fmt.Sprintf("doc-%04d", i), int64(1))
		}
		if _, err := momo.Exec(context.Background(), db, sb.String(), args...); err != nil {
			t.Fatal(err)
		}
	}
}

// Catches: Each and Iter on a MySQL-family server that buffer or lose rows,
// leak the connection when the loop ends early (the pool holds three, so a
// leak per early stop hangs the sixth query), stop reading the cursor on an
// error or a canceled context without reporting it, or cannot run a second
// query while the cursor is open.
func TestStreamingOnMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		const rows = 1200
		mwSeedBulk(t, db, rows)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		q := momo.Select[suite.Document](db).OrderBy(suite.DocID.Asc())

		var ids []int64
		err := q.Each(ctx, func(d *suite.Document) error {
			ids = append(ids, d.ID)
			return nil
		})
		if err != nil || len(ids) != rows || !slices.IsSorted(ids) {
			t.Fatalf("Each: %d rows, sorted %v, err %v", len(ids), slices.IsSorted(ids), err)
		}
		var iterIDs []int64
		for d, err := range q.Iter(ctx) {
			if err != nil {
				t.Fatal(err)
			}
			iterIDs = append(iterIDs, d.ID)
		}
		if !slices.Equal(iterIDs, ids) {
			t.Fatalf("Iter returned %d rows that differ from Each (%d rows)", len(iterIDs), len(ids))
		}

		// A filtered, limited stream.
		n := 0
		err = q.Where(suite.DocTenantID.Eq(2)).Limit(7).Each(ctx, func(d *suite.Document) error {
			if d.TenantID != 2 {
				t.Errorf("row %d has tenant %d, want 2", d.ID, d.TenantID)
			}
			n++
			return nil
		})
		if err != nil || n != 7 {
			t.Fatalf("Each with Where and Limit: %d rows, %v", n, err)
		}

		// Stopping early, five times, must hand the connection back each time.
		stop := errors.New("stop")
		for i := 0; i < 5; i++ {
			seen := 0
			err := q.Each(ctx, func(*suite.Document) error {
				seen++
				if seen == 3 {
					return stop
				}
				return nil
			})
			if !errors.Is(err, stop) || seen != 3 {
				t.Fatalf("Each stopped by an error: seen %d, err %v", seen, err)
			}
			seen = 0
			for _, err := range q.Iter(ctx) {
				if err != nil {
					t.Fatal(err)
				}
				seen++
				if seen == 3 {
					break
				}
			}
			if seen != 3 {
				t.Fatalf("Iter break: seen %d", seen)
			}
		}
		if c, err := q.Count(ctx); err != nil || c != rows {
			t.Fatalf("Count after early stops = %d, %v, want %d", c, err, rows)
		}

		// A second query runs on another connection while the cursor is open.
		inner := int64(-1)
		seen := 0
		err = q.Each(ctx, func(*suite.Document) error {
			seen++
			if seen == 10 {
				c, err := q.Where(suite.DocTenantID.Eq(0)).Count(ctx)
				inner = c
				return err
			}
			return nil
		})
		if err != nil || inner != rows/5 {
			t.Fatalf("query during Each: inner = %d, err %v, want %d", inner, err, rows/5)
		}

		// A canceled context ends the stream with ErrCanceled.
		cctx, ccancel := context.WithCancel(ctx)
		seen = 0
		err = q.Each(cctx, func(*suite.Document) error {
			seen++
			if seen == 5 {
				ccancel()
			}
			return nil
		})
		if !errors.Is(err, momo.ErrCanceled) {
			t.Fatalf("Each with a canceled context = %v after %d rows, want ErrCanceled", err, seen)
		}

		// A stream inside a transaction.
		err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			c := 0
			for _, err := range momo.Select[suite.Document](tx).Iter(ctx) {
				if err != nil {
					return err
				}
				c++
			}
			if c != rows {
				return fmt.Errorf("Iter in a transaction saw %d rows, want %d", c, rows)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
