// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: a nested Union member that fails to run on a dialect, or loses its
// own UNION versus UNION ALL semantics across levels, or a member's WITH clause
// that does not bind to the member.
func TestSelectUnionNestedAndWithMembers(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedUnionDocs(t, db)
		ctx := context.Background()
		docs := momo.TableRef{Name: "documents"}
		titled := func(title string) momo.SelectQuery[momo.Row1[string]] {
			return momo.Select[momo.Row1[string]](db).From(docs).Columns(suite.DocTitle).Where(suite.DocTitle.Eq(title))
		}
		title := momo.ColumnOf[string]("", "title")
		titles := func(rows []momo.Row1[string]) []string {
			out := make([]string, len(rows))
			for i, r := range rows {
				out[i] = r.C1
			}
			return out
		}
		same := func(got, want []string) bool {
			if len(got) != len(want) {
				return false
			}
			for i := range got {
				if got[i] != want[i] {
					return false
				}
			}
			return true
		}

		// Outer UNION over a nested UNION ALL: the outer deduplicates, so the
		// repeated "a" collapses.
		distinct, err := momo.SelectUnion[momo.Row1[string]](db, false,
			titled("a"), momo.Union(true, titled("x"), titled("a")),
		).OrderBy(title.Asc()).All(ctx)
		if err != nil || !same(titles(distinct), []string{"a", "x"}) {
			t.Fatalf("UNION over nested UNION ALL = %v, %v; want [a x]", distinct, err)
		}

		// Outer UNION ALL over a nested UNION: the nested level deduplicates
		// its two "a" rows to one, the outer keeps its own "a" as well.
		kept, err := momo.SelectUnion[momo.Row1[string]](db, true,
			titled("a"), momo.Union(false, titled("a"), titled("a")),
		).OrderBy(title.Asc()).All(ctx)
		if err != nil || !same(titles(kept), []string{"a", "a"}) {
			t.Fatalf("UNION ALL over nested UNION = %v, %v; want [a a]", kept, err)
		}

		// A nested member that has its own LIMIT keeps it.
		limited, err := momo.SelectUnion[momo.Row1[string]](db, true,
			titled("a"),
			momo.Union(true,
				momo.Select[momo.Row1[string]](db).From(docs).Columns(suite.DocTitle).
					Where(suite.DocTenantID.Eq(2)).OrderBy(suite.DocTitle.Asc()).Limit(1),
				titled("c")),
		).OrderBy(title.Asc()).All(ctx)
		if err != nil || !same(titles(limited), []string{"a", "c", "x"}) {
			t.Fatalf("nested limited member = %v, %v; want [a c x]", limited, err)
		}

		// A member with its own WITH clause.
		cte := momo.Select[momo.Row1[string]](db).
			With("picked", titled("z")).
			From(momo.CTE("picked")).Columns(momo.ColumnOf[string]("picked", "title"))
		withRows, err := momo.SelectUnion[momo.Row1[string]](db, true, cte, titled("b")).
			OrderBy(title.Asc()).All(ctx)
		if err != nil || !same(titles(withRows), []string{"b", "z"}) {
			t.Fatalf("member with WITH = %v, %v; want [b z]", withRows, err)
		}
	})
}
