// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

// pgArrayPred is a PostgreSQL array predicate: left op $n, or
// left = ANY($n), with the slice bound as one array parameter. Other
// dialects have no array columns, so the build fails with ErrUnsupported.
type pgArrayPred struct {
	feature string
	left    Expr
	op      string // rendered between left and the bind; empty for ANY
	arg     any
}

func (p pgArrayPred) AppendSQL(b *Builder) {
	if b.d.Name != dialect.Postgres {
		b.unsupported(p.feature)
		return
	}
	p.left.AppendSQL(b)
	if p.op == "" {
		b.WriteString(" = ANY(")
		b.Arg(p.arg)
		b.WriteString(")")
		return
	}
	b.WriteString(p.op)
	b.Arg(p.arg)
}

func (pgArrayPred) resultType() bool { return false }

// boundSlice clones vs and turns nil into an empty slice, which binds as an
// empty array instead of NULL.
func boundSlice[T any](vs []T) []T {
	if vs == nil {
		return []T{}
	}
	return slices.Clone(vs)
}

// ArrayContains is true when the array column contains every one of vals
// (PostgreSQL @>). The slice is bound as one array parameter. With no values
// it is true for every non-NULL array. SQLite fails the build with
// ErrUnsupported.
func ArrayContains[T any](col Expression[[]T], vals ...T) Predicate {
	return pgArrayPred{"array containment (@>)", col, " @> ", boundSlice(vals)}
}

// ArrayContainedBy is true when every element of the array column is among
// vals (PostgreSQL <@). SQLite fails the build with ErrUnsupported.
func ArrayContainedBy[T any](col Expression[[]T], vals ...T) Predicate {
	return pgArrayPred{"array contained-by (<@)", col, " <@ ", boundSlice(vals)}
}

// ArrayOverlaps is true when the array column shares at least one element
// with vals (PostgreSQL &&). With no values it is false. SQLite fails the
// build with ErrUnsupported.
func ArrayOverlaps[T any](col Expression[[]T], vals ...T) Predicate {
	return pgArrayPred{"array overlap (&&)", col, " && ", boundSlice(vals)}
}

// AnyOf is true when the scalar column equals one of vals (col = ANY($n)),
// with the slice bound as a single array parameter. Unlike In it uses one
// bind however many values there are, and an empty list is false. SQLite
// fails the build with ErrUnsupported; use In there.
func AnyOf[T any](col Expression[T], vals ...T) Predicate {
	return pgArrayPred{"= ANY(array)", col, "", boundSlice(vals)}
}
