// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/vector"
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

func TestDistinctFromPerDialect(t *testing.T) {
	cases := []struct {
		d    dialect.Name
		p    momo.Predicate
		want string
	}{
		{dialect.Postgres, DocTitle.IsDistinctFrom("a"), `"documents"."title" IS DISTINCT FROM $1`},
		{dialect.Postgres, DocTitle.IsNotDistinctFrom("a"), `"documents"."title" IS NOT DISTINCT FROM $1`},
		{dialect.SQLite, DocTitle.IsDistinctFrom("a"), `"documents"."title" IS NOT ?`},
		{dialect.MySQL, DocTitle.IsDistinctFrom("a"), "NOT (`documents`.`title` <=> ?)"},
		{dialect.MySQL, DocTitle.IsNotDistinctFrom("a"), "`documents`.`title` <=> ?"},
	}
	for _, c := range cases {
		db := momo.New(fakedriver.New(c.d))
		got, err := momo.Select[unitDocument](db).Where(c.p).SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.d, err)
		}
		if !strings.Contains(got.SQL, c.want) {
			t.Errorf("%s: got %s, want it to contain %s", c.d, got.SQL, c.want)
		}
	}
}

// Variadic arguments are copied on store, so a caller reusing its slice
// cannot change a query built earlier.
func TestVariadicSlicesAreCloned(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	cases := []struct {
		name string
		mk   func() (q momo.SelectQuery[unitDocument], mutate func())
	}{
		{"In", func() (momo.SelectQuery[unitDocument], func()) {
			vs := []int64{1, 2}
			return momo.Select[unitDocument](pg).Where(DocTenantID.In(vs...)), func() { vs[0] = 99 }
		}},
		{"And", func() (momo.SelectQuery[unitDocument], func()) {
			ps := []momo.Predicate{DocTenantID.Eq(1), DocTenantID.Eq(2)}
			return momo.Select[unitDocument](pg).Where(momo.And(ps...)), func() { ps[0] = DocTenantID.Eq(99) }
		}},
		{"Or", func() (momo.SelectQuery[unitDocument], func()) {
			ps := []momo.Predicate{DocTenantID.Eq(1), DocTenantID.Eq(2)}
			return momo.Select[unitDocument](pg).Where(momo.Or(ps...)), func() { ps[0] = DocTenantID.Eq(99) }
		}},
		{"SQL", func() (momo.SelectQuery[unitDocument], func()) {
			args := []any{int64(1)}
			return momo.Select[unitDocument](pg).Where(momo.SQL("a = ?", args...)), func() { args[0] = int64(99) }
		}},
		{"TypedSQL", func() (momo.SelectQuery[unitDocument], func()) {
			args := []any{int64(1)}
			return momo.Select[unitDocument](pg).Where(momo.TypedSQL[bool]("a = ?", args...)), func() { args[0] = int64(99) }
		}},
		{"Union", func() (momo.SelectQuery[unitDocument], func()) {
			qs := []momo.Subquery{momo.Select[unitDocument](pg), momo.Select[unitDocument](pg)}
			return momo.Select[unitDocument](pg).From(momo.FromSubquery(momo.Union(true, qs...), "u")), func() { qs[1] = momo.Select[unitDocument](pg).Where(DocTenantID.Eq(99)) }
		}},
		{"With", func() (momo.SelectQuery[unitDocument], func()) {
			cols := []string{"x"}
			return momo.Select[unitDocument](pg).With("c", momo.Select[unitDocument](pg), cols...), func() { cols[0] = "mutated" }
		}},
	}
	for _, c := range cases {
		q, mutate := c.mk()
		want, err := q.SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		mutate()
		got, err := q.SQL()
		if err != nil {
			t.Fatalf("%s after mutation: %v", c.name, err)
		}
		if got.SQL != want.SQL || fmt.Sprint(got.Args) != fmt.Sprint(want.Args) {
			t.Errorf("%s: query changed after the caller mutated its slice:\n got %s %v\nwant %s %v", c.name, got.SQL, got.Args, want.SQL, want.Args)
		}
	}
}

// The types below are written the way a subpackage outside momo writes a
// custom expression: only exported names are used.

// extUpper is a string expression, upper(src). It embeds momo.Ops[string] so it
// gets Eq, In, Asc and the rest, and it forwards the source's flags so a
// sensitive column stays masked through the wrapper.
type extUpper struct {
	momo.Ops[string]
	src momo.Expr
}

func extUpperOf(src momo.Expr) extUpper {
	u := extUpper{src: src}
	u.Ops = momo.NewOps[string](u)
	return u
}

func (u extUpper) AppendSQL(b *momo.Builder) {
	b.WriteString("upper(")
	u.src.AppendSQL(b)
	b.WriteString(")")
}

