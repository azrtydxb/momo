// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// This file holds the forwarders for the extension names that left the public
// API when the extension surface was frozen (see Ops). Each keeps its old
// signature, including uint16 flags, for one release and is removed in the
// next. The feature packages of this module no longer use them; they build on
// internal/exprkit.

// NewBuilder returns a builder for the dialect.
//
// Deprecated: a custom expression receives its Builder from the statement
// that renders it, and a test renders through a query's SQL method on a
// NewOffline handle. NewBuilder is removed in the next release.
func NewBuilder(d *dialect.Dialect) *Builder { return newBuilder(d) }

// WriteByte appends a trusted SQL byte. It always returns nil.
//
// Deprecated: use Builder.WriteString. WriteByte is removed in the next
// release.
func (b *Builder) WriteByte(c byte) error { b.writeByte(c); return nil }

// ArgFor binds v like Arg and records it as sensitive when flags carries
// FlagSensitive.
//
// Deprecated: compare through the Ops methods of an Expression, which mask a
// value compared with a sensitive column. ArgFor is removed in the next
// release.
func (b *Builder) ArgFor(flags uint16, v any) { b.argFor(ColumnFlags(flags), v) }

// RenderSensitive renders e and returns flags with FlagSensitive added when e
// read a sensitive column.
//
// Deprecated: use the Ops methods of an Expression. RenderSensitive is removed
// in the next release.
func (b *Builder) RenderSensitive(e Expr, flags uint16) uint16 {
	return uint16(b.renderSens(e, ColumnFlags(flags)))
}

// ExprFor renders e with every argument it binds recorded as sensitive when
// flags carries FlagSensitive.
//
// Deprecated: use the Ops methods of an Expression. ExprFor is removed in the
// next release.
func (b *Builder) ExprFor(flags uint16, e Expr) { b.exprFor(ColumnFlags(flags), e) }

// MarkSensitive records that the expression being rendered read a sensitive
// column.
//
// Deprecated: render the column through its own AppendSQL, which records it.
// MarkSensitive is removed in the next release.
func (b *Builder) MarkSensitive() { b.markSensitive() }

// AppendFilter writes " FILTER (WHERE p)" for an aggregate call that was just
// rendered.
//
// Deprecated: AppendFilter is removed in the next release; use agg.Call.Filter
// or FilteredAgg.
func (b *Builder) AppendFilter(p Predicate) { appendFilter(b, p) }

// Unsupported records an ErrUnsupported build error for feature, naming the
// dialect being rendered.
//
// Deprecated: call Builder.SetErr with an *Error of kind ErrUnsupported.
// Unsupported is removed in the next release.
func (b *Builder) Unsupported(feature string) { b.unsupported(feature) }

// FlagsOf returns the column flags of e, or 0 when e does not implement
// Flagged.
//
// Deprecated: assert Flagged on the expression. FlagsOf is removed in the next
// release. A type that implemented the old Flags() uint16 no longer satisfies
// Flagged, and FlagsOf then returns 0 for it.
func FlagsOf(e Expr) uint16 { return uint16(flagsOf(e)) }

// IsBound reports whether e is a Go value bound as a parameter by Value,
// possibly wrapped by Typed.
//
// Deprecated: IsBound is removed in the next release.
func IsBound(e Expr) bool { return isBound(e) }

// PredicateFunc adapts a render function to a Predicate.
//
// Deprecated: write a type that embeds Ops[bool], or wrap an Expr with Typed.
// PredicateFunc is removed in the next release.
type PredicateFunc func(b *Builder)

// AppendSQL calls f.
func (f PredicateFunc) AppendSQL(b *Builder) { f(b) }

func (PredicateFunc) resultType() bool { return false }

// ValidName checks an identifier-like name: non-empty, at most 63 bytes, a
// letter or underscore first, then letters, digits, underscores or dollar
// signs.
//
// Deprecated: ValidName is removed in the next release.
func ValidName(s string) error { return validName(s) }

// ValidTypeName checks a "word word(1,2)" shaped SQL type name, such as
// bigint, double precision or numeric(10,2).
//
// Deprecated: ValidTypeName is removed in the next release.
func ValidTypeName(s string) error { return validTypeName(s) }

// WindowOf turns the aggregate call into a window function: call OVER (...).
//
// Deprecated: WindowOf is removed in the next release; agg.Call.Over builds
// the window function of an aggregate.
func WindowOf[T any](call Expr, w WindowSpec) WindowExpr[T] {
	return newWindowExpr[T](call, "", false, false, w)
}

// VectorColumnOf returns the VectorColumn behind any vector column descriptor
// that satisfies VectorSource.
//
// Deprecated: VectorColumnOf is removed in the next release; the descriptors
// themselves have Table, ColumnName, Dim and Flags.
func VectorColumnOf[V VectorValue](s VectorSource[V]) VectorColumn[V] { return s.vectorColumn() }

// MetaTableSchema returns the normalized table definition of model T from its
// generated metadata, or nil for a projection.
//
// Deprecated: read Meta.TableSchema from the model's MomoMeta. MetaTableSchema
// is removed in the next release.
func MetaTableSchema[T any, PT Modeler[T]]() *schema.Table {
	m := metaOf[T, PT]()
	if m == nil {
		return nil
	}
	return m.TableSchema
}
