// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// parityEach runs fn once per dialect on a fresh database: PostgreSQL (the
// reference), SQLite, MySQL 8.4, MySQL 9.7 and MariaDB. A dialect whose server
// URL is unset skips, or fails under MOMO_REQUIRE_INTEGRATION=1, as the other
// integration tests do. It names MySQL-family servers apart because testopen.Each
// opens PostgreSQL and SQLite only.
func parityEach(t *testing.T, fn func(t *testing.T, db *momo.DB, name string)) {
	t.Helper()
	for _, o := range []struct {
		name string
		open func(*testing.T, ...momo.Option) *momo.DB
	}{
		{"postgres", testopen.Postgres},
		{"sqlite", testopen.SQLite},
		{"mysql84", testopen.MySQL84},
		{"mysql97", testopen.MySQL97},
		{"mariadb", testopen.MariaDB},
	} {
		t.Run(o.name, func(t *testing.T) {
			db := o.open(t)
			suite.CreateDocuments(t, db, suite.CapsOf(db))
			fn(t, db, o.name)
		})
	}
}

// parityDoc is one seeded documents row; a nil body is SQL NULL.
type parityDoc struct {
	tenant  int64
	title   string
	body    any
	version int64
}

func seedParityDocs(t *testing.T, db *momo.DB, docs ...parityDoc) {
	t.Helper()
	for _, d := range docs {
		if _, err := momo.Exec(context.Background(), db,
			"INSERT INTO documents (tenant_id, title, body, version) VALUES (?, ?, ?, ?)", d.tenant, d.title, d.body, d.version); err != nil {
			t.Fatal(err)
		}
	}
}

// parityTitles returns the titles matching where, sorted in byte order.
func parityTitles(t *testing.T, db *momo.DB, where momo.Predicate) []string {
	t.Helper()
	rows, err := momo.Select[suite.Document](db).Where(where).OrderBy(suite.DocID.Asc()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Title
	}
	// The databases collate differently; compare as a set in byte order.
	slices.Sort(out)
	return out
}

// Catches: ILike on SQLite, MySQL or MariaDB matching case-sensitively, not
// matching at all, losing the ESCAPE clause (a literal % or _ acting as a
// wildcard), or NOT ILIKE not inverting. PostgreSQL is the reference result.
func TestILikeEmulationRunsOnEveryDialect(t *testing.T) {
	parityEach(t, func(t *testing.T, db *momo.DB, name string) {
		seedParityDocs(t, db,
			parityDoc{1, "Hello World", nil, 1}, parityDoc{1, "HELLO there", nil, 1}, parityDoc{1, "hello", nil, 1},
			parityDoc{1, "Goodbye", nil, 1}, parityDoc{1, "50% OFF", nil, 1}, parityDoc{1, "50 percent", nil, 1},
		)
		cases := []struct {
			name string
			pred momo.Predicate
			want []string
		}{
			{"prefix any case", momo.ILike(suite.DocTitle, "hello%"), []string{"HELLO there", "Hello World", "hello"}},
			{"suffix any case", momo.ILike(suite.DocTitle, "%WORLD"), []string{"Hello World"}},
			{"underscore wildcard", momo.ILike(suite.DocTitle, "h_llo"), []string{"hello"}},
			{"escaped percent", momo.ILike(suite.DocTitle, momo.EscapeLike("50% off")), []string{"50% OFF"}},
			{"not ilike", momo.Not(momo.ILike(suite.DocTitle, "%o%")), []string{"50 percent"}},
		}
		for _, c := range cases {
			got := parityTitles(t, db, c.pred)
			if !slices.Equal(got, c.want) {
				t.Errorf("%s: got %q, want %q", c.name, got, c.want)
			}
		}
	})
}

// Catches: the documented non-ASCII difference changing without the docs
// following. PostgreSQL folds "Ä" to "ä" (given a UTF-8 database with a
// locale that does), SQLite folds ASCII only so it does not, and MySQL and
// MariaDB fold through LOWER and the column collation.
func TestILikeNonASCIIPerDialect(t *testing.T) {
	parityEach(t, func(t *testing.T, db *momo.DB, name string) {
		seedParityDocs(t, db, parityDoc{1, "ÄPFEL", nil, 1}, parityDoc{1, "äpfel", nil, 1}, parityDoc{1, "birne", nil, 1})
		want := []string{"ÄPFEL", "äpfel"}
		if name == "sqlite" {
			want = []string{"äpfel"} // LOWER and LIKE fold ASCII only: "Ä" stays "Ä"
		}
		got := parityTitles(t, db, momo.ILike(suite.DocTitle, "äpfel"))
		if !slices.Equal(got, want) {
			t.Errorf("ILike \"äpfel\": got %q, want %q", got, want)
		}
	})
}

