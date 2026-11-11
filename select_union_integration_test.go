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

// unionHit is a plain struct: no generated metadata, columns matched by name.
type unionHit struct {
	TenantID int64
	Title    string
}

func seedUnionDocs(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	for _, row := range []struct {
		tenant int64
		title  string
	}{{1, "a"}, {1, "b"}, {1, "c"}, {2, "x"}, {2, "y"}, {2, "z"}} {
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", row.tenant, row.title); err != nil {
			t.Fatal(err)
		}
	}
}

// Catches: members with their own ORDER BY and LIMIT failing to run (a syntax
// error) or limiting the whole union instead of each member, and a union that
// cannot be executed at the top level with an outer ORDER BY and LIMIT, on
// every dialect.
func TestSelectUnionMemberLimitsAndOuterClauses(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedUnionDocs(t, db)
		ctx := context.Background()
		docs := momo.TableRef{Name: "documents"}
		member := func(tenant int64, desc bool) momo.SelectQuery[momo.Row2[int64, string]] {
			q := momo.Select[momo.Row2[int64, string]](db).From(docs).
				Columns(suite.DocTenantID, suite.DocTitle).Where(suite.DocTenantID.Eq(tenant))
			if desc {
				return q.OrderBy(suite.DocTitle.Desc()).Limit(2)
			}
			return q.OrderBy(suite.DocTitle.Asc()).Limit(2)
		}
		title := momo.ColumnOf[string]("", "title")

		// Top two of each tenant: c, b from tenant 1 and x, y from tenant 2.
		rows, err := momo.SelectUnion[unionHit](db, true, member(1, true), member(2, false)).
			OrderBy(title.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []unionHit{{1, "b"}, {1, "c"}, {2, "x"}, {2, "y"}}
		if len(rows) != len(want) {
			t.Fatalf("rows = %+v, want %+v", rows, want)
		}
		for i := range want {
			if rows[i] != want[i] {
				t.Fatalf("rows = %+v, want %+v", rows, want)
			}
		}

		// An outer LIMIT and OFFSET apply to the whole union.
		page, err := momo.SelectUnion[unionHit](db, true, member(1, true), member(2, false)).
			OrderBy(title.Desc()).Limit(2).Offset(1).All(ctx)
		if err != nil || len(page) != 2 || page[0].Title != "x" || page[1].Title != "c" {
			t.Fatalf("outer page = %+v, %v; want x, c", page, err)
		}

		// A tuple row and the streaming form read the same union.
		var titles []string
		for r, err := range momo.SelectUnion[momo.Row2[int64, string]](db, false,
			momo.Select[momo.Row2[int64, string]](db).From(docs).Columns(suite.DocTenantID, suite.DocTitle).
				Where(suite.DocTenantID.Eq(1)).OrderBy(suite.DocTitle.Asc()).Limit(1),
			momo.Select[momo.Row2[int64, string]](db).From(docs).Columns(suite.DocTenantID, suite.DocTitle).
				Where(suite.DocTenantID.Eq(2)).OrderBy(suite.DocTitle.Asc()).Limit(1),
		).OrderBy(title.Desc()).Rows(ctx) {
			if err != nil {
				t.Fatal(err)
			}
			titles = append(titles, r.C2)
		}
		if len(titles) != 2 || titles[0] != "x" || titles[1] != "a" {
			t.Fatalf("streamed titles = %v, want [x a]", titles)
		}

		// The same members as a derived table, through the existing Union.
		u := momo.Union(true, member(1, true), member(2, false))
		ut := momo.ColumnOf[string]("u", "title")
		derived, err := momo.Select[momo.Row1[string]](db).
			From(momo.FromSubquery(u, "u")).Columns(ut).OrderBy(ut.Asc()).All(ctx)
		if err != nil || len(derived) != 4 || derived[0].C1 != "b" || derived[3].C1 != "y" {
			t.Fatalf("derived union = %+v, %v; want b..y", derived, err)
		}
	})
}