func (u extUpper) Flags() uint16 { return momo.FlagsOf(u.src) }

// extPrefix is a boolean predicate, src LIKE prefix%. It embeds momo.Ops[bool].
type extPrefix struct {
	momo.Ops[bool]
	src    momo.Expr
	prefix string
}

func extPrefixOf(src momo.Expr, prefix string) extPrefix {
	p := extPrefix{src: src, prefix: prefix}
	p.Ops = momo.NewOps[bool](p)
	return p
}

func (p extPrefix) AppendSQL(b *momo.Builder) {
	fl := b.RenderSensitive(p.src, momo.FlagsOf(p.src))
	b.WriteString(" LIKE ")
	b.ArgFor(fl, p.prefix+"%")
}

// extNeedsPG renders only on PostgreSQL.
type extNeedsPG struct{ momo.Ops[string] }

func extNeedsPGExpr() extNeedsPG {
	e := extNeedsPG{}
	e.Ops = momo.NewOps[string](e)
	return e
}

func (extNeedsPG) AppendSQL(b *momo.Builder) {
	if b.Dialect().Name != dialect.Postgres {
		b.Unsupported("needs_pg()")
		return
	}
	b.WriteString("needs_pg()")
}

func extSelect(d dialect.Name) momo.SelectQuery[unitTally] {
	return momo.Select[unitTally](momo.New(fakedriver.New(d))).From(momo.TableRef{Name: "documents"})
}

// Catches: the extension types not satisfying Expression[T] from outside the
// package, or Ops losing the host binding so comparisons render the wrong
// operand.
func TestExtensionOpsCustomExpressions(t *testing.T) {
	got, err := extSelect(dialect.Postgres).
		Columns(extUpperOf(suite.DocTitle)).
		Where(extPrefixOf(suite.DocTitle, "ab"), extUpperOf(suite.DocTitle).Eq("X")).
		SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		`SELECT upper("documents"."title")`,
		`"documents"."title" LIKE $1`,
		`upper("documents"."title") = $2`,
	} {
		if !strings.Contains(got.SQL, frag) {
			t.Errorf("SQL %q lacks %q", got.SQL, frag)
		}
	}
	if want := []any{"ab%", "X"}; !slices.Equal(got.Args, want) {
		t.Errorf("args = %v, want %v", got.Args, want)
	}

	in, err := extSelect(dialect.Postgres).Columns(suite.DocTitle).Where(extUpperOf(suite.DocTitle).In("A", "B")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(in.SQL, `upper("documents"."title") IN ($1, $2)`) {
		t.Errorf("In SQL = %q", in.SQL)
	}
}

// Catches: Typed or PredicateFunc not producing a usable Expression, or the
// func predicate not reaching the builder.
func TestExtensionTypedAndPredicateFunc(t *testing.T) {
	typed := momo.Typed[string](extUpperOf(suite.DocTitle))
	fn := momo.PredicateFunc(func(b *momo.Builder) {
		b.WriteString("1 = ")
		b.Arg(1)
	})
	got, err := extSelect(dialect.Postgres).Columns(typed).Where(fn).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.SQL, `1 = $1`) {
		t.Errorf("SQL %q lacks the func predicate", got.SQL)
	}
	if want := []any{1}; !slices.Equal(got.Args, want) {
		t.Errorf("args = %v, want %v", got.Args, want)
	}
}

// Catches: ArgFor and FlagsOf not carrying the sensitive flag, so a custom
// expression leaks a value that Flags-based built-ins mask.
func TestExtensionMasksSensitiveArgs(t *testing.T) {
	cases := map[string]func(momo.Column[string]) momo.Predicate{
		"argfor":  func(c momo.Column[string]) momo.Predicate { return extPrefixOf(c, "pw") },
		"ops.eq":  func(c momo.Column[string]) momo.Predicate { return extUpperOf(c).Eq("pw") },
		"exprfor": func(c momo.Column[string]) momo.Predicate { return extForPred{c: c} },
	}
	for name, mk := range cases {
		for _, sensitive := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/sensitive=%v", name, sensitive), func(t *testing.T) {
				var flags uint16
				if sensitive {
					flags = momo.FlagSensitive
				}
				col := momo.NewColumn[string]("documents", "secret", flags)
				rec := &obstest.Recorder{}
				drv := fakedriver.New(dialect.Postgres)
				db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
				if _, err := momo.Select[suite.Document](db).Where(mk(col)).All(t.Context()); err != nil {
					t.Fatal(err)
				}
				evs, calls := rec.Events(), drv.Calls()
				if len(evs) == 0 || len(calls) == 0 {
					t.Fatal("no statement recorded")
				}
				seen, sent := evs[len(evs)-1].Args, calls[len(calls)-1].Args
				if len(seen) != 1 || len(sent) != 1 {
					t.Fatalf("want one argument, hook saw %v, driver got %v", seen, sent)
				}
				if sent[0] == momo.RedactedArg {
					t.Fatal("the driver received the placeholder")
				}
				if got := seen[0] == momo.RedactedArg; got != sensitive {
					t.Fatalf("hook saw %v, masked = %v, want masked = %v", seen[0], got, sensitive)
				}
			})
		}
	}
}

