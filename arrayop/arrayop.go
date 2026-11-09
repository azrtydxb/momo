// SPDX-License-Identifier: Apache-2.0

// Package arrayop holds the PostgreSQL array predicates: containment,
// contained-by, overlap and the single-bind "= ANY" membership test. Each
// binds its values as one array parameter, so the bind count does not grow
// with the list. Other dialects have no array columns, so the build fails with
// momo.ErrUnsupported there; use momo's In on SQLite and MySQL.
//
// The package builds on the exported extension API of the root package (Ops,
// NewOps, Builder.ArgFor, Builder.RenderSensitive, Builder.Unsupported and
// FlagsOf) and is the reference for writing a feature package outside the root.
package arrayop

import (
	"slices"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// pred is a PostgreSQL array predicate: left op $n, or left = ANY($n), with
// the slice bound as one array parameter. Embedding momo.Ops[bool] makes it a
// momo.Predicate.
type pred struct {
	momo.Ops[bool]
	feature string
	left    momo.Expr
	op      string // rendered between left and the bind; empty for ANY
	arg     any
}

func newPred(feature string, left momo.Expr, op string, arg any) pred {
	p := pred{feature: feature, left: left, op: op, arg: arg}
	p.Ops = momo.NewOps[bool](p)
	return p
}

// AppendSQL renders the predicate, or records ErrUnsupported off PostgreSQL.
func (p pred) AppendSQL(b *momo.Builder) {
	if b.Dialect().Name != dialect.Postgres {
		b.Unsupported(p.feature)
		return
	}
	fl := b.RenderSensitive(p.left, momo.FlagsOf(p.left))
	if p.op == "" {
		b.WriteString(" = ANY(")
		b.ArgFor(fl, p.arg)
		b.WriteString(")")
		return
	}
	b.WriteString(p.op)
	b.ArgFor(fl, p.arg)
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
// it is true for every non-NULL array. SQLite fails the build with
// ErrUnsupported.
func Contains[T any](col momo.Expression[[]T], vals ...T) momo.Predicate {
	return newPred("array containment (@>)", col, " @> ", boundSlice(vals))
}

// ContainedBy is true when every element of the array column is among
// vals (PostgreSQL <@). SQLite fails the build with ErrUnsupported.
func ContainedBy[T any](col momo.Expression[[]T], vals ...T) momo.Predicate {
	return newPred("array contained-by (<@)", col, " <@ ", boundSlice(vals))
}

// Overlaps is true when the array column shares at least one element
// with vals (PostgreSQL &&). With no values it is false. SQLite fails the
// build with ErrUnsupported.
func Overlaps[T any](col momo.Expression[[]T], vals ...T) momo.Predicate {
	return newPred("array overlap (&&)", col, " && ", boundSlice(vals))
}

// AnyOf is true when the scalar column equals one of vals (col = ANY($n)),
// with the slice bound as a single array parameter. Unlike In it uses one
// bind however many values there are, and an empty list is false. SQLite
// fails the build with ErrUnsupported; use In there.
func AnyOf[T any](col momo.Expression[T], vals ...T) momo.Predicate {
	return newPred("= ANY(array)", col, "", boundSlice(vals))
}
