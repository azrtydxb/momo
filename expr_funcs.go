// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ------------------------------------------------------- function calls

// FuncExpr is a function call whose SQL result type corresponds to T. It is
// an Expression[T] and can be aliased with As.
type FuncExpr[T any] struct {
	exprOps[T]
	name string
	args []any
}

// Func is a typed call of the SQL function name. name must be a plain
// identifier, optionally schema-qualified (lower, pg_catalog.lower); anything
// else fails the build with ErrValidation, so a name cannot smuggle SQL. An
// argument that is an Expr (a column, another call, Value, ...) renders
// inline; any other Go value is bound as a parameter. Momo cannot verify
// that T is what the function returns.
func Func[T any](name string, args ...any) FuncExpr[T] {
	f := FuncExpr[T]{name: name, args: slices.Clone(args)}
	f.exprOps = selfOps[T](f)
	return f
}

// As names the call in a select list: name(args) AS alias.
func (f FuncExpr[T]) As(alias string) Aliased[T] { return As[T](f, alias) }

func (FuncExpr[T]) resultType() T { var z T; return z }

// AppendSQL writes the function call to b.
func (f FuncExpr[T]) AppendSQL(b *Builder) {
	name := f.name
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

// intLit renders an int as a SQL literal, for positions and offsets that must
// stay identical text in SELECT and GROUP BY.
type intLit int

func (n intLit) AppendSQL(b *Builder) { b.WriteString(strconv.Itoa(int(n))) }

// validTypeName is the in-package spelling of ValidTypeName.
func validTypeName(s string) error { return ValidTypeName(s) }

// ValidTypeName checks a "word word(1,2)" shaped SQL type name: words of
// letters, digits and underscores separated by single spaces, with an
// optional (n) or (n,m) suffix, such as bigint, double precision or
// numeric(10,2). It returns a descriptive error otherwise. Code outside the
// package uses it to check a type name before writing it into a CAST.
// ValidTypeName is an extension point; see Ops.
func ValidTypeName(s string) error {
	base, suffix, hasParen := strings.Cut(s, "(")
	if hasParen {
		nums, ok := strings.CutSuffix(suffix, ")")
		if !ok || nums == "" {
			return errors.New("malformed length suffix")
		}
		for _, n := range strings.Split(nums, ",") {
			if n == "" || strings.Trim(n, "0123456789") != "" {
				return errors.New("length suffix must be digits")
			}
		}
	}
	if base == "" {
		return errors.New("name is empty")
	}
	for _, w := range strings.Split(base, " ") {
		if err := validName(w); err != nil || strings.Contains(w, "$") {
			return errors.New("must be words of letters, digits and underscores")
		}
	}
	return nil
}
