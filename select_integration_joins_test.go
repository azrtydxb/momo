// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// pairRow is a projection of two aliased title columns.
type pairRow struct {
	momo.Projection
	L, R string
}

var pairRowMeta = momo.Meta[pairRow]{
	Columns:    []momo.ColumnMeta{{Name: "l_title"}, {Name: "r_title"}},
	Ptr:        func(m *pairRow, i int) any { return []any{&m.L, &m.R}[i] },
	Value:      func(m *pairRow, i int) any { return []any{m.L, m.R}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
}

func (*pairRow) MomoMeta() *momo.Meta[pairRow] { return &pairRowMeta }

func selfJoin(db *momo.DB) momo.SelectQuery[pairRow] {
	left := momo.TableRef{Name: "documents", Alias: "l"}
	right := momo.TableRef{Name: "documents", Alias: "r"}
	return momo.Select[pairRow](db).
		From(left).
		Columns(suite.DocTitle.As("l").AsExpr(), suite.DocTitle.As("r").AsExpr()).
		Join(right, suite.DocTenantID.As("l").EqExpr(suite.DocTenantID.As("r")), suite.DocID.As("l").NeExpr(suite.DocID.As("r"))).
		OrderBy(suite.DocTitle.As("l").Asc(), suite.DocTitle.As("r").Asc())
}

// A self-join renders each side under its own quoted alias and never mixes
// the two sides up.
func TestSelfJoinSQLQualifiesEachSide(t *testing.T) {
	const want = `SELECT "l"."title", "r"."title" FROM "documents" AS "l" INNER JOIN "documents" AS "r" ON ("l"."tenant_id" = "r"."tenant_id") AND ("l"."id" <> "r"."id") ORDER BY "l"."title" ASC, "r"."title" ASC`
	for _, name := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		c, err := selfJoin(momo.New(fakedriver.New(name))).SQL()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c.SQL != want {
			t.Errorf("%s SQL =\n%s\nwant\n%s", name, c.SQL, want)
		}
	}
}

// A column of a table that is not a FROM or JOIN source is a build error,
// not a runtime SQL error.
func TestJoinColumnFromMissingTableErrors(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	right := momo.TableRef{Name: "documents", Alias: "r"}
	for name, on := range map[string]momo.Predicate{
		"unknown alias":         suite.DocTenantID.As("l").EqExpr(suite.DocTenantID.As("x")),
		"table hidden by alias": suite.DocTenantID.As("l").EqExpr(suite.DocTenantID),
	} {
		_, err := momo.Select[pairRow](db).
			From(momo.TableRef{Name: "documents", Alias: "l"}).
			Columns(suite.DocTitle.As("l").AsExpr(), suite.DocTitle.As("r").AsExpr()).
			Join(right, on).SQL()
		if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "not in the query") {
			t.Errorf("%s: err = %v, want ErrValidation naming the missing table", name, err)
		}
	}
}

// A correlated subquery may reference the enclosing query's table.
func TestCorrelatedSubqueryReferenceAllowed(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	inner := momo.Select[pairRow](db).
		From(momo.TableRef{Name: "documents", Alias: "i"}).
		Columns(suite.DocTitle.As("i").AsExpr(), suite.DocTitle.As("i").AsExpr()).
		Where(suite.DocTenantID.As("i").EqExpr(suite.DocTenantID.As("o")))
	if _, err := momo.Select[pairRow](db).
		From(momo.TableRef{Name: "documents", Alias: "o"}).
		Columns(suite.DocTitle.As("o").AsExpr(), suite.DocTitle.As("o").AsExpr()).
		Where(momo.Exists(inner)).SQL(); err != nil {
		t.Fatalf("correlated subquery: %v", err)
	}
	// The same subquery with nothing to correlate to is an error.
	if _, err := inner.SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("uncorrelated inner = %v, want ErrValidation", err)
	}
}