// extForPred renders `src = (subexpr)` through ExprFor, where the sub
// expression binds one argument.
type extForPred struct {
	momo.Ops[bool]
	c momo.Expr
}

func (p extForPred) AppendSQL(b *momo.Builder) {
	fl := b.RenderSensitive(p.c, momo.FlagsOf(p.c))
	b.WriteString(" = ")
	b.ExprFor(fl, momo.Value("pw"))
}

// Catches: Unsupported not recording ErrUnsupported on the builder.
func TestExtensionUnsupported(t *testing.T) {
	if _, err := extSelect(dialect.Postgres).Columns(extNeedsPGExpr()).SQL(); err != nil {
		t.Fatalf("postgres: %v", err)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		_, err := extSelect(d).Columns(extNeedsPGExpr()).SQL()
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s err = %v, want ErrUnsupported", d, err)
		}
	}
}

// Catches: the vector accessor not reaching the column behind a source.
func TestExtensionVectorSource(t *testing.T) {
	col := momo.NewVectorColumn[vector.Vector]("items", "emb", 3, 0)
	var src momo.VectorSource[vector.Vector] = col
	got := momo.VectorColumnOf(src)
	if got.ColumnName() != "emb" || got.Table() != "items" || got.Dim() != 3 {
		t.Errorf("accessor returned %s.%s dim %d", got.Table(), got.ColumnName(), got.Dim())
	}
}

// shout renders upper(src). Embedding momo.Ops[string] makes it an
// Expression[string] with Eq, In, Asc and the other comparison methods.
type shout struct {
	momo.Ops[string]
	src momo.Expr
}

func newShout(src momo.Expr) shout {
	s := shout{src: src}
	s.Ops = momo.NewOps[string](s)
	return s
}

func (s shout) AppendSQL(b *momo.Builder) {
	b.WriteString("upper(")
	s.src.AppendSQL(b)
	b.WriteString(")")
}

// Flags forwards the wrapped column's flags, so a sensitive column stays
// masked in hook events.
func (s shout) Flags() uint16 { return momo.FlagsOf(s.src) }

// startsWith is a predicate: src LIKE prefix%. RenderSensitive and ArgFor keep
// the bound prefix masked when src is a sensitive column.
type startsWith struct {
	momo.Ops[bool]
	src    momo.Expr
	prefix string
}

func newStartsWith(src momo.Expr, prefix string) startsWith {
	p := startsWith{src: src, prefix: prefix}
	p.Ops = momo.NewOps[bool](p)
	return p
}

func (p startsWith) AppendSQL(b *momo.Builder) {
	fl := b.RenderSensitive(p.src, momo.FlagsOf(p.src))
	b.WriteString(" LIKE ")
	b.ArgFor(fl, p.prefix+"%")
}

// A custom expression embeds momo.Ops[T], writes identifiers through the
// builder and binds values as arguments. The result works in Columns, Where
// and OrderBy like a built-in expression.
func ExampleOps() {
	// SQL() only compiles the statement, so no connection is needed here.
	db := momo.New(pgxdriver.New(nil))
	title := momo.NewColumn[string]("documents", "title", 0)

	c, err := momo.Select[unitTally](db).
		From(momo.TableRef{Name: "documents"}).
		Columns(newShout(title)).
		Where(newStartsWith(title, "ab")).
		SQL()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(c.SQL)
	fmt.Println(c.Args)
	// Output:
	// SELECT upper("documents"."title") FROM "documents" WHERE "documents"."title" LIKE $1
	// [ab%]
}

// PredicateFunc turns a render function into a Predicate without defining a
// type.
func ExamplePredicateFunc() {
	db := momo.New(pgxdriver.New(nil))
	active := momo.PredicateFunc(func(b *momo.Builder) {
		b.Ident("active")
		b.WriteString(" IS TRUE")
	})

	c, err := momo.Select[unitTally](db).
		From(momo.TableRef{Name: "documents"}).
		Columns(momo.NewColumn[string]("documents", "title", 0)).
		Where(active).
		SQL()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(c.SQL)
	// Output:
	// SELECT "documents"."title" FROM "documents" WHERE "active" IS TRUE
}

// extWrapNoFlags wraps a column without implementing Flagged, the way an
// embedder that forgot Flags would.
type extWrapNoFlags struct {
	momo.Ops[string]
	src momo.Expr
}

