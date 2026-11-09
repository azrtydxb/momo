// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// kvRow is a two-field projection: a key and a count.
type kvRow struct {
	K int64
	N int64
}

var kvRowMeta = momo.Meta[kvRow]{
	Columns:  []momo.ColumnMeta{{Name: "k"}, {Name: "n"}},
	Ptr:      func(m *kvRow, i int) any { return [...]any{&m.K, &m.N}[i] },
	Value:    func(m *kvRow, i int) any { return [...]any{m.K, m.N}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*kvRow) MomoMeta() *momo.Meta[kvRow] { return &kvRowMeta }

// strCount is a projection of a string key and a count.
type strCount struct {
	S string
	N int64
}

var strCountMeta = momo.Meta[strCount]{
	Columns:  []momo.ColumnMeta{{Name: "s"}, {Name: "n"}},
	Ptr:      func(m *strCount, i int) any { return [...]any{&m.S, &m.N}[i] },
	Value:    func(m *strCount, i int) any { return [...]any{m.S, m.N}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*strCount) MomoMeta() *momo.Meta[strCount] { return &strCountMeta }

var pairK = momo.ColumnOf[int64]("s", "tenant_id")

func typedDB(name dialect.Name) *momo.DB { return momo.New(fakedriver.New(name)) }

// quoteFor rewrites a PostgreSQL-style golden string for MySQL's backticks.
func quoteFor(d dialect.Name, s string) string {
	if d == dialect.MySQL {
		return strings.ReplaceAll(s, `"`, "`")
	}
	return s
}

var typedDialects = []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL}

// Catches: the alias dropped or unquoted, and the alias not following the
// dialect's identifier quoting.
func TestAliasGolden(t *testing.T) {
	const want = `SELECT "documents"."tenant_id", COUNT(*) AS "n" FROM "documents" GROUP BY "documents"."tenant_id"`
	for _, d := range typedDialects {
		got, err := momo.Select[kvRow](typedDB(d)).From(momo.TableRef{Name: "documents"}).
			Columns(DocTenantID, momo.CountAll().As("n")).GroupBy(DocTenantID).SQL()
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		if got.SQL != quoteFor(d, want) {
			t.Errorf("%s:\n got %s\nwant %s", d, got.SQL, quoteFor(d, want))
		}
	}
}

func aggregateInner(db *momo.DB) momo.SelectQuery[kvRow] {
	return momo.Select[kvRow](db).From(momo.TableRef{Name: "documents"}).
		Columns(DocTenantID, momo.CountAll().As("n")).GroupBy(DocTenantID)
}

// Catches: Aliased.Ref not naming the alias, so the outer query cannot
// reference the aggregate by name from a FROM subquery or a CTE.
func TestAliasReferencedFromOuterQuery(t *testing.T) {
	db := typedDB(dialect.Postgres)
	const inner = `(SELECT "documents"."tenant_id", COUNT(*) AS "n" FROM "documents" GROUP BY "documents"."tenant_id")`
	n := momo.CountAll().As("n")

	got, err := momo.Select[kvRow](db).From(momo.FromSubquery(aggregateInner(db), "s")).
		Columns(pairK, n.Ref("s")).Where(n.Ref("s").Gt(1)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "s"."tenant_id", "s"."n" FROM ` + inner + ` AS "s" WHERE "s"."n" > $1`
	if got.SQL != want || len(got.Args) != 1 {
		t.Errorf("subquery:\n got %s %v\nwant %s", got.SQL, got.Args, want)
	}

	got, err = momo.Select[kvRow](db).With("s", aggregateInner(db)).From(momo.CTE("s")).
		Columns(pairK, n.Ref("s")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want = `WITH "s" AS ` + inner + ` SELECT "s"."tenant_id", "s"."n" FROM "s"`
	if got.SQL != want {
		t.Errorf("cte:\n got %s\nwant %s", got.SQL, want)
	}
}

// Catches: aliases rendered verbatim (injection, reserved words unquoted) or
// accepted when empty or malformed.
func TestAliasValidation(t *testing.T) {
	db := typedDB(dialect.Postgres)
	build := func(alias string) (momo.Compiled, error) {
		return momo.Select[kvRow](db).From(momo.TableRef{Name: "documents"}).
			Columns(DocTenantID, momo.As[int64](momo.CountAll(), alias)).SQL()
	}
	got, err := build("order")
	if err != nil || !strings.Contains(got.SQL, `COUNT(*) AS "order"`) {
		t.Errorf("reserved word alias = %q, %v; want it quoted", got.SQL, err)
	}
	for _, bad := range []string{"", "a b", `x" FROM t; --`, "1a", "a-b", strings.Repeat("a", 64)} {
		if _, err := build(bad); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("alias %q err = %v, want ErrValidation", bad, err)
		}
	}
}

// Catches: Func binding a plain argument inline or dropping the schema
// qualifier of the function name.
func TestFuncGolden(t *testing.T) {
	cases := []struct {
		e    momo.Expr
		want string
	}{
		{momo.Func[string]("coalesce", DocTitle, "x"), `coalesce("documents"."title", $1)`},
		{momo.Func[string]("pg_catalog.lower", DocTitle), `pg_catalog.lower("documents"."title")`},
	}
	for _, c := range cases {
		got, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).Columns(c.e).SQL()
		if want := `SELECT ` + c.want + ` FROM "documents"`; err != nil || got.SQL != want {
			t.Errorf("func:\n got %s (%v)\nwant %s", got.SQL, err, want)
		}
	}
}

// Catches: a function name reaching the SQL text unchecked.
func TestFuncNameValidation(t *testing.T) {
	for _, bad := range []string{"", "lower; DROP TABLE x", "a b", "1x", "a..b", "f(", `"q"`} {
		_, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
			Columns(momo.Func[string](bad, DocTitle)).SQL()
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("name %q err = %v, want ErrValidation", bad, err)
		}
	}
}
