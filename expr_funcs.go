// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// ------------------------------------------------------- function calls

// FuncExpr is a function call whose SQL result type corresponds to T. It is
// an Expression[T] and can be aliased with As.
type FuncExpr[T any] struct {
	name string
	args []any
	// alt overrides the function name per dialect.
	alt map[dialect.Name]string
}

// Func is a typed call of the SQL function name. name must be a plain
// identifier, optionally schema-qualified (lower, pg_catalog.lower); anything
// else fails the build with ErrValidation, so a name cannot smuggle SQL. An
// argument that is an Expr (a column, another call, Value, ...) renders
// inline; any other Go value is bound as a parameter. Momo cannot verify
// that T is what the function returns.
func Func[T any](name string, args ...any) FuncExpr[T] {
	return FuncExpr[T]{name: name, args: slices.Clone(args)}
}

// As names the call in a select list: name(args) AS alias.
func (f FuncExpr[T]) As(alias string) Aliased[T] { return As[T](f, alias) }

func (FuncExpr[T]) resultType() T { var z T; return z }

func (f FuncExpr[T]) AppendSQL(b *Builder) {
	name := f.name
	if n, ok := f.alt[b.d.Name]; ok {
		name = n
	}
	for _, part := range strings.Split(name, ".") {
		if err := validName(part); err != nil {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("function name %q: %w", name, err)})
			return
		}
	}
	b.WriteString(name)
	b.writeByte('(')
	for i, a := range f.args {
		if i > 0 {
			b.WriteString(", ")
		}
		if e, ok := a.(Expr); ok {
			e.AppendSQL(b)
		} else {
			b.Arg(a)
		}
	}
	b.writeByte(')')
}

// intLit renders an int as a SQL literal. Substr positions are literals, not
// parameters, so the same expression in SELECT and GROUP BY stays identical
// text and PostgreSQL accepts the grouping.
type intLit int

func (n intLit) AppendSQL(b *Builder) { b.WriteString(strconv.Itoa(int(n))) }

// Substr is substr(e, from, length): length characters of e starting at the
// 1-based position from. The positions render as literals.
func Substr(e Expression[string], from, length int) FuncExpr[string] {
	return Func[string]("substr", e, intLit(from), intLit(length))
}

// SubstrFrom is substr(e, from): everything from the 1-based position from.
func SubstrFrom(e Expression[string], from int) FuncExpr[string] {
	return Func[string]("substr", e, intLit(from))
}

// Lower is lower(e).
func Lower(e Expression[string]) FuncExpr[string] { return Func[string]("lower", e) }

// Upper is upper(e).
func Upper(e Expression[string]) FuncExpr[string] { return Func[string]("upper", e) }

// Length is the length of e in characters (CHAR_LENGTH on MySQL, whose
// LENGTH counts bytes).
func Length(e Expression[string]) FuncExpr[int64] {
	f := Func[int64]("length", e)
	f.alt = map[dialect.Name]string{dialect.MySQL: "char_length"}
	return f
}

// CurrentSchemaExpr is the current schema name.
type CurrentSchemaExpr struct{}

// CurrentSchema is current_schema() on PostgreSQL and DATABASE() on MySQL
// (where a schema is a database). SQLite has no schemas and fails with
// ErrUnsupported.
func CurrentSchema() CurrentSchemaExpr { return CurrentSchemaExpr{} }

// As names the expression in a select list.
func (c CurrentSchemaExpr) As(alias string) Aliased[string] { return As[string](c, alias) }

func (CurrentSchemaExpr) resultType() string { return "" }

func (CurrentSchemaExpr) AppendSQL(b *Builder) {
	switch b.d.Name {
	case dialect.Postgres:
		b.WriteString("current_schema()")
	case dialect.MySQL:
		b.WriteString("DATABASE()")
	default:
		b.unsupported("current_schema()")
	}
}

// ------------------------------------------------------------------ CASE

type caseWhen struct {
	cond Predicate
	then Expr
}

// CaseExpr is CASE WHEN ... THEN ... [ELSE ...] END whose branches all have
// the Go type T. Build it with Case. Without an Else the result is NULL when
// no branch matches. It is immutable: every method returns a new value.
type CaseExpr[T any] struct {
	whens []caseWhen
	els   Expr
}

// Case starts a CASE expression whose branches produce T, for example
// Case[string]().When(host.Excluded().Ne(""), "x").ElseExpr(host).
func Case[T any]() CaseExpr[T] { return CaseExpr[T]{} }

// When adds a branch yielding the bound value v.
func (c CaseExpr[T]) When(cond Predicate, v T) CaseExpr[T] {
	return c.WhenExpr(cond, Value(v))
}

// WhenExpr adds a branch yielding an expression.
func (c CaseExpr[T]) WhenExpr(cond Predicate, e Expression[T]) CaseExpr[T] {
	c.whens = append(slices.Clip(c.whens), caseWhen{cond, e})
	return c
}

// Else sets the bound value used when no branch matches.
func (c CaseExpr[T]) Else(v T) CaseExpr[T] { return c.ElseExpr(Value(v)) }

// ElseExpr sets the expression used when no branch matches.
func (c CaseExpr[T]) ElseExpr(e Expression[T]) CaseExpr[T] {
	c.els = e
	return c
}

// As names the expression in a select list.
func (c CaseExpr[T]) As(alias string) Aliased[T] { return As[T](c, alias) }

func (CaseExpr[T]) resultType() T { var z T; return z }

func (c CaseExpr[T]) AppendSQL(b *Builder) {
	if len(c.whens) == 0 {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("CASE needs at least one When")})
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
	l  Expr
	op string
	r  Expr
}

// As names the expression in a select list.
func (a ArithExpr[T]) As(alias string) Aliased[T] { return As[T](a, alias) }

func (ArithExpr[T]) resultType() T { var z T; return z }

func (a ArithExpr[T]) AppendSQL(b *Builder) {
	b.writeByte('(')
	a.l.AppendSQL(b)
	b.WriteString(a.op)
	a.r.AppendSQL(b)
	b.writeByte(')')
}

// Add is e + v, for example in a SET: quota.SetExpr(momo.Add(quota, 5)).
// Go methods cannot constrain a type parameter, so arithmetic is a function
// that accepts numeric T only; a string column does not compile.
func Add[T Numeric](e Expression[T], v T) ArithExpr[T] { return ArithExpr[T]{e, " + ", argExpr{v}} }

// AddExpr is e + other, for example answers + EXCLUDED.answers:
// answers.SetExpr(momo.AddExpr(answers, answers.Excluded())).
func AddExpr[T Numeric](e, other Expression[T]) ArithExpr[T] { return ArithExpr[T]{e, " + ", other} }

// Sub is e - v.
func Sub[T Numeric](e Expression[T], v T) ArithExpr[T] { return ArithExpr[T]{e, " - ", argExpr{v}} }

// SubExpr is e - other.
func SubExpr[T Numeric](e, other Expression[T]) ArithExpr[T] { return ArithExpr[T]{e, " - ", other} }

// Mul is e * v.
func Mul[T Numeric](e Expression[T], v T) ArithExpr[T] { return ArithExpr[T]{e, " * ", argExpr{v}} }

// MulExpr is e * other.
func MulExpr[T Numeric](e, other Expression[T]) ArithExpr[T] { return ArithExpr[T]{e, " * ", other} }
