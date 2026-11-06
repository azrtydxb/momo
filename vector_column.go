// SPDX-License-Identifier: Apache-2.0

package momo

import "github.com/azrtydxb/momo/vector"

// VectorValue is the set of Go types a vector column can hold.
type VectorValue interface {
	vector.Vector | vector.HalfVector | vector.SparseVector | vector.Bits
}

// VectorSource is any vector column descriptor a distance builder accepts:
// VectorColumn and its nullable and permission variants (ReadOnlyVectorColumn,
// CreateOnlyVectorColumn, ImmutableVectorColumn). A write restriction does
// not stop a column from being searched.
type VectorSource[V VectorValue] interface {
	vectorColumn() VectorColumn[V]
}

// vectorBase is the read side shared by every vector column variant. It
// delegates to an inner Column so predicate, ordering and flag semantics are
// identical to a plain column. Range comparisons (Lt, Le, Gt, Ge, Between)
// are deliberately absent: vectors have no meaningful total order, and the
// nearest-neighbour builders are the ordering tool for them.
type vectorBase[V VectorValue] struct {
	col Column[V]
	dim int
}

func (c vectorBase[V]) vectorColumn() VectorColumn[V] { return VectorColumn[V]{vectorBase: c} }

// AppendSQL renders the qualified, quoted column.
func (c vectorBase[V]) AppendSQL(b *Builder) { c.col.AppendSQL(b) }

// AsExpr returns the column as an Expression[V].
func (c vectorBase[V]) AsExpr() Expression[V] { return c.col.AsExpr() }
func (c vectorBase[V]) resultType() V         { return c.col.resultType() }
func (c vectorBase[V]) order() Order          { return c.col.order() }

// ColumnName returns the unquoted column name.
func (c vectorBase[V]) ColumnName() string { return c.col.ColumnName() }

// Table returns the qualifier (table name or alias).
func (c vectorBase[V]) Table() string { return c.col.Table() }

// Flags returns the generated column flags.
func (c vectorBase[V]) Flags() uint16 { return c.col.Flags() }

// Dim returns the declared dimension (0 when unchecked).
func (c vectorBase[V]) Dim() int { return c.dim }

func (c vectorBase[V]) Eq(v V) Predicate                 { return c.col.Eq(v) }
func (c vectorBase[V]) Ne(v V) Predicate                 { return c.col.Ne(v) }
func (c vectorBase[V]) EqExpr(e Expression[V]) Predicate { return c.col.EqExpr(e) }
func (c vectorBase[V]) NeExpr(e Expression[V]) Predicate { return c.col.NeExpr(e) }

// IsNull is true when the column holds SQL NULL.
func (c vectorBase[V]) IsNull() Predicate { return c.col.IsNull() }

// IsNotNull is true when the column holds a value.
func (c vectorBase[V]) IsNotNull() Predicate { return c.col.IsNotNull() }

// IsDistinctFrom is a NULL-safe inequality.
func (c vectorBase[V]) IsDistinctFrom(v V) Predicate { return c.col.IsDistinctFrom(v) }

// IsNotDistinctFrom is a NULL-safe equality.
func (c vectorBase[V]) IsNotDistinctFrom(v V) Predicate { return c.col.IsNotDistinctFrom(v) }

func (c vectorBase[V]) In(vs ...V) Predicate            { return c.col.In(vs...) }
func (c vectorBase[V]) NotIn(vs ...V) Predicate         { return c.col.NotIn(vs...) }
func (c vectorBase[V]) InQuery(q Subquery) Predicate    { return c.col.InQuery(q) }
func (c vectorBase[V]) NotInQuery(q Subquery) Predicate { return c.col.NotInQuery(q) }
func (c vectorBase[V]) Asc() Order                      { return c.col.Asc() }
func (c vectorBase[V]) Desc() Order                     { return c.col.Desc() }

func (c vectorBase[V]) as(alias string) vectorBase[V] { c.col = c.col.As(alias); return c }

func newVectorBase[V VectorValue](table, name string, dim int, flags uint16) vectorBase[V] {
	return vectorBase[V]{col: NewColumn[V](table, name, flags), dim: dim}
}

// VectorColumn is a typed, writable vector column descriptor. Its dimension
// is known at generation time so distance arguments are validated at build
// time. It offers the same predicate, ordering and write methods as Column
// except the range comparisons.
type VectorColumn[V VectorValue] struct{ vectorBase[V] }

// NewVectorColumn is used by generated code.
func NewVectorColumn[V VectorValue](table, name string, dim int, flags uint16) VectorColumn[V] {
	return VectorColumn[V]{newVectorBase[V](table, name, dim, flags)}
}