func extWrapNoFlagsOf(src momo.Expr) extWrapNoFlags {
	w := extWrapNoFlags{src: src}
	w.Ops = momo.NewOps[string](w)
	return w
}

func (w extWrapNoFlags) AppendSQL(b *momo.Builder) { w.src.AppendSQL(b) }

// extMarker writes a column identifier directly, as a distance expression
// does, and marks it sensitive with MarkSensitive.
type extMarker struct {
	momo.Ops[string]
	table, name string
	sensitive   bool
}

func extMarkerOf(table, name string, sensitive bool) extMarker {
	m := extMarker{table: table, name: name, sensitive: sensitive}
	m.Ops = momo.NewOps[string](m)
	return m
}

func (m extMarker) AppendSQL(b *momo.Builder) {
	if m.sensitive {
		b.MarkSensitive()
	}
	b.QualifiedIdent(m.table, m.name)
}

// extCompare is the comparison surface Ops gives a string expression.
type extCompare interface {
	Eq(string) momo.Predicate
	Ne(string) momo.Predicate
	Lt(string) momo.Predicate
	Between(lo, hi string) momo.Predicate
	In(vs ...string) momo.Predicate
	IsDistinctFrom(string) momo.Predicate
	IsNotDistinctFrom(string) momo.Predicate
}

// Catches: a foreign expression over a sensitive column leaking the compared
// value because it does not implement Flagged (the wrapper) or writes the
// identifier itself (the marker), in any comparison form and dialect.
func TestExtensionMasksValuesComparedWithForeignExpressions(t *testing.T) {
	hosts := map[string]func(sensitive bool) extCompare{
		"wrapper without Flagged": func(sensitive bool) extCompare {
			var flags uint16
			if sensitive {
				flags = momo.FlagSensitive
			}
			return extWrapNoFlagsOf(momo.NewColumn[string]("documents", "secret", flags))
		},
		"marker": func(sensitive bool) extCompare { return extMarkerOf("documents", "secret", sensitive) },
	}
	forms := map[string]func(extCompare) momo.Predicate{
		"eq":                   func(c extCompare) momo.Predicate { return c.Eq("pw") },
		"ne":                   func(c extCompare) momo.Predicate { return c.Ne("pw") },
		"lt":                   func(c extCompare) momo.Predicate { return c.Lt("pw") },
		"between":              func(c extCompare) momo.Predicate { return c.Between("a", "b") },
		"in":                   func(c extCompare) momo.Predicate { return c.In("a", "b") },
		"is distinct from":     func(c extCompare) momo.Predicate { return c.IsDistinctFrom("pw") },
		"is not distinct from": func(c extCompare) momo.Predicate { return c.IsNotDistinctFrom("pw") },
	}
	for hostName, mkHost := range hosts {
		for formName, mkForm := range forms {
			for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
				for _, sensitive := range []bool{true, false} {
					t.Run(fmt.Sprintf("%s/%s/%s/sensitive=%v", hostName, formName, d, sensitive), func(t *testing.T) {
						rec := &obstest.Recorder{}
						drv := fakedriver.New(d)
						db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
						pred := mkForm(mkHost(sensitive))
						if _, err := momo.Select[suite.Document](db).Where(pred).All(t.Context()); err != nil {
							t.Fatal(err)
						}
						evs, calls := rec.Events(), drv.Calls()
						if len(evs) == 0 || len(calls) == 0 {
							t.Fatal("no statement recorded")
						}
						seen, sent := evs[len(evs)-1].Args, calls[len(calls)-1].Args
						if len(seen) == 0 || len(seen) != len(sent) {
							t.Fatalf("hook saw %v, driver got %v", seen, sent)
						}
						for i := range seen {
							if sent[i] == momo.RedactedArg {
								t.Fatalf("the driver received the placeholder in %v", sent)
							}
							if got := seen[i] == momo.RedactedArg; got != sensitive {
								t.Fatalf("arg %d: hook saw %v, masked = %v, want masked = %v", i, seen[i], got, sensitive)
							}
						}
					})
				}
			}
		}
	}
}

// Catches: ValidName accepting a name that is empty, too long, starts with a
// digit or carries a character that is not part of an identifier.
func TestValidName(t *testing.T) {
	for _, ok := range []string{"a", "_x", "lower", "col$1", "Straße", strings.Repeat("a", 63)} {
		if err := momo.ValidName(ok); err != nil {
			t.Errorf("ValidName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "1a", "a b", "a-b", `a"b`, "a;b", "$a", strings.Repeat("a", 64)} {
		if err := momo.ValidName(bad); err == nil {
			t.Errorf("ValidName(%q) = nil, want an error", bad)
		}
	}
}
