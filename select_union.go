// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
)

// UnionQuery runs a UNION or UNION ALL directly, with an outer ORDER BY, LIMIT
// and OFFSET, and scans its rows into T. Build one with SelectUnion.
//
// UnionQuery is an immutable value: every method returns a new query.
type UnionQuery[T any] struct {
	q       Querier
	all     bool
	members []Subquery
	order   []Order
	limit   int64 // -1: none
	offset  int64
	err     error
}

// SelectUnion starts a top-level UNION (all=false) or UNION ALL (all=true) of
// members, whose rows scan into T. T is a model, a projection, a tuple row
// Row1 to Row10 or a plain struct, matched to the result columns as for Query.
// Each member is rendered as Union renders it: in parentheses when it has its
// own ORDER BY, LIMIT, OFFSET or lock, as SELECT * FROM (member) on SQLite.
//
//	rows, err := momo.SelectUnion[hit](db, true,
//		momo.Select[hit](db).From(teachers).Columns(...).OrderBy(name.Asc()).Limit(5),
//		momo.Select[hit](db).From(students).Columns(...).OrderBy(name.Asc()).Limit(5),
//	).OrderBy(momo.Asc(momo.ColumnOf[string]("", "full_name"))).Limit(8).All(ctx)
//
// An outer ORDER BY on a union may name only result columns, not expressions
// over them: build them with ColumnOf and an empty source. A qualified column
// or an expression is an ErrValidation at build time.
//
// Hooks and interceptors see op "select" with an empty table name.
func SelectUnion[T any](q Querier, all bool, members ...Subquery) UnionQuery[T] {
	u := UnionQuery[T]{q: q, all: all, members: slices.Clone(members), limit: -1}
	if len(members) == 0 {
		u.err = newError(ErrValidation, "build", "", errors.New("SelectUnion needs at least one member"))
	}
	for _, m := range members {
		if m == nil {
			u.err = newError(ErrValidation, "build", "", errors.New("SelectUnion: nil member"))
		}
	}
	return u
}

// OrderBy adds outer ORDER BY items, which order the whole union.
func (u UnionQuery[T]) OrderBy(items ...Orderable) UnionQuery[T] {
	o := slices.Clip(u.order)
	for _, it := range items {
		o = append(o, it.order())
	}
	u.order = o
	return u
}

// Limit caps the rows of the whole union.
func (u UnionQuery[T]) Limit(n int64) UnionQuery[T] {
	if n < 0 {
		u.err = newError(ErrValidation, "build", "", fmt.Errorf("Limit must not be negative, got %d", n))
		return u
	}
	u.limit = n
	return u
}

// Offset skips rows of the whole union.
func (u UnionQuery[T]) Offset(n int64) UnionQuery[T] {
	if n < 0 {
		u.err = newError(ErrValidation, "build", "", fmt.Errorf("Offset must not be negative, got %d", n))
		return u
	}
	u.offset = n
	return u
}

func (u UnionQuery[T]) render(b *Builder) {
	if u.err != nil {
		b.SetErr(u.err)
		return
	}
	setOp{u.all, u.members}.renderMembers(b)
	if len(u.order) > 0 {
		b.WriteString(" ORDER BY ")
		for i, o := range u.order {
			if !unqualifiedColumn(o.Expr) {
				b.SetErr(newError(ErrValidation, "build", "", errors.New("an outer ORDER BY on a union may name result columns only, built with ColumnOf and an empty source")))
				return
			}
			if o.Nulls != NullsDefault && !b.d.NullsOrdering {
				// The emulation sorts by an (expr IS NULL) expression, which a
				// union's outer ORDER BY cannot hold; refuse instead of sending
				// SQL the server may reject.
				b.Unsupported("NULLS FIRST/LAST in the outer ORDER BY of a union")
				return
			}
			if i > 0 {
				b.WriteString(", ")
			}
			o.appendSQL(b)
		}
	}
	(&selectCore{limit: u.limit, offset: u.offset}).writeLimitOffset(b)
}

// unqualifiedColumn reports whether e is a column with no table qualifier, the
// only thing a union's outer ORDER BY can name on every dialect.
func unqualifiedColumn(e Expr) bool {
	switch c := e.(type) {
	case colRef:
		return c.table == "" && c.name != ""
	case interface{ ref() colRef }:
		r := c.ref()
		return r.table == "" && r.name != ""
	}
	return false
}

func (u UnionQuery[T]) compiled() (Compiled, error) {
	if u.q == nil {
		return Compiled{}, noQuerier("select", "")
	}
	return compile(u.q.Dialect(), u.render)
}

// SQL compiles the union without executing it.
func (u UnionQuery[T]) SQL() (Compiled, error) { return u.compiled() }

// All runs the union and returns every row. An empty result is a nil slice and
// a nil error.
func (u UnionQuery[T]) All(ctx context.Context) ([]T, error) {
	var out []T
	for v, err := range u.Rows(ctx) {
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Rows runs the union and yields one T per row without buffering the result.
// An error is yielded once, as the final element.
func (u UnionQuery[T]) Rows(ctx context.Context) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		c, err := u.compiled()
		if err != nil {
			yield(zero, err)
			return
		}
		r, sc, err := openCompiled[T](ctx, u.q, "select", "", c)
		if err != nil {
			yield(zero, err)
			return
		}
		defer func() { _ = r.Close() }()
		for {
			if err := ctx.Err(); err != nil {
				yield(zero, newError(ErrCanceled, "select", "", err))
				return
			}
			if !r.Next() {
				break
			}
			var v T
			if err := sc.scan(r, &v); err != nil {
				yield(zero, err)
				return
			}
			if !yield(v, nil) {
				return
			}
		}
		if err := r.Err(); err != nil {
			yield(zero, err)
			return
		}
		if err := r.Close(); err != nil {
			yield(zero, err)
		}
	}
}
