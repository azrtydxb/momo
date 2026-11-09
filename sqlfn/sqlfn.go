// SPDX-License-Identifier: Apache-2.0

// Package sqlfn holds the typed scalar SQL functions and operators: Substr,
// Lower, Upper, Length, CurrentSchema, Case, the arithmetic helpers Add, Sub
// and Mul, CoalesceExpr, Cast and Concat. Each is a value expression with the
// comparison and ordering methods of a column (Eq, Lt, In, Asc, ...), and
// each can be named in a select list with As.
//
// The package builds on the exported extension API of the root package
// (momo.Func, momo.Ops, momo.NewOps, Builder.Unsupported, momo.ValidTypeName)
// and uses no unexported name. Functions the package does not wrap are called
// with momo.Func, which stays in the root package.
package sqlfn

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// intLit renders an int as a SQL literal. Substr positions are literals, not
// parameters, so the same expression in SELECT and GROUP BY stays identical
// text and PostgreSQL accepts the grouping.
type intLit int

func (n intLit) AppendSQL(b *momo.Builder) { b.WriteString(strconv.Itoa(int(n))) }

// Substr is substr(e, from, length): length characters of e starting at the
// 1-based position from. The positions render as literals.
func Substr(e momo.Expression[string], from, length int) momo.FuncExpr[string] {
	return momo.Func[string]("substr", e, intLit(from), intLit(length))
}

// SubstrFrom is substr(e, from): everything from the 1-based position from.
func SubstrFrom(e momo.Expression[string], from int) momo.FuncExpr[string] {
	return momo.Func[string]("substr", e, intLit(from))
}

// Lower is lower(e).
func Lower(e momo.Expression[string]) momo.FuncExpr[string] { return momo.Func[string]("lower", e) }

// Upper is upper(e).
func Upper(e momo.Expression[string]) momo.FuncExpr[string] { return momo.Func[string]("upper", e) }

// Length is the length of e in characters (CHAR_LENGTH on MySQL, whose
// LENGTH counts bytes).
func Length(e momo.Expression[string]) LengthExpr {
	l := LengthExpr{e: e}
	l.Ops = momo.NewOps[int64](l)
	return l
}

// LengthExpr is the character length of a string expression.
type LengthExpr struct {
	momo.Ops[int64]
	e momo.Expression[string]
}

// As names the expression in a select list.
func (l LengthExpr) As(alias string) momo.Aliased[int64] { return momo.As[int64](l, alias) }

// AppendSQL writes the length call to b.
func (l LengthExpr) AppendSQL(b *momo.Builder) {
	switch b.Dialect().Name {
	case dialect.MySQL: // LENGTH counts bytes there
		b.WriteString("char_length(")
	default:
		b.WriteString("length(")
	}
	l.e.AppendSQL(b)
	b.WriteString(")")
}

// CurrentSchemaExpr is the current schema name.
type CurrentSchemaExpr struct {
	momo.Ops[string]
}

// CurrentSchema is current_schema() on PostgreSQL and DATABASE() on MySQL
// (where a schema is a database). SQLite has no schemas and fails with
// ErrUnsupported.
func CurrentSchema() CurrentSchemaExpr {
	c := CurrentSchemaExpr{}
	c.Ops = momo.NewOps[string](c)
	return c
}

// As names the expression in a select list.
func (c CurrentSchemaExpr) As(alias string) momo.Aliased[string] {
	return momo.As[string](c, alias)
}

// AppendSQL writes the current_schema call to b.
func (CurrentSchemaExpr) AppendSQL(b *momo.Builder) {
	switch b.Dialect().Name {
	case dialect.Postgres:
		b.WriteString("current_schema()")
	case dialect.MySQL:
		b.WriteString("DATABASE()")
	default:
		b.Unsupported("current_schema()")
	}
}

// ------------------------------------------------------------------ CASE

type caseWhen struct {
	cond momo.Predicate
	then momo.Expr
}

