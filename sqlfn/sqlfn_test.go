// SPDX-License-Identifier: Apache-2.0

package sqlfn_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/sqlfn"
)

var (
	docTitle     = momo.NewColumn[string]("documents", "title", 0)
	docTenantID  = momo.NewColumn[int64]("documents", "tenant_id", 0)
	docPublished = momo.NewColumn[bool]("documents", "published", 0)
)

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

// quoteFor rewrites a PostgreSQL-style golden string for MySQL's backticks.
func quoteFor(d dialect.Name, s string) string {
	if d == dialect.MySQL {
		return strings.ReplaceAll(s, `"`, "`")
	}
	return s
}

var typedDialects = []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL}

// Catches: positions bound as parameters (SELECT and GROUP BY would then
// differ and PostgreSQL rejects the grouping), and per-dialect names wrong.
func TestStringFunctionsGolden(t *testing.T) {
	sub := func() momo.FuncExpr[string] { return sqlfn.Substr(docTitle, 1, 3) }
	for _, d := range typedDialects {
		got, err := momo.Select[exprtest.Tally](exprtest.DB(d)).From(momo.TableRef{Name: "documents"}).
			Columns(sub()).GroupBy(sub()).SQL()
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		want := quoteFor(d, `SELECT substr("documents"."title", 1, 3) FROM "documents" GROUP BY substr("documents"."title", 1, 3)`)
		if got.SQL != want || len(got.Args) != 0 {
			t.Errorf("%s substr:\n got %s %v\nwant %s", d, got.SQL, got.Args, want)
		}
	}
	cases := []struct {
		d    dialect.Name
		e    momo.Expr
		want string
	}{
		{dialect.Postgres, sqlfn.SubstrFrom(docTitle, 4), `substr("documents"."title", 4)`},
		{dialect.Postgres, sqlfn.Lower(docTitle), `lower("documents"."title")`},
		{dialect.SQLite, sqlfn.Upper(docTitle), `upper("documents"."title")`},
		{dialect.Postgres, sqlfn.Length(docTitle), `length("documents"."title")`},
		{dialect.SQLite, sqlfn.Length(docTitle), `length("documents"."title")`},
		{dialect.MySQL, sqlfn.Length(docTitle), "char_length(`documents`.`title`)"},
		{dialect.Postgres, sqlfn.CurrentSchema(), `current_schema()`},
		{dialect.MySQL, sqlfn.CurrentSchema(), `DATABASE()`},
	}
	for _, c := range cases {
		got, err := exprtest.CompileColumns(c.d, c.e)
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
	_, err := exprtest.CompileColumns(dialect.SQLite, sqlfn.CurrentSchema())
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("current_schema on SQLite = %v, want ErrUnsupported", err)
	}
}

// Catches: a CASE rendered without its branches, an ELSE lost, an empty CASE
// accepted, and the branch values not bound in order.
func TestCaseGolden(t *testing.T) {
	run := func(e momo.Expr) (momo.Compiled, error) {
		return exprtest.CompileColumns(dialect.Postgres, e)
	}
	got, err := run(sqlfn.Case[string]().When(docPublished.Eq(true), "yes").When(docTenantID.Gt(5), "big").Else("no"))
	want := `SELECT CASE WHEN "documents"."published" = $1 THEN $2 WHEN "documents"."tenant_id" > $3 THEN $4 ELSE $5 END FROM "documents"`
	if err != nil || got.SQL != want || len(got.Args) != 5 {
		t.Errorf("case:\n got %s %v (%v)\nwant %s", got.SQL, got.Args, err, want)
	}
	got, err = run(sqlfn.Case[string]().WhenExpr(docPublished.Eq(true), docTitle).As("t"))
	want = `SELECT CASE WHEN "documents"."published" = $1 THEN "documents"."title" END AS "t" FROM "documents"`
	if err != nil || got.SQL != want {
		t.Errorf("case without else:\n got %s (%v)\nwant %s", got.SQL, err, want)
	}
	if _, err := run(sqlfn.Case[string]().Else("x")); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("CASE without When = %v, want ErrValidation", err)
	}
}

