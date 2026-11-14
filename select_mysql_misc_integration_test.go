// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/sqlfn"
)

// Catches: row-value comparison, CURRENT_DATE and the Explain and infinity
// refusals misbehaving on a MySQL-family server: a row comparison that treats
// its elements one by one, a date that is not the session date, an Explain
// that sends SQL the server cannot summarise, and a one-element row that is
// sent to the server instead of ErrValidation.
func TestRowValuesAndDatesOnMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		seedAnalytics(t, db)
		ctx := context.Background()
		titles := func(p momo.Predicate) []string {
			t.Helper()
			rows, err := momo.Select[suite.Document](db).Where(p).OrderBy(suite.DocTitle.Asc()).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			out := make([]string, len(rows))
			for i, r := range rows {
				out[i] = r.Title
			}
			return out
		}
		row := momo.Row(suite.DocTenantID, suite.DocVersion)
		bound := momo.Row(momo.Value(int64(3)), momo.Value(int64(8)))
		if got := titles(row.Lt(bound)); !slices.Equal(got, []string{"a", "b", "c", "d", "e", "f"}) {
			t.Errorf("row < (3, 8) = %v", got)
		}
		if got := titles(row.Ge(bound)); !slices.Equal(got, []string{"g"}) {
			t.Errorf("row >= (3, 8) = %v", got)
		}

		// Typed predicates beyond the shared suite: Between, In, NotIn, the
		// NULL-safe comparisons (<=> on MySQL) and the boolean combinators.
		if got := titles(suite.DocVersion.Between(5, 9)); !slices.Equal(got, []string{"d", "e", "f", "g"}) {
			t.Errorf("Between(5, 9) = %v", got)
		}
		if got := titles(suite.DocTitle.In("a", "d", "zz")); !slices.Equal(got, []string{"a", "d"}) {
			t.Errorf("In = %v", got)
		}
		if got := titles(suite.DocTitle.NotIn("a", "b", "c", "d")); !slices.Equal(got, []string{"e", "f", "g"}) {
			t.Errorf("NotIn = %v", got)
		}
		if got := titles(suite.DocBody.IsDistinctFrom("x")); len(got) != 7 {
			t.Errorf("NULL body IS DISTINCT FROM 'x' matched %d rows, want 7", len(got))
		}
		if got := titles(suite.DocBody.IsNotDistinctFrom("x")); len(got) != 0 {
			t.Errorf("NULL body IS NOT DISTINCT FROM 'x' matched %v, want none", got)
		}
		if got := titles(momo.Or(suite.DocTenantID.Eq(2), momo.And(suite.DocTenantID.Eq(3), suite.DocVersion.Gt(7)))); !slices.Equal(got, []string{"d", "g"}) {
			t.Errorf("Or/And = %v", got)
		}
		if got := titles(momo.Not(suite.DocTenantID.Eq(1))); !slices.Equal(got, []string{"d", "e", "f", "g"}) {
			t.Errorf("Not = %v", got)
		}

		_, err := momo.Select[suite.Document](db).Where(momo.Row(suite.DocTenantID).Lt(momo.Row(momo.Value(int64(1))))).All(ctx)
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("one-element row = %v, want ErrValidation", err)
		}

		day, err := momo.Select[anT](db).From(anDocs).Columns(sqlfn.CurrentDate()).Limit(1).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if d := time.Since(day.T); d < -48*time.Hour || d > 48*time.Hour || day.T.Hour() != 0 {
			t.Errorf("CURRENT_DATE = %v, want today's date at midnight", day.T)
		}
		if _, err := momo.Select[anT](db).From(anDocs).Columns(sqlfn.Infinity()).Limit(1).All(ctx); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("Infinity = %v, want ErrUnsupported", err)
		}
		if _, err := momo.Select[suite.Document](db).Explain(ctx); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("Explain = %v, want ErrUnsupported", err)
		}
	})
}

// Catches: bulk-insert chunking on a MySQL-family server writing a chunk
// larger than WithMaxParams allows, losing rows between chunks, or writing a
// single row that does not fit instead of failing with ErrLimitExceeded.
func TestInsertChunkingHonorsMaxParamsOnMySQLFamily(t *testing.T) {
	// Five columns per row: a limit of 12 binds takes two rows per statement.
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		ms := make([]suite.Document, 10)
		for i := range ms {
			ms[i] = suite.Document{TenantID: int64(i), Title: "chunk"}
		}
		res, err := momo.Insert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(ctx)
		if err != nil || res.RowsAffected != 10 {
			t.Fatalf("insert = %+v, %v", res, err)
		}
		if n, err := momo.Select[suite.Document](db).Count(ctx); err != nil || n != 10 {
			t.Fatalf("count = %d, %v, want 10", n, err)
		}
	}, momo.WithMaxParams(12))

	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		ms := []suite.Document{{TenantID: 1, Title: "too wide"}}
		_, err := momo.Insert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(ctx)
		if !errors.Is(err, momo.ErrLimitExceeded) {
			t.Fatalf("one row over the limit = %v, want ErrLimitExceeded", err)
		}
		if n, err := momo.Select[suite.Document](db).Count(ctx); err != nil || n != 0 {
			t.Fatalf("count = %d, %v, want 0", n, err)
		}
	}, momo.WithMaxParams(3))
}
