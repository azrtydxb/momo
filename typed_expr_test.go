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

// Node is a table with a text column to merge and a counter to accumulate.
type Node struct {
	Name     string
	Hostname string
	Hits     int64
}

var (
	nodeName     = momo.NewColumn[string]("nodes", "name", momo.FlagPK)
	nodeHostname = momo.NewColumn[string]("nodes", "hostname", 0)
	nodeHits     = momo.NewColumn[int64]("nodes", "hits", 0)
)

var nodeMeta = &momo.Meta[Node]{
	Table: "nodes",
	Columns: []momo.ColumnMeta{
		{Name: "name", Flags: nodeName.Flags()},
		{Name: "hostname", Flags: nodeHostname.Flags()},
		{Name: "hits", Flags: nodeHits.Flags()},
	},
	Ptr:      func(m *Node, i int) any { return [...]any{&m.Name, &m.Hostname, &m.Hits}[i] },
	Value:    func(m *Node, i int) any { return [...]any{m.Name, m.Hostname, m.Hits}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*Node) MomoMeta() *momo.Meta[Node] { return nodeMeta }

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

// Catches: positions bound as parameters (SELECT and GROUP BY would then
// differ and PostgreSQL rejects the grouping), and per-dialect names wrong.
func TestStringFunctionsGolden(t *testing.T) {
	sub := func() momo.FuncExpr[string] { return momo.Substr(DocTitle, 1, 3) }
	for _, d := range typedDialects {
		db := typedDB(d)
		got, err := momo.Select[strCount](db).From(momo.TableRef{Name: "documents"}).
			Columns(sub(), momo.CountAll()).GroupBy(sub()).SQL()
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		want := quoteFor(d, `SELECT substr("documents"."title", 1, 3), COUNT(*) FROM "documents" GROUP BY substr("documents"."title", 1, 3)`)
		if got.SQL != want || len(got.Args) != 0 {
			t.Errorf("%s substr:\n got %s %v\nwant %s", d, got.SQL, got.Args, want)
		}
	}
	cases := []struct {
		d    dialect.Name
		e    momo.Expr
		want string
	}{
		{dialect.Postgres, momo.SubstrFrom(DocTitle, 4), `substr("documents"."title", 4)`},
		{dialect.Postgres, momo.Lower(DocTitle), `lower("documents"."title")`},
		{dialect.SQLite, momo.Upper(DocTitle), `upper("documents"."title")`},
		{dialect.Postgres, momo.Length(DocTitle), `length("documents"."title")`},
		{dialect.SQLite, momo.Length(DocTitle), `length("documents"."title")`},
		{dialect.MySQL, momo.Length(DocTitle), "char_length(`documents`.`title`)"},
		{dialect.Postgres, momo.Func[string]("coalesce", DocTitle, "x"), `coalesce("documents"."title", $1)`},
		{dialect.Postgres, momo.Func[string]("pg_catalog.lower", DocTitle), `pg_catalog.lower("documents"."title")`},
		{dialect.Postgres, momo.CurrentSchema(), `current_schema()`},
		{dialect.MySQL, momo.CurrentSchema(), `DATABASE()`},
	}
	for _, c := range cases {
		got, err := momo.Select[unitTally](typedDB(c.d)).From(momo.TableRef{Name: "documents"}).Columns(c.e).SQL()
		if err != nil {
			t.Errorf("%s %s: %v", c.d, c.want, err)
			continue
		}
		if want := quoteFor(c.d, "SELECT "+c.want+` FROM "documents"`); got.SQL != want {
			t.Errorf("%s:\n got %s\nwant %s", c.d, got.SQL, want)
		}
	}
}

// Catches: current_schema() silently rendered where SQLite has no schemas.
func TestCurrentSchemaRefusedOnSQLite(t *testing.T) {
	_, err := momo.Select[unitTally](typedDB(dialect.SQLite)).From(momo.TableRef{Name: "documents"}).Columns(momo.CurrentSchema()).SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("current_schema on SQLite = %v, want ErrUnsupported", err)
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

// Catches: a CASE rendered without its branches, an ELSE lost, an empty CASE
// accepted, and the branch values not bound in order.
func TestCaseGolden(t *testing.T) {
	db := typedDB(dialect.Postgres)
	run := func(e momo.Expr) (momo.Compiled, error) {
		return momo.Select[unitTally](db).From(momo.TableRef{Name: "documents"}).Columns(e).SQL()
	}
	got, err := run(momo.Case[string]().When(DocPublished.Eq(true), "yes").When(DocTenantID.Gt(5), "big").Else("no"))
	want := `SELECT CASE WHEN "documents"."published" = $1 THEN $2 WHEN "documents"."tenant_id" > $3 THEN $4 ELSE $5 END FROM "documents"`
	if err != nil || got.SQL != want || len(got.Args) != 5 {
		t.Errorf("case:\n got %s %v (%v)\nwant %s", got.SQL, got.Args, err, want)
	}
	got, err = run(momo.Case[string]().WhenExpr(DocPublished.Eq(true), DocTitle).As("t"))
	want = `SELECT CASE WHEN "documents"."published" = $1 THEN "documents"."title" END AS "t" FROM "documents"`
	if err != nil || got.SQL != want {
		t.Errorf("case without else:\n got %s (%v)\nwant %s", got.SQL, err, want)
	}
	if _, err := run(momo.Case[string]().Else("x")); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("CASE without When = %v, want ErrValidation", err)
	}
}

func nodeUpsert(q momo.Querier) momo.InsertQuery[Node] {
	merge := momo.Case[string]().
		WhenExpr(nodeHostname.Excluded().Ne(""), nodeHostname.Excluded()).
		ElseExpr(nodeHostname)
	return momo.Insert[Node](q).
		Values(nodeName.Set("a"), nodeHostname.Set("h"), nodeHits.Set(1)).
		OnConflict(nodeName).
		DoUpdate(
			nodeHostname.SetExpr(merge),
			nodeHits.SetExpr(momo.AddExpr(nodeHits, nodeHits.Excluded())),
		)
}

// Catches: Excluded comparisons losing the EXCLUDED qualifier, the CASE
// else branch qualified wrongly, and column arithmetic not parenthesised or
// not reading the existing row.
func TestUpsertCaseAndArithmeticGolden(t *testing.T) {
	const tail = `ON CONFLICT ("name") DO UPDATE SET "hostname" = CASE WHEN EXCLUDED."hostname" <> %s THEN EXCLUDED."hostname" ELSE "nodes"."hostname" END, "hits" = ("nodes"."hits" + EXCLUDED."hits")`
	cases := []struct {
		d    dialect.Name
		want string
	}{
		{dialect.Postgres, `INSERT INTO "nodes" ("name", "hostname", "hits") VALUES ($1, $2, $3) ` + strings.Replace(tail, "%s", "$4", 1)},
		{dialect.SQLite, `INSERT INTO "nodes" ("name", "hostname", "hits") VALUES (?, ?, ?) ` + strings.Replace(tail, "%s", "?", 1)},
	}
	for _, c := range cases {
		got, err := nodeUpsert(typedDB(c.d)).SQL()
		if err != nil || got.SQL != c.want || len(got.Args) != 4 {
			t.Errorf("%s:\n got %s %v (%v)\nwant %s", c.d, got.SQL, got.Args, err, c.want)
		}
	}
	if _, err := nodeUpsert(typedDB(dialect.MySQL)).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("upsert on MySQL = %v, want ErrUnsupported", err)
	}
}