// Catches: a CASE built without its constructor panicking at render time
// instead of failing the build.
func TestZeroCaseComparisonFailsBuild(t *testing.T) {
	var c sqlfn.CaseExpr[string]
	_, err := exprtest.Compile(dialect.Postgres, c.Eq("a"))
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("zero CaseExpr comparison err = %v, want ErrValidation", err)
	}
}

func nodeUpsert(q momo.Querier) momo.InsertQuery[Node] {
	merge := sqlfn.Case[string]().
		WhenExpr(nodeHostname.Excluded().Ne(""), nodeHostname.Excluded()).
		ElseExpr(nodeHostname)
	return momo.Insert[Node](q).
		Values(nodeName.Set("a"), nodeHostname.Set("h"), nodeHits.Set(1)).
		OnConflict(nodeName).
		DoUpdate(
			nodeHostname.SetExpr(merge),
			nodeHits.SetExpr(sqlfn.AddExpr(nodeHits, nodeHits.Excluded())),
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
		got, err := nodeUpsert(exprtest.DB(c.d)).SQL()
		if err != nil || got.SQL != c.want || len(got.Args) != 4 {
			t.Errorf("%s:\n got %s %v (%v)\nwant %s", c.d, got.SQL, got.Args, err, c.want)
		}
	}
	my, err := nodeUpsert(exprtest.DB(dialect.MySQL)).SQL()
	if err != nil || !strings.Contains(my.SQL, "ON DUPLICATE KEY UPDATE") || strings.Contains(my.SQL, "EXCLUDED") {
		t.Errorf("upsert on MySQL = %s, %v, want ON DUPLICATE KEY UPDATE without EXCLUDED", my.SQL, err)
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
		got, err := momo.Update[Node](exprtest.DB(d)).
			Set(
				nodeHits.SetExpr(sqlfn.Mul(sqlfn.Add(nodeHits, 5), 2)),
				nodeHostname.SetExpr(sqlfn.Lower(nodeHostname)),
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
	got, err := momo.Update[Node](exprtest.DB(dialect.Postgres)).
		Set(nodeHits.SetExpr(sqlfn.SubExpr(nodeHits, momo.Value[int64](1)))).
		Where(nodeName.Eq("a")).SQL()
	if want := `UPDATE "nodes" SET "hits" = ("nodes"."hits" - $1) WHERE "nodes"."name" = $2`; err != nil || got.SQL != want {
		t.Errorf("sub:\n got %s (%v)\nwant %s", got.SQL, err, want)
	}
}

// Catches: COALESCE over expressions dropping an argument, CAST rendered with
// the wrong type text, and concat using the wrong operator per dialect.
func TestCoalesceCastConcatGolden(t *testing.T) {
	cases := []struct {
		d    dialect.Name
		e    momo.Expr
		want string
	}{
		{dialect.Postgres, sqlfn.CoalesceExpr(docTitle, sqlfn.Lower(docTitle)), `coalesce("documents"."title", lower("documents"."title"))`},
		{dialect.SQLite, sqlfn.CoalesceExpr(docTitle, docTitle, sqlfn.Upper(docTitle)), `coalesce("documents"."title", "documents"."title", upper("documents"."title"))`},
		{dialect.Postgres, sqlfn.Cast[int64](momo.Sum(docTenantID), "bigint"), `CAST(SUM("documents"."tenant_id") AS bigint)`},
		{dialect.Postgres, sqlfn.Cast[float64](docTenantID, "numeric(10,2)").As("v"), `CAST("documents"."tenant_id" AS numeric(10,2)) AS "v"`},
		{dialect.Postgres, sqlfn.Cast[float64](docTenantID, "double precision"), `CAST("documents"."tenant_id" AS double precision)`},
		{dialect.MySQL, sqlfn.Cast[int64](docTenantID, "signed"), "CAST(`documents`.`tenant_id` AS signed)"},
		{dialect.Postgres, sqlfn.Concat(docTitle, momo.Value("-x"), docTitle), `("documents"."title" || $1 || "documents"."title")`},
		{dialect.SQLite, sqlfn.Concat(docTitle, docTitle), `("documents"."title" || "documents"."title")`},
		{dialect.MySQL, sqlfn.Concat(docTitle, docTitle), "CONCAT(`documents`.`title`, `documents`.`title`)"},
	}
	for _, c := range cases {
		got, err := exprtest.CompileColumns(c.d, c.e)
		if err != nil {
			t.Errorf("%s %s: %v", c.d, c.want, err)
			continue
		}
		if want := quoteFor(c.d, "SELECT "+c.want+` FROM "documents"`); got.SQL != want {
			t.Errorf("%s:\n got %s\nwant %s", c.d, got.SQL, want)
		}
	}
}

// Catches: CAST on SQLite rendering a type the other dialects need.
func TestCastGoldenSQLite(t *testing.T) {
	got, err := exprtest.CompileColumns(dialect.SQLite, sqlfn.Cast[int64](docTitle, "integer"))
	want := `SELECT CAST("documents"."title" AS integer) FROM "documents"`
	if err != nil || got.SQL != want {
		t.Errorf("sqlite cast = %q, %v; want %s", got.SQL, err, want)
	}
}

// Catches: a CAST type name reaching the SQL text unchecked.
func TestCastTypeValidation(t *testing.T) {
	for _, bad := range []string{"", "bigint; DROP TABLE x", "big)int", "a(", "numeric(x)", "numeric()", "x--", "a  b", "int(1,)", "(10)"} {
		_, err := exprtest.CompileColumns(dialect.Postgres, sqlfn.Cast[int64](docTenantID, bad))
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("type %q err = %v, want ErrValidation", bad, err)
		}
	}
}

// Catches: a moved expression binding the value compared with a sensitive
// column through the plain argument path, leaking it into hook events.
func TestComparisonsMaskSensitiveColumn(t *testing.T) {
	for _, sensitive := range []bool{true, false} {
		var fl uint16
		if sensitive {
			fl = momo.FlagSensitive
		}
		secret := momo.NewColumn[string]("docs", "secret", fl)
		n := momo.NewColumn[int64]("docs", "n", fl)
		visible := momo.NewColumn[int64]("docs", "id", 0)
		preds := map[string]momo.Predicate{
			"length":   sqlfn.Length(secret).Eq(7),
			"arith":    sqlfn.Add(n, 1).Eq(7),
			"cast":     sqlfn.Cast[string](secret, "text").Eq("x"),
			"concat":   sqlfn.Concat(secret, secret).Eq("x"),
			"case":     sqlfn.Case[string]().WhenExpr(visible.Eq(1), secret).Else("z").Eq("x"),
			"coalesce": sqlfn.CoalesceExpr(secret, secret).Eq("x"),
		}
		for name, p := range preds {
			rec := &obstest.Recorder{}
			db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec), momo.WithArgLogging())
			if _, err := momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "docs"}).Columns(momo.CountAll()).Where(p).All(t.Context()); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			evs := rec.Events()
			if len(evs) == 0 {
				t.Fatalf("%s: no event", name)
			}
			args := evs[len(evs)-1].Args
			masked := 0
			for _, a := range args {
				if a == momo.RedactedArg {
					masked++
				}
			}
			if sensitive && masked == 0 {
				t.Errorf("%s: sensitive column but no masked argument in %v", name, args)
			}
			if !sensitive && masked != 0 {
				t.Errorf("%s: visible column but masked arguments in %v", name, args)
			}
		}
	}
}