// As returns the same column qualified by a table alias.
func (c VectorColumn[V]) As(alias string) VectorColumn[V] {
	c.vectorBase = c.vectorBase.as(alias)
	return c
}

// Set assigns a value in INSERT or UPDATE.
func (c VectorColumn[V]) Set(v V) Assignment { return c.col.Set(v) }

// SetExpr assigns an expression.
func (c VectorColumn[V]) SetExpr(e Expression[V]) Assignment { return c.col.SetExpr(e) }

// SetDefault assigns the column's database default.
func (c VectorColumn[V]) SetDefault() Assignment { return c.col.SetDefault() }

// SetExcluded assigns the value proposed for insertion in an upsert.
func (c VectorColumn[V]) SetExcluded() Assignment { return c.col.SetExcluded() }

// Excluded is the vector proposed for insertion in an upsert, rendered as
// EXCLUDED.<col>. See Column.Excluded.
func (c VectorColumn[V]) Excluded() Expression[V] { return c.col.Excluded() }

// NullVectorColumn is the descriptor of a nullable vector column. It adds
// SetNull.
type NullVectorColumn[V VectorValue] struct{ VectorColumn[V] }

// NewNullVectorColumn is used by generated code.
func NewNullVectorColumn[V VectorValue](table, name string, dim int, flags uint16) NullVectorColumn[V] {
	return NullVectorColumn[V]{NewVectorColumn[V](table, name, dim, flags|FlagNullable)}
}

// As returns the same column qualified by a table alias.
func (c NullVectorColumn[V]) As(alias string) NullVectorColumn[V] {
	c.VectorColumn = c.VectorColumn.As(alias)
	return c
}

// SetNull assigns SQL NULL.
func (c NullVectorColumn[V]) SetNull() Assignment {
	return Assignment{col: c.ColumnName(), kind: setNull}
}

// ReadOnlyVectorColumn is a read-only vector column (database generated or
// computed). It can be searched and filtered but has no write method, so a
// Set on it does not compile. Nullability comes from the flags.
type ReadOnlyVectorColumn[V VectorValue] struct{ vectorBase[V] }

// NewReadOnlyVectorColumn is used by generated code.
func NewReadOnlyVectorColumn[V VectorValue](table, name string, dim int, flags uint16) ReadOnlyVectorColumn[V] {
	return ReadOnlyVectorColumn[V]{newVectorBase[V](table, name, dim, flags)}
}

// As returns the same column qualified by a table alias.
func (c ReadOnlyVectorColumn[V]) As(alias string) ReadOnlyVectorColumn[V] {
	c.vectorBase = c.vectorBase.as(alias)
	return c
}

// CreateOnlyVectorColumn is a vector column writable in INSERT only. Its Set
// returns an InsertAssignment, which Update.Set does not accept.
type CreateOnlyVectorColumn[V VectorValue] struct{ vectorBase[V] }

// NewCreateOnlyVectorColumn is used by generated code.
func NewCreateOnlyVectorColumn[V VectorValue](table, name string, dim int, flags uint16) CreateOnlyVectorColumn[V] {
	return CreateOnlyVectorColumn[V]{newVectorBase[V](table, name, dim, flags)}
}

// As returns the same column qualified by a table alias.
func (c CreateOnlyVectorColumn[V]) As(alias string) CreateOnlyVectorColumn[V] {
	c.vectorBase = c.vectorBase.as(alias)
	return c
}

// Set assigns v on insert; only Insert accepts the result.
func (c CreateOnlyVectorColumn[V]) Set(v V) InsertAssignment { return InsertAssignment{c.col.Set(v)} }

// ImmutableVectorColumn is a vector column set on insert and never changed.
// Its Set returns an InsertAssignment, which Update.Set does not accept.
type ImmutableVectorColumn[V VectorValue] struct{ vectorBase[V] }

// NewImmutableVectorColumn is used by generated code.
func NewImmutableVectorColumn[V VectorValue](table, name string, dim int, flags uint16) ImmutableVectorColumn[V] {
	return ImmutableVectorColumn[V]{newVectorBase[V](table, name, dim, flags)}
}

// As returns the same column qualified by a table alias.
func (c ImmutableVectorColumn[V]) As(alias string) ImmutableVectorColumn[V] {
	c.vectorBase = c.vectorBase.as(alias)
	return c
}

// Set assigns v on insert; only Insert accepts the result.
func (c ImmutableVectorColumn[V]) Set(v V) InsertAssignment { return InsertAssignment{c.col.Set(v)} }
