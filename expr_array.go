// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

// ArrayOf is the array constructor ARRAY[e1, e2, ...] of the element
// expressions, an Expression[[]T]. Each element renders inline: a column, a
// call, or a bound Value. Use it where SQL needs an array built from other
// expressions, such as the path argument of jsonb_set:
//
//	momo.ArrayOf(sqlfn.Concat(momo.Value("at_execution_"), momo.Value(kind)))
//
// An empty list fails the build with ErrValidation, because PostgreSQL cannot
// type ARRAY[]. Arrays are PostgreSQL only; other dialects fail with
// ErrUnsupported. To bind a Go slice as one array parameter use Value.
func ArrayOf[T any](elems ...Expression[T]) Expression[[]T] {
	cp := make([]Expr, len(elems))
	for i, e := range elems {
		cp[i] = e
	}
	return typed[[]T]{arrayOf{elems: slices.Clone(cp)}}
}

type arrayOf struct{ elems []Expr }

func (a arrayOf) AppendSQL(b *Builder) {
	if b.Dialect().Name != dialect.Postgres {
		b.Unsupported("ARRAY[...] constructor (ArrayOf)")
		return
	}
	if len(a.elems) == 0 {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("ArrayOf needs at least one element")})
		return
	}
	b.WriteString("ARRAY[")
	for i, e := range a.elems {
		if i > 0 {
			b.WriteString(", ")
		}
		e.AppendSQL(b)
	}
	b.writeByte(']')
}

// ArraySubquery is ARRAY(subquery): the rows of a one-column subquery
// collected into an array, an Expression[[]T] where T is the Go type of that
// column. The subquery may be a SelectQuery or a set operation such as a
// UNION, so ARRAY(SELECT a FROM x UNION SELECT b FROM y) scans into []T. The
// subquery renders its own parentheses. An empty result is an empty array, not
// NULL. Arrays are PostgreSQL only; other dialects fail with ErrUnsupported.
func ArraySubquery[T any](sub Subquery) Expression[[]T] {
	return typed[[]T]{arraySubquery{sub}}
}

type arraySubquery struct{ q Subquery }

func (a arraySubquery) AppendSQL(b *Builder) {
	if b.Dialect().Name != dialect.Postgres {
		b.Unsupported("ARRAY(subquery) (ArraySubquery)")
		return
	}
	if a.q == nil {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("ArraySubquery needs a subquery")})
		return
	}
	b.WriteString("ARRAY")
	a.q.AppendSQL(b)
}
