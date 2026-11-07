// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: a computed expression missing Asc/Desc or accepted by OrderBy in
// the wrong form (ASC dropped, alias rendered with AS in ORDER BY).
func TestOrderByComputedExpressions(t *testing.T) {
	cases := []struct {
		name  string
		order momo.Orderable
		want  string
	}{
		{"lower asc", momo.Lower(DocTitle), `ORDER BY lower("documents"."title") ASC`},
		{"lower desc", momo.Lower(DocTitle).Desc(), `ORDER BY lower("documents"."title") DESC`},
		{"substr", momo.Substr(DocTitle, 1, 3).Asc(), `ORDER BY substr("documents"."title", 1, 3) ASC`},
		{"case", momo.Case[int64]().When(DocTenantID.Eq(1), 0).Else(1).Desc(), `ORDER BY CASE WHEN "documents"."tenant_id" = $1 THEN $2 ELSE $3 END DESC`},
		{"arith", momo.Add(DocTenantID, 5).Desc(), `ORDER BY ("documents"."tenant_id" + $1) DESC`},
		{"cast", momo.Cast[int64](DocTitle, "bigint"), `ORDER BY CAST("documents"."title" AS bigint) ASC`},
		{"concat", momo.Concat(DocTitle, DocTitle).Desc(), `ORDER BY ("documents"."title" || "documents"."title") DESC`},
		{"length", momo.Length(DocTitle).Desc(), `ORDER BY length("documents"."title") DESC`},
		{"alias", momo.CountAll().As("n").Desc(), `ORDER BY "n" DESC`},
	}
	for _, c := range cases {
		got, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
			Columns(momo.CountAll()).OrderBy(c.order).SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !strings.HasSuffix(got.SQL, c.want) {
			t.Errorf("%s:\n got %s\nwant suffix %s", c.name, got.SQL, c.want)
		}
	}
}

// Catches: a computed expression without the comparison methods of Column,
// or comparisons binding the value in the wrong place.
func TestWhereOnComputedExpressions(t *testing.T) {
	cases := []struct {
		name string
		p    momo.Predicate
		want string
		args []any
	}{
		{"substr eq", momo.Substr(DocTitle, 1, 3).Eq("abc"), `substr("documents"."title", 1, 3) = $1`, []any{"abc"}},
		{"lower in", momo.Lower(DocTitle).In("a", "b"), `lower("documents"."title") IN ($1, $2)`, []any{"a", "b"}},
		{"upper ne", momo.Upper(DocTitle).Ne("X"), `upper("documents"."title") <> $1`, []any{"X"}},
		{"length ge", momo.Length(DocTitle).Ge(3), `length("documents"."title") >= $1`, []any{int64(3)}},
		{"arith gt", momo.Mul(DocTenantID, 2).Gt(10), `("documents"."tenant_id" * $1) > $2`, []any{int64(2), int64(10)}},
		{"arith eq expr", momo.Add(DocTenantID, 1).EqExpr(DocTenantID), `("documents"."tenant_id" + $1) = "documents"."tenant_id"`, []any{int64(1)}},
		{"case eq", momo.Case[string]().When(DocTenantID.Eq(1), "a").Else("b").Eq("a"), `CASE WHEN "documents"."tenant_id" = $1 THEN $2 ELSE $3 END = $4`, []any{int64(1), "a", "b", "a"}},
		{"cast is null", momo.Cast[int64](DocTitle, "bigint").IsNull(), `CAST("documents"."title" AS bigint) IS NULL`, nil},
		{"func lt", momo.Func[int64]("abs", DocTenantID).Lt(4), `abs("documents"."tenant_id") < $1`, []any{int64(4)}},
		{"concat between", momo.Concat(DocTitle, DocTitle).Between("a", "z"), `("documents"."title" || "documents"."title") BETWEEN $1 AND $2`, []any{"a", "z"}},
	}
	for _, c := range cases {
		got, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
			Columns(momo.CountAll()).Where(c.p).SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !strings.HasSuffix(got.SQL, "WHERE "+c.want) || !slices.Equal(got.Args, c.args) {
			t.Errorf("%s:\n got %s %v\nwant WHERE %s %v", c.name, got.SQL, got.Args, c.want, c.args)
		}
	}
}

// Catches: an alias rendered inside GROUP BY (GROUP BY COUNT(*) AS "n").
func TestGroupByRejectsAlias(t *testing.T) {
	_, err := momo.Select[kvRow](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
		Columns(DocTenantID, momo.CountAll()).GroupBy(momo.CountAll().As("n")).SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("GroupBy(alias) err = %v, want ErrValidation", err)
	}
}

// Catches: a CASE built without its constructor panicking at render time
// instead of failing the build.
func TestZeroCaseComparisonFailsBuild(t *testing.T) {
	var c momo.CaseExpr[string]
	_, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
		Columns(momo.CountAll()).Where(c.Eq("a")).SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("zero CaseExpr comparison err = %v, want ErrValidation", err)
	}
}

// Catches: CAST on SQLite rendering a type the other dialects need.
func TestCastGoldenSQLite(t *testing.T) {
	got, err := momo.Select[unitTally](typedDB(dialect.SQLite)).From(momo.TableRef{Name: "documents"}).
		Columns(momo.Cast[int64](DocTitle, "integer")).SQL()
	want := `SELECT CAST("documents"."title" AS integer) FROM "documents"`
	if err != nil || got.SQL != want {
		t.Errorf("sqlite cast = %q, %v; want %s", got.SQL, err, want)
	}
}

// Catches: placeholder numbering restarting or skipping when a bound
// parameter inside a CTE is followed by one in the outer WHERE.
func TestPlaceholderNumberingAcrossCTEAndOuterWhere(t *testing.T) {
	db := typedDB(dialect.Postgres)
	inner := momo.Select[kvRow](db).From(momo.TableRef{Name: "documents"}).
		Columns(DocTenantID, momo.CountAll().As("n")).
		Where(DocTitle.Eq("inner")).GroupBy(DocTenantID)
	n := momo.CountAll().As("n")
	got, err := momo.Select[kvRow](db).With("t", inner).From(momo.CTE("t")).
		Columns(pairTenant("t"), n.Ref("t")).
		Where(n.Ref("t").Gt(7), pairTenant("t").Ne(9)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.SQL, `"documents"."title" = $1`) || !strings.Contains(got.SQL, `"t"."n" > $2`) ||
		!strings.Contains(got.SQL, `"t"."tenant_id" <> $3`) || strings.Contains(got.SQL, "$4") ||
		!slices.Equal(got.Args, []any{"inner", int64(7), int64(9)}) {
		t.Errorf("numbering: %s %v", got.SQL, got.Args)
	}
}

func pairTenant(source string) momo.Column[int64] {
	return momo.ColumnOf[int64](source, "tenant_id")
}
