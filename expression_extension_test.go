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
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/vector"
)

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

func extSelect(d dialect.Name) momo.SelectQuery[suite.Document] {
	return momo.Select[suite.Document](momo.New(fakedriver.New(d)))
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

	in, err := extSelect(dialect.Postgres).Where(extUpperOf(suite.DocTitle).In("A", "B")).SQL()
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
	got, err := extSelect(dialect.Postgres).Columns(typed).Where(fn, typed.Eq("Q")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.SQL, `1 = $1`) {
		t.Errorf("SQL %q lacks the func predicate", got.SQL)
	}
	if want := []any{1, "Q"}; !slices.Equal(got.Args, want) {
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
