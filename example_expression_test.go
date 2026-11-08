// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testmodels/example"
)

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

	c, err := momo.Select[example.Document](db).
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

	c, err := momo.Select[example.Document](db).
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