// CaseExpr is CASE WHEN ... THEN ... [ELSE ...] END whose branches all have
// the Go type T. Build it with Case. Without an Else the result is NULL when
// no branch matches. It is immutable: every method returns a new value. A
// CaseExpr built without Case fails the build with ErrValidation.
type CaseExpr[T any] struct {
	momo.Ops[T]
	whens []caseWhen
	els   momo.Expr
}

// Case starts a CASE expression whose branches produce T, for example
// Case[string]().When(host.Excluded().Ne(""), "x").ElseExpr(host).
func Case[T any]() CaseExpr[T] { return CaseExpr[T]{}.bind() }

// bind points the embedded methods at the current value.
func (c CaseExpr[T]) bind() CaseExpr[T] {
	c.Ops = momo.Ops[T]{}
	c.Ops = momo.NewOps[T](c)
	return c
}

// When adds a branch yielding the bound value v.
func (c CaseExpr[T]) When(cond momo.Predicate, v T) CaseExpr[T] {
	return c.WhenExpr(cond, momo.Value(v))
}

// WhenExpr adds a branch yielding an expression.
func (c CaseExpr[T]) WhenExpr(cond momo.Predicate, e momo.Expression[T]) CaseExpr[T] {
	c.whens = append(slices.Clip(c.whens), caseWhen{cond, e})
	return c.bind()
}

// Else sets the bound value used when no branch matches.
func (c CaseExpr[T]) Else(v T) CaseExpr[T] { return c.ElseExpr(momo.Value(v)) }

// ElseExpr sets the expression used when no branch matches.
func (c CaseExpr[T]) ElseExpr(e momo.Expression[T]) CaseExpr[T] {
	c.els = e
	return c.bind()
}

// As names the expression in a select list.
func (c CaseExpr[T]) As(alias string) momo.Aliased[T] { return momo.As[T](c, alias) }

// AppendSQL writes the CASE expression to b.
func (c CaseExpr[T]) AppendSQL(b *momo.Builder) {
	if len(c.whens) == 0 {
		b.SetErr(&momo.Error{Kind: momo.ErrValidation, Op: "build", Err: errors.New("CASE needs at least one When")})
		return
	}
	b.WriteString("CASE")
	for _, w := range c.whens {
		b.WriteString(" WHEN ")
		w.cond.AppendSQL(b)
		b.WriteString(" THEN ")
		w.then.AppendSQL(b)
	}
	if c.els != nil {
		b.WriteString(" ELSE ")
		c.els.AppendSQL(b)
	}
	b.WriteString(" END")
}

// ------------------------------------------------------------ arithmetic

// Numeric is the constraint of the arithmetic helpers.
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// ArithExpr is a parenthesised binary arithmetic expression over T.
type ArithExpr[T Numeric] struct {
	momo.Ops[T]
	l  momo.Expr
	op string
	r  momo.Expr
}

func arith[T Numeric](l momo.Expr, op string, r momo.Expr) ArithExpr[T] {
	a := ArithExpr[T]{l: l, op: op, r: r}
	a.Ops = momo.NewOps[T](a)
	return a
}

// As names the expression in a select list.
func (a ArithExpr[T]) As(alias string) momo.Aliased[T] { return momo.As[T](a, alias) }

// AppendSQL writes the arithmetic expression to b.
func (a ArithExpr[T]) AppendSQL(b *momo.Builder) {
	b.WriteString("(")
	a.l.AppendSQL(b)
	b.WriteString(a.op)
	a.r.AppendSQL(b)
	b.WriteString(")")
}

// Add is e + v, for example in a SET: quota.SetExpr(sqlfn.Add(quota, 5)).
// Go methods cannot constrain a type parameter, so arithmetic is a function
// that accepts numeric T only; a string column does not compile.
func Add[T Numeric](e momo.Expression[T], v T) ArithExpr[T] {
	return arith[T](e, " + ", momo.Value(v))
}

// AddExpr is e + other, for example answers + EXCLUDED.answers:
// answers.SetExpr(sqlfn.AddExpr(answers, answers.Excluded())).
func AddExpr[T Numeric](e, other momo.Expression[T]) ArithExpr[T] {
	return arith[T](e, " + ", other)
}