// Catches: NULLS FIRST or LAST on MySQL and MariaDB putting NULLs on the wrong
// end, sorting the non-NULL values wrongly, or (the bound-parameter case) the
// duplicated expression sending its argument to the wrong placeholder.
func TestNullsOrderingRunsOnEveryDialect(t *testing.T) {
	parityEach(t, func(t *testing.T, db *momo.DB, name string) {
		seedParityDocs(t, db,
			parityDoc{1, "t1", "b", 1}, parityDoc{1, "t2", nil, 1}, parityDoc{1, "t3", "a", 1},
			parityDoc{1, "t4", nil, 1}, parityDoc{1, "t5", "c", 1},
		)
		byBody := func(o momo.Order) []string {
			rows, err := momo.Select[suite.Document](db).OrderBy(o, suite.DocID.Asc()).All(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			out := make([]string, len(rows))
			for i, r := range rows {
				out[i] = r.Title
			}
			return out
		}
		cases := []struct {
			name string
			o    momo.Order
			want []string
		}{
			{"asc nulls first", suite.DocBody.Asc().NullsFirst(), []string{"t2", "t4", "t3", "t1", "t5"}},
			{"asc nulls last", suite.DocBody.Asc().NullsLast(), []string{"t3", "t1", "t5", "t2", "t4"}},
			{"desc nulls first", suite.DocBody.Desc().NullsFirst(), []string{"t2", "t4", "t5", "t1", "t3"}},
			{"desc nulls last", suite.DocBody.Desc().NullsLast(), []string{"t5", "t1", "t3", "t2", "t4"}},
			// NULLIF(body, ?) is NULL for t1 too, so t1 joins the NULLs; the
			// bound "b" is rendered twice on MySQL and must be bound twice.
			{"expression with a bound argument", momo.Asc(momo.TypedSQL[string]("NULLIF(body, ?)", "b")).NullsLast(), []string{"t3", "t5", "t1", "t2", "t4"}},
		}
		for _, c := range cases {
			if got := byBody(c.o); !slices.Equal(got, c.want) {
				t.Errorf("%s: got %q, want %q", c.name, got, c.want)
			}
		}
	})
}

// Catches: an aggregate FILTER on MySQL and MariaDB returning a different sum,
// count, minimum or maximum than PostgreSQL's FILTER (a count that includes
// the filtered-out rows, a NULL that becomes 0), or a filtered window sum that
// ignores the predicate.
func TestAggregateFilterRunsOnEveryDialect(t *testing.T) {
	parityEach(t, func(t *testing.T, db *momo.DB, name string) {
		seedParityDocs(t, db,
			parityDoc{1, "t1", "b", 10}, parityDoc{1, "t2", nil, 20}, parityDoc{2, "t3", "a", 30},
			parityDoc{2, "t4", nil, 40}, parityDoc{3, "t5", "c", 50},
		)
		ctx := context.Background()
		docs := momo.TableRef{Name: "documents"}
		big := suite.DocVersion.Gt(15)

		rows, err := momo.Select[momo.Row6[int64, int64, int64, string, string, int64]](db).From(docs).Columns(
			momo.Sum(suite.DocVersion).Filter(big),
			momo.CountAll().Filter(big),
			momo.CountOf(suite.DocBody).Filter(big),
			momo.Min(suite.DocTitle).Filter(big),
			momo.Max(suite.DocTitle).Filter(big),
			momo.CountDistinct(suite.DocTenantID).Filter(big),
		).All(ctx)
		if err != nil || len(rows) != 1 {
			t.Fatalf("filtered aggregates: %v, %d rows", err, len(rows))
		}
		if r := rows[0]; r.C1 != 140 || r.C2 != 4 || r.C3 != 2 || r.C4 != "t2" || r.C5 != "t5" || r.C6 != 3 {
			t.Errorf("sum, count(*), count(body), min, max, count(distinct) = %d %d %d %q %q %d, want 140 4 2 t2 t5 3", r.C1, r.C2, r.C3, r.C4, r.C5, r.C6)
		}

		// No row passes the filter: SUM is NULL and COUNT is 0, as with FILTER.
		none := suite.DocVersion.Gt(1000)
		empty, err := momo.Select[momo.Row2[*int64, int64]](db).From(docs).Columns(
			momo.Sum(suite.DocVersion).Filter(none), momo.CountAll().Filter(none)).All(ctx)
		if err != nil || len(empty) != 1 {
			t.Fatalf("empty filter: %v, %d rows", err, len(empty))
		}
		if empty[0].C1 != nil || empty[0].C2 != 0 {
			t.Errorf("empty filter: sum = %v, count = %d, want NULL and 0", empty[0].C1, empty[0].C2)
		}

		// Grouped, with a filter that binds a value.
		grouped, err := momo.Select[momo.Row2[int64, int64]](db).From(docs).
			Columns(suite.DocTenantID, momo.Sum(suite.DocVersion).Filter(suite.DocTitle.Ne("t2"))).
			GroupBy(suite.DocTenantID).OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got [][2]int64
		for _, r := range grouped {
			got = append(got, [2]int64{r.C1, r.C2})
		}
		if want := [][2]int64{{1, 10}, {2, 70}, {3, 50}}; !slices.Equal(got, want) {
			t.Errorf("grouped filtered sum = %v, want %v", got, want)
		}

		// A filtered aggregate used as a window function.
		win, err := momo.Select[momo.Row2[string, int64]](db).From(docs).Columns(
			suite.DocTitle, momo.Sum(suite.DocVersion).Filter(big).Over(momo.Over().PartitionBy(suite.DocTenantID))).
			OrderBy(suite.DocTitle.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var sums []int64
		for _, r := range win {
			sums = append(sums, r.C2)
		}
		if want := []int64{20, 20, 70, 70, 50}; !slices.Equal(sums, want) {
			t.Errorf("filtered window sums = %v, want %v", sums, want)
		}
	})
}