// Catches: the comparison and ordering methods lost from the moved
// expressions, so a function result cannot be filtered or sorted on.
func TestFunctionResultsCompareAndOrder(t *testing.T) {
	cases := []struct {
		name  string
		order momo.Orderable
		want  string
	}{
		{"lower asc", sqlfn.Lower(docTitle), `ORDER BY lower("documents"."title") ASC`},
		{"lower desc", sqlfn.Lower(docTitle).Desc(), `ORDER BY lower("documents"."title") DESC`},
		{"substr", sqlfn.Substr(docTitle, 1, 3).Asc(), `ORDER BY substr("documents"."title", 1, 3) ASC`},
		{"case", sqlfn.Case[int64]().When(docTenantID.Eq(1), 0).Else(1).Desc(), `ORDER BY CASE WHEN "documents"."tenant_id" = $1 THEN $2 ELSE $3 END DESC`},
		{"arith", sqlfn.Add(docTenantID, 5).Desc(), `ORDER BY ("documents"."tenant_id" + $1) DESC`},
		{"cast", sqlfn.Cast[int64](docTitle, "bigint"), `ORDER BY CAST("documents"."title" AS bigint) ASC`},
		{"concat", sqlfn.Concat(docTitle, docTitle).Desc(), `ORDER BY ("documents"."title" || "documents"."title") DESC`},
		{"length", sqlfn.Length(docTitle).Desc(), `ORDER BY length("documents"."title") DESC`},
	}
	for _, c := range cases {
		got, err := momo.Select[exprtest.Tally](exprtest.DB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
			Columns(momo.CountAll()).OrderBy(c.order).SQL()
		if err != nil || !strings.HasSuffix(got.SQL, c.want) {
			t.Errorf("%s:\n got %s (%v)\nwant suffix %s", c.name, got.SQL, err, c.want)
		}
	}
	preds := []struct {
		name string
		p    momo.Predicate
		want string
		args []any
	}{
		{"substr eq", sqlfn.Substr(docTitle, 1, 3).Eq("abc"), `substr("documents"."title", 1, 3) = $1`, []any{"abc"}},
		{"lower in", sqlfn.Lower(docTitle).In("a", "b"), `lower("documents"."title") IN ($1, $2)`, []any{"a", "b"}},
		{"upper ne", sqlfn.Upper(docTitle).Ne("X"), `upper("documents"."title") <> $1`, []any{"X"}},
		{"length ge", sqlfn.Length(docTitle).Ge(3), `length("documents"."title") >= $1`, []any{int64(3)}},
		{"arith gt", sqlfn.Mul(docTenantID, 2).Gt(10), `("documents"."tenant_id" * $1) > $2`, []any{int64(2), int64(10)}},
		{"arith eq expr", sqlfn.Add(docTenantID, 1).EqExpr(docTenantID), `("documents"."tenant_id" + $1) = "documents"."tenant_id"`, []any{int64(1)}},
		{"case eq", sqlfn.Case[string]().When(docTenantID.Eq(1), "a").Else("b").Eq("a"), `CASE WHEN "documents"."tenant_id" = $1 THEN $2 ELSE $3 END = $4`, []any{int64(1), "a", "b", "a"}},
		{"cast is null", sqlfn.Cast[int64](docTitle, "bigint").IsNull(), `CAST("documents"."title" AS bigint) IS NULL`, nil},
		{"concat between", sqlfn.Concat(docTitle, docTitle).Between("a", "z"), `("documents"."title" || "documents"."title") BETWEEN $1 AND $2`, []any{"a", "z"}},
		{"current schema eq", sqlfn.CurrentSchema().Eq("public"), `current_schema() = $1`, []any{"public"}},
	}
	for _, c := range preds {
		got, err := momo.Select[exprtest.Tally](exprtest.DB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
			Columns(momo.CountAll()).Where(c.p).SQL()
		want := "SELECT COUNT(*) FROM \"documents\" WHERE " + c.want
		if err != nil || got.SQL != want || len(got.Args) != len(c.args) {
			t.Errorf("%s:\n got %s %v (%v)\nwant %s %v", c.name, got.SQL, got.Args, err, want, c.args)
			continue
		}
		for i := range c.args {
			if got.Args[i] != c.args[i] {
				t.Errorf("%s: arg %d = %v, want %v", c.name, i, got.Args[i], c.args[i])
			}
		}
	}
}
