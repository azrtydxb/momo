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
		if got := titles(suite.DocBody.IsDistinctFrom("x")); !slices.Equal(got, []string{"a", "b", "c", "d", "e", "f", "g"}) {
			t.Errorf("NULL body IS DISTINCT FROM 'x' = %v, want every title", got)
		}
		if got := titles(suite.DocBody.IsNotDistinctFrom("x")); !slices.Equal(got, []string{}) {
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

		// The session time zone is UTC, so CURRENT_DATE is the UTC date; the
		// clock is read on both sides of the query to tolerate midnight.
		const dayLayout = "2006-01-02"
		before := time.Now().UTC().Format(dayLayout)
		day, err := momo.Select[anT](db).From(anDocs).Columns(sqlfn.CurrentDate()).Limit(1).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		after := time.Now().UTC().Format(dayLayout)
		if got := day.T.UTC().Format(dayLayout); (got != before && got != after) || day.T.UTC().Hour() != 0 {
			t.Errorf("CURRENT_DATE = %v, want the UTC date %s (or %s) at midnight", day.T, before, after)
		}
		if _, err := momo.Select[anT](db).From(anDocs).Columns(sqlfn.Infinity()).Limit(1).All(ctx); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("Infinity = %v, want ErrUnsupported", err)
		}
		if _, err := momo.Select[suite.Document](db).Explain(ctx); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("Explain = %v, want ErrUnsupported", err)
		}
	})
}

// mwInsertHook records the bind count of every INSERT statement.
type mwInsertHook struct{ inserts []int }

func (h *mwInsertHook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context {
	return ctx
}

func (h *mwInsertHook) AfterQuery(_ context.Context, e *momo.QueryEvent) {
	if e.Op == "insert" {
		h.inserts = append(h.inserts, e.NumArgs)
	}
}

// Catches: bulk-insert chunking on a MySQL-family server sending a statement
// with more binds than WithMaxParams allows, a wrong chunk count, losing rows
// between chunks, or writing a single row that does not fit instead of failing
// with ErrLimitExceeded.
func TestInsertChunkingHonorsMaxParamsOnMySQLFamily(t *testing.T) {
	// Five columns per row: a limit of 12 binds takes two rows (10 binds) per
	// statement, so ten rows are five statements.
	hook := &mwInsertHook{}
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		hook.inserts = nil
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
		if len(hook.inserts) != 5 {
			t.Errorf("insert statements = %d (binds %v), want 5", len(hook.inserts), hook.inserts)
		}
		for _, n := range hook.inserts {
			if n > 12 {
				t.Errorf("an insert bound %d parameters, over the limit of 12 (all: %v)", n, hook.inserts)
			}
		}
	}, momo.WithMaxParams(12), momo.WithHook(hook))

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
