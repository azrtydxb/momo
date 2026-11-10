// SPDX-License-Identifier: Apache-2.0

// Package arrayop holds the PostgreSQL array predicates: containment,
// contained-by, overlap and the single-bind "= ANY" membership test. Each
// binds its values as one array parameter, so the bind count does not grow
// with the list. Other dialects have no array columns, so the build fails with
// momo.ErrUnsupported there; use momo's In on SQLite and MySQL.
//
// The package builds on the exported extension API of the root package
// (PredicateFunc, Builder.ArgFor, Builder.RenderSensitive, Builder.Unsupported
// and FlagsOf) and is the reference for writing a feature package outside the
// root.
package arrayop

import (
	"slices"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// build returns the predicate left op $n, or left = ANY($n) when op is empty,
// with vals bound as one array parameter. A predicate only renders, so it is a
// momo.PredicateFunc and needs no comparison methods.
//
// The left operand renders first so that a sensitive column marks the flags
// that the bind then carries: the values compared with it stay masked in hook
// events.
func build(feature string, left momo.Expr, op string, vals any) momo.Predicate {
	return momo.PredicateFunc(func(b *momo.Builder) {
		if b.Dialect().Name != dialect.Postgres {
			b.Unsupported(feature)
			return
		}
		fl := b.RenderSensitive(left, momo.FlagsOf(left))
		cast := castFor(vals)
		if op == "" {
			b.WriteString(" = ANY(")
			b.ArgFor(fl, vals)
			b.WriteString(cast + ")")
			return
		}
		b.WriteString(op)
		b.ArgFor(fl, vals)
		b.WriteString(cast)
	})
}

// castFor is the explicit array cast the bind carries. A uuid array is cast
// ($1::uuid[]) so the server types the parameter even where it cannot infer it
// from the other operand; the other element types infer fine and stay bare.
func castFor(vals any) string {
	if _, ok := vals.([]types.UUID); ok {
		return "::uuid[]"
	}
	return ""
}

// boundSlice clones vs and turns nil into an empty slice, which binds as an
// empty array instead of NULL.
func boundSlice[T any](vs []T) []T {
	if vs == nil {
		return []T{}
	}
	return slices.Clone(vs)
}

// Contains is true when the array column contains every one of vals
// (PostgreSQL @>). The slice is bound as one array parameter. With no values
// it is true for every non-NULL array. Other dialects fail the build
// with ErrUnsupported.
func Contains[T any](col momo.Expression[[]T], vals ...T) momo.Predicate {
	return build("array containment (@>)", col, " @> ", boundSlice(vals))
}

// ContainedBy is true when every element of the array column is among
// vals (PostgreSQL <@). Other dialects fail the build with ErrUnsupported.
func ContainedBy[T any](col momo.Expression[[]T], vals ...T) momo.Predicate {
	return build("array contained-by (<@)", col, " <@ ", boundSlice(vals))
}

// Overlaps is true when the array column shares at least one element
// with vals (PostgreSQL &&). With no values it is false. Other dialects
// fail the build with ErrUnsupported.
func Overlaps[T any](col momo.Expression[[]T], vals ...T) momo.Predicate {
	return build("array overlap (&&)", col, " && ", boundSlice(vals))
}

// AnyOf is true when the scalar column equals one of vals (col = ANY($n)),
// with the slice bound as a single array parameter. Unlike In it uses one
// bind however many values there are, and an empty list is false. Other
// dialects fail the build with ErrUnsupported; use In there.
func AnyOf[T any](col momo.Expression[T], vals ...T) momo.Predicate {
	return build("= ANY(array)", col, "", boundSlice(vals))
}
