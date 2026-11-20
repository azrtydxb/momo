// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
)

// This file is the worked example of the frozen extension API: custom
// expressions built outside the packages of momo, with nothing but Expr,
// Expression, Ops, NewOps, Typed, Flagged and the Builder methods Ident, Arg,
// Dialect, SetErr and WriteString. It deliberately imports no internal
// package for the expressions themselves; internal/suite only supplies the demo
// column. CI compiles and runs it, so a change that breaks the frozen surface
// fails here first.
//
// A caller's value reaches the database in one of two ways here. Comparing
// through an Ops method (Eq, Gt, In and the rest) binds it with the flags of the
// host, so a value compared with a sensitive column is masked in hook events.
// Builder.Arg binds a value without that mask, so write it only for a value that
// is not compared with a column the caller may have marked sensitive.

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

// Flags forwards the wrapped column's flags, so Eq on initcap(secret) masks the
// compared value in hook events when the column is sensitive.
func (e initcapExpr) Flags() momo.ColumnFlags {
	if f, ok := e.src.(momo.Flagged); ok {
		return f.Flags()
	}
	return 0
}

// lengthExpr is char_length(src), an integer expression. A comparison such as
// Gt goes through Ops, which is how a value stays masked beside a sensitive
// column; the host forwards the column's flags like initcapExpr does.
type lengthExpr struct {
	momo.Ops[int64]
	src momo.Expr
}

func charLength(src momo.Expression[string]) lengthExpr {
	e := lengthExpr{src: src}
	e.Ops = momo.NewOps[int64](e)
	return e
}

func (e lengthExpr) AppendSQL(b *momo.Builder) {
	b.WriteString("char_length(")
	e.src.AppendSQL(b)
	b.WriteString(")")
}

func (e lengthExpr) Flags() momo.ColumnFlags {
	if f, ok := e.src.(momo.Flagged); ok {
		return f.Flags()
	}
	return 0
}

// blank is a predicate with no caller value in it, so it binds nothing. It has no
// comparison methods of its own, and momo.Typed gives the plain Expr its result
// type: a predicate is an Expression[bool].
type blank struct{ src momo.Expr }

func (s blank) AppendSQL(b *momo.Builder) {
	b.WriteString("(")
	s.src.AppendSQL(b)
	b.WriteString(" IS NULL OR ")
	s.src.AppendSQL(b)
	b.WriteString(" = '')")
}

func isBlank(src momo.Expression[string]) momo.Predicate {
	return momo.Typed[bool](blank{src: src})
}

// A custom expression composes with the built-in query builder: initcap in the
// select list and in a comparison through the embedded Ops, an integer
// expression compared with Gt, and a Typed predicate in WHERE. On SQLite the
// build fails with momo.ErrUnsupported.
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
		Where(charLength(title).Gt(5))
	c, err = p.SQL()
	fmt.Println(c.SQL, c.Args, err)

	r := momo.Select[unitTally](pg).From(momo.TableRef{Name: "documents"}).
		Columns(title).
		Where(isBlank(title))
	c, err = r.SQL()
	fmt.Println(c.SQL, c.Args, err)

	lite := momo.NewOffline(dialect.SQLite)
	_, err = momo.Select[unitTally](lite).From(momo.TableRef{Name: "documents"}).
		Columns(initcap(title)).SQL()
	fmt.Println(errors.Is(err, momo.ErrUnsupported))
	// Output:
	// SELECT initcap("documents"."title") FROM "documents" WHERE initcap("documents"."title") = $1 [Hello World] <nil>
	// SELECT "documents"."title" FROM "documents" WHERE char_length("documents"."title") > $1 [5] <nil>
	// SELECT "documents"."title" FROM "documents" WHERE ("documents"."title" IS NULL OR "documents"."title" = '') [] <nil>
	// true
}