// Catches: arithmetic operators swapped, operands unparenthesised when
// nested, and values interpolated instead of bound.
func TestUpdateArithmeticGolden(t *testing.T) {
	for _, d := range typedDialects {
		ph := func(n int) string {
			if d == dialect.Postgres {
				return "$" + string(rune('0'+n))
			}
			return "?"
		}
		got, err := momo.Update[Node](typedDB(d)).
			Set(
				nodeHits.SetExpr(momo.Mul(momo.Add(nodeHits, 5), 2)),
				nodeHostname.SetExpr(momo.Lower(nodeHostname)),
			).
			Where(nodeName.Eq("a")).SQL()
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		want := quoteFor(d, `UPDATE "nodes" SET "hits" = (("nodes"."hits" + `+ph(1)+`) * `+ph(2)+`), "hostname" = lower("nodes"."hostname") WHERE "nodes"."name" = `+ph(3))
		if got.SQL != want || len(got.Args) != 3 {
			t.Errorf("%s:\n got %s %v\nwant %s", d, got.SQL, got.Args, want)
		}
	}
	got, err := momo.Update[Node](typedDB(dialect.Postgres)).
		Set(nodeHits.SetExpr(momo.SubExpr(nodeHits, momo.Value[int64](1)))).
		Where(nodeName.Eq("a")).SQL()
	if want := `UPDATE "nodes" SET "hits" = ("nodes"."hits" - $1) WHERE "nodes"."name" = $2`; err != nil || got.SQL != want {
		t.Errorf("sub:\n got %s (%v)\nwant %s", got.SQL, err, want)
	}
}