// Sub is e - v.
func Sub[T Numeric](e momo.Expression[T], v T) ArithExpr[T] {
	return arith[T](e, " - ", momo.Value(v))
}

// SubExpr is e - other.
func SubExpr[T Numeric](e, other momo.Expression[T]) ArithExpr[T] {
	return arith[T](e, " - ", other)
}

// Mul is e * v.
func Mul[T Numeric](e momo.Expression[T], v T) ArithExpr[T] {
	return arith[T](e, " * ", momo.Value(v))
}

// MulExpr is e * other.
func MulExpr[T Numeric](e, other momo.Expression[T]) ArithExpr[T] {
	return arith[T](e, " * ", other)
}

// ------------------------------------------- coalesce, cast, concatenation

// CoalesceExpr is COALESCE(a, b, more...): the first non-NULL expression.
// Unlike momo.Coalesce, whose fallback is a bound value, every argument is an
// expression of the same type T, for example CoalesceExpr(site, licenseID).
func CoalesceExpr[T any](a, b momo.Expression[T], more ...momo.Expression[T]) momo.FuncExpr[T] {
	args := make([]any, 0, 2+len(more))
	args = append(args, a, b)
	for _, m := range more {
		args = append(args, m)
	}
	return momo.Func[T]("coalesce", args...)
}

// CastExpr is CAST(expr AS type) with a caller-chosen result type T.
type CastExpr[T any] struct {
	momo.Ops[T]
	e       momo.Expr
	sqlType string
}

// Cast is CAST(e AS sqlType). T is the Go type of the result; Momo cannot
// verify that it matches sqlType. sqlType is not portable (MySQL casts to
// SIGNED, not bigint) and is validated by momo.ValidTypeName: words of
// letters, digits and underscores separated by single spaces, with an
// optional (n) or (n,m) suffix, such as bigint, double precision or
// numeric(10,2). Anything else fails the build with ErrValidation.
func Cast[T any](e momo.Expr, sqlType string) CastExpr[T] {
	c := CastExpr[T]{e: e, sqlType: sqlType}
	c.Ops = momo.NewOps[T](c)
	return c
}

// As names the expression in a select list.
func (c CastExpr[T]) As(alias string) momo.Aliased[T] { return momo.As[T](c, alias) }

// AppendSQL writes the CAST expression to b.
func (c CastExpr[T]) AppendSQL(b *momo.Builder) {
	if err := momo.ValidTypeName(c.sqlType); err != nil {
		b.SetErr(&momo.Error{Kind: momo.ErrValidation, Op: "build", Err: fmt.Errorf("cast type %q: %w", c.sqlType, err)})
		return
	}
	b.WriteString("CAST(")
	c.e.AppendSQL(b)
	b.WriteString(" AS ")
	b.WriteString(c.sqlType)
	b.WriteString(")")
}

// ConcatExpr is string concatenation.
type ConcatExpr struct {
	momo.Ops[string]
	parts []momo.Expression[string]
}

// Concat joins two or more string expressions: (a || b) on PostgreSQL and
// SQLite, CONCAT(a, b) on MySQL. A NULL part makes the result NULL; wrap
// nullable parts in CoalesceExpr.
func Concat(a, b momo.Expression[string], more ...momo.Expression[string]) ConcatExpr {
	c := ConcatExpr{parts: append([]momo.Expression[string]{a, b}, more...)}
	c.Ops = momo.NewOps[string](c)
	return c
}

// As names the expression in a select list.
func (c ConcatExpr) As(alias string) momo.Aliased[string] { return momo.As[string](c, alias) }

// AppendSQL writes the concatenation to b.
func (c ConcatExpr) AppendSQL(b *momo.Builder) {
	open, sep, closing := "(", " || ", ")"
	if b.Dialect().Name == dialect.MySQL {
		open, sep = "CONCAT(", ", "
	}
	b.WriteString(open)
	for i, p := range c.parts {
		if i > 0 {
			b.WriteString(sep)
		}
		p.AppendSQL(b)
	}
	b.WriteString(closing)
}
