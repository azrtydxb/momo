// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
)

// This file is the worked example of the frozen extension API: a custom
// expression built outside the packages of momo, with nothing but Expr,
// Expression, Ops, NewOps, Typed, Flagged and the Builder methods Ident, Arg,
// Dialect, SetErr and WriteString. It deliberately imports no internal
// package for the expression itself; internal/suite only supplies the demo
// column. CI compiles and runs it, so a change that breaks the frozen surface
// fails here first.

// initcapExpr is initcap(src), a PostgreSQL string function. It embeds
// momo.Ops[string], so it has Eq, In, Asc and the other comparison and ordering
// methods without writing them.
type initcapExpr struct {
	momo.Ops[string]
	src momo.Expr
}

// initcap builds the expression. NewOps binds the embedded methods to the
// finished host, so it runs after the struct is filled in.
func initcap(src momo.Expression[string]) initcapExpr {
	e := initcapExpr{src: src}
	e.Ops = momo.NewOps[string](e)
	return e
}

// AppendSQL writes the SQL: fixed text with WriteString, the operand through its
// own AppendSQL, and a refusal through SetErr on a dialect without the function.
func (e initcapExpr) AppendSQL(b *momo.Builder) {
	if b.Dialect().Name != dialect.Postgres {
		b.SetErr(&momo.Error{
			Kind: momo.ErrUnsupported,
			Op:   "build",
			Err:  fmt.Errorf("initcap is not supported by %s", b.Dialect().Name),
		})
		return
	}
	b.WriteString("initcap(")
	e.src.AppendSQL(b)
	b.WriteString(")")
}

// Flags forwards the wrapped column's flags, so a value compared with
// initcap(secret) is masked in hook events when the column is sensitive.
func (e initcapExpr) Flags() momo.ColumnFlags {
	if f, ok := e.src.(momo.Flagged); ok {
		return f.Flags()
	}
	return 0
}

// prefixPredicate is a predicate. It has no comparison methods of its own, so
// momo.Typed gives the plain Expr its result type: a predicate is an
// Expression[bool]. The value goes through Arg, never into the SQL text.
type prefixPredicate struct {
	src    momo.Expr
	prefix string
}

func (s prefixPredicate) AppendSQL(b *momo.Builder) {
	b.WriteString("starts_with(")
	s.src.AppendSQL(b)
	b.WriteString(", ")
	b.Arg(s.prefix)
	b.WriteString(")")
}

func hasPrefix(src momo.Expression[string], prefix string) momo.Predicate {
	return momo.Typed[bool](prefixPredicate{src: src, prefix: prefix})
}

// A custom expression composes with the built-in query builder: initcap in the
// select list and in a comparison through the embedded Ops, and the Typed
// predicate in WHERE. On SQLite the build fails with momo.ErrUnsupported.
func ExampleNewOps() {
	title := suite.DocTitle
	pg := momo.NewOffline(dialect.Postgres)

	q := momo.Select[unitTally](pg).From(momo.TableRef{Name: "documents"}).
		Columns(initcap(title)).
		Where(initcap(title).Eq("Hello World"))
	c, err := q.SQL()
	fmt.Println(c.SQL, c.Args, err)

	p := momo.Select[unitTally](pg).From(momo.TableRef{Name: "documents"}).
		Columns(title).
		Where(hasPrefix(title, "Hello"))
	c, err = p.SQL()
	fmt.Println(c.SQL, c.Args, err)

	lite := momo.NewOffline(dialect.SQLite)
	_, err = momo.Select[unitTally](lite).From(momo.TableRef{Name: "documents"}).
		Columns(initcap(title)).SQL()
	fmt.Println(errors.Is(err, momo.ErrUnsupported))
	// Output:
	// SELECT initcap("documents"."title") FROM "documents" WHERE initcap("documents"."title") = $1 [Hello World] <nil>
	// SELECT "documents"."title" FROM "documents" WHERE starts_with("documents"."title", $1) [Hello] <nil>
	// true
}
