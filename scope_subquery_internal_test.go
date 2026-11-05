// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// The per-SELECT scope check must accept every valid subquery and CTE shape
// and still reject a column of a table that is nowhere in scope. Each case
// would fail if the check over-rejected (or, for the last, under-rejected).
func TestScopeAcceptsSubqueryAndCTEShapes(t *testing.T) {
	docs := TableRef{Name: "documents"}
	tenant := NewColumn[int64]("documents", "tenant_id", 0)
	seqN := ColumnOf[int64]("seq", "n")
	bigT := ColumnOf[int64]("big", "tenant_id")
	row := func() SelectQuery[scopeRow] { return Select[scopeRow](nil) }

	recursive := Union(true,
		RawSubquery(`SELECT 1 AS n`),
		row().From(CTE("seq")).Columns(TypedSQL[int64](`"seq"."n" + 1`)).Where(seqN.Lt(5)),
	)
	cteBody := row().From(docs).Columns(tenant).Where(tenant.Ge(2))
	outer := NewColumn[int64]("o", "tenant_id", 0)
	inner := NewColumn[int64]("i", "tenant_id", 0)

	cases := []struct {
		name string
		q    SelectQuery[scopeRow]
		ok   bool
	}{
		{"recursive CTE referencing itself", row().WithRecursive("seq", recursive, "n").From(CTE("seq")).Columns(seqN), true},
		{"CTE referenced from main query", row().With("big", cteBody, "tenant_id").From(CTE("big")).Columns(bigT), true},
		{"CTE referenced from IN subquery", row().With("big", cteBody, "tenant_id").From(docs).Columns(tenant).
			Where(tenant.InQuery(row().From(CTE("big")).Columns(bigT))), true},
		{"scalar subquery in SELECT", row().From(docs).Columns(row().From(docs).Columns(Max(tenant))), true},
		{"IN subquery", row().From(docs).Columns(tenant).Where(tenant.InQuery(row().From(docs).Columns(tenant))), true},
		{"correlated EXISTS", row().From(TableRef{Name: "documents", Alias: "o"}).Columns(outer).
			Where(Exists(row().From(TableRef{Name: "documents", Alias: "i"}).Columns(inner).Where(inner.EqExpr(outer)))), true},
		{"correlated subquery two levels deep", row().From(TableRef{Name: "documents", Alias: "o"}).Columns(outer).
			Where(Exists(row().From(docs).Columns(tenant).
				Where(Exists(row().From(TableRef{Name: "documents", Alias: "i"}).Columns(inner).Where(inner.EqExpr(outer)))))), true},
		{"column of a table that is not in scope", row().From(docs).Columns(tenant).
			Where(tenant.InQuery(row().From(docs).Columns(NewColumn[int64]("elsewhere", "tenant_id", 0)))), false},
		{"correlated reference to a name no enclosing SELECT has", row().From(docs).Columns(tenant).
			Where(Exists(row().From(docs).Columns(tenant).Where(tenant.EqExpr(outer)))), false},
	}
	for _, name := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		for _, tc := range cases {
			_, err := compile(dialect.For(name), tc.q.render)
			if tc.ok && err != nil {
				t.Errorf("%s/%s: rejected: %v", name, tc.name, err)
			}
			if !tc.ok && !errors.Is(err, ErrValidation) {
				t.Errorf("%s/%s: err = %v, want ErrValidation", name, tc.name, err)
			}
		}
	}
}
