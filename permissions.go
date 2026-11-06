// SPDX-License-Identifier: Apache-2.0

package momo

// InsertAssignment is one explicit column write that is valid in an INSERT
// only. Create-only and immutable columns produce it from Set; it is not an
// Assignment, so Update.Set rejects it at compile time. Raw SQL is outside
// this guarantee.
type InsertAssignment struct{ a Assignment }

// ColumnName returns the assigned column.
func (a InsertAssignment) ColumnName() string { return a.a.col }

// ReadOnlyColumn is the descriptor of a read-only column (DB generated or computed). It has predicate and ordering methods only; no write method exists, so a Set on it does not compile.
type ReadOnlyColumn[T any] struct{ col Column[T] }

// NewReadOnlyColumn is used by generated code.
func NewReadOnlyColumn[T any](table, name string, flags uint16) ReadOnlyColumn[T] {
	return ReadOnlyColumn[T]{col: NewColumn[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c ReadOnlyColumn[T]) As(alias string) ReadOnlyColumn[T] { c.col = c.col.As(alias); return c }

func (c ReadOnlyColumn[T]) AppendSQL(b *Builder)             { c.col.AppendSQL(b) }
func (c ReadOnlyColumn[T]) AsExpr() Expression[T]            { return c.col.AsExpr() }
func (c ReadOnlyColumn[T]) resultType() T                    { return c.col.resultType() }
func (c ReadOnlyColumn[T]) order() Order                     { return c.col.order() }
func (c ReadOnlyColumn[T]) ColumnName() string               { return c.col.ColumnName() }
func (c ReadOnlyColumn[T]) Table() string                    { return c.col.Table() }
func (c ReadOnlyColumn[T]) Flags() uint16                    { return c.col.Flags() }
func (c ReadOnlyColumn[T]) Eq(v T) Predicate                 { return c.col.Eq(v) }
func (c ReadOnlyColumn[T]) Ne(v T) Predicate                 { return c.col.Ne(v) }
func (c ReadOnlyColumn[T]) Lt(v T) Predicate                 { return c.col.Lt(v) }
func (c ReadOnlyColumn[T]) Le(v T) Predicate                 { return c.col.Le(v) }
func (c ReadOnlyColumn[T]) Gt(v T) Predicate                 { return c.col.Gt(v) }
func (c ReadOnlyColumn[T]) Ge(v T) Predicate                 { return c.col.Ge(v) }
func (c ReadOnlyColumn[T]) IsNull() Predicate                { return c.col.IsNull() }
func (c ReadOnlyColumn[T]) IsNotNull() Predicate             { return c.col.IsNotNull() }
func (c ReadOnlyColumn[T]) EqExpr(e Expression[T]) Predicate { return c.col.EqExpr(e) }
func (c ReadOnlyColumn[T]) NeExpr(e Expression[T]) Predicate { return c.col.NeExpr(e) }
func (c ReadOnlyColumn[T]) LtExpr(e Expression[T]) Predicate { return c.col.LtExpr(e) }
func (c ReadOnlyColumn[T]) LeExpr(e Expression[T]) Predicate { return c.col.LeExpr(e) }
func (c ReadOnlyColumn[T]) GtExpr(e Expression[T]) Predicate { return c.col.GtExpr(e) }
func (c ReadOnlyColumn[T]) GeExpr(e Expression[T]) Predicate { return c.col.GeExpr(e) }
func (c ReadOnlyColumn[T]) IsDistinctFrom(v T) Predicate     { return c.col.IsDistinctFrom(v) }
func (c ReadOnlyColumn[T]) IsNotDistinctFrom(v T) Predicate  { return c.col.IsNotDistinctFrom(v) }
func (c ReadOnlyColumn[T]) Between(lo, hi T) Predicate       { return c.col.Between(lo, hi) }
func (c ReadOnlyColumn[T]) In(vs ...T) Predicate             { return c.col.In(vs...) }
func (c ReadOnlyColumn[T]) NotIn(vs ...T) Predicate          { return c.col.NotIn(vs...) }
func (c ReadOnlyColumn[T]) InQuery(q Subquery) Predicate     { return c.col.InQuery(q) }
func (c ReadOnlyColumn[T]) NotInQuery(q Subquery) Predicate  { return c.col.NotInQuery(q) }
func (c ReadOnlyColumn[T]) Asc() Order                       { return c.col.Asc() }
func (c ReadOnlyColumn[T]) Desc() Order                      { return c.col.Desc() }

// NullReadOnlyColumn is the descriptor of a nullable read-only column. It has predicate and ordering methods only.
type NullReadOnlyColumn[T any] struct{ col NullColumn[T] }

// NewNullReadOnlyColumn is used by generated code.
func NewNullReadOnlyColumn[T any](table, name string, flags uint16) NullReadOnlyColumn[T] {
	return NullReadOnlyColumn[T]{col: NewNullColumn[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c NullReadOnlyColumn[T]) As(alias string) NullReadOnlyColumn[T] {
	c.col = c.col.As(alias)
	return c
}

func (c NullReadOnlyColumn[T]) AppendSQL(b *Builder)             { c.col.AppendSQL(b) }
func (c NullReadOnlyColumn[T]) AsExpr() Expression[T]            { return c.col.AsExpr() }
func (c NullReadOnlyColumn[T]) resultType() T                    { return c.col.resultType() }
func (c NullReadOnlyColumn[T]) order() Order                     { return c.col.order() }
func (c NullReadOnlyColumn[T]) ColumnName() string               { return c.col.ColumnName() }
func (c NullReadOnlyColumn[T]) Table() string                    { return c.col.Table() }
func (c NullReadOnlyColumn[T]) Flags() uint16                    { return c.col.Flags() }
func (c NullReadOnlyColumn[T]) Eq(v T) Predicate                 { return c.col.Eq(v) }
func (c NullReadOnlyColumn[T]) Ne(v T) Predicate                 { return c.col.Ne(v) }
func (c NullReadOnlyColumn[T]) Lt(v T) Predicate                 { return c.col.Lt(v) }
func (c NullReadOnlyColumn[T]) Le(v T) Predicate                 { return c.col.Le(v) }
func (c NullReadOnlyColumn[T]) Gt(v T) Predicate                 { return c.col.Gt(v) }
func (c NullReadOnlyColumn[T]) Ge(v T) Predicate                 { return c.col.Ge(v) }
func (c NullReadOnlyColumn[T]) IsNull() Predicate                { return c.col.IsNull() }
func (c NullReadOnlyColumn[T]) IsNotNull() Predicate             { return c.col.IsNotNull() }
func (c NullReadOnlyColumn[T]) EqExpr(e Expression[T]) Predicate { return c.col.EqExpr(e) }
func (c NullReadOnlyColumn[T]) NeExpr(e Expression[T]) Predicate { return c.col.NeExpr(e) }
func (c NullReadOnlyColumn[T]) LtExpr(e Expression[T]) Predicate { return c.col.LtExpr(e) }
func (c NullReadOnlyColumn[T]) LeExpr(e Expression[T]) Predicate { return c.col.LeExpr(e) }
func (c NullReadOnlyColumn[T]) GtExpr(e Expression[T]) Predicate { return c.col.GtExpr(e) }
func (c NullReadOnlyColumn[T]) GeExpr(e Expression[T]) Predicate { return c.col.GeExpr(e) }
func (c NullReadOnlyColumn[T]) IsDistinctFrom(v T) Predicate     { return c.col.IsDistinctFrom(v) }
func (c NullReadOnlyColumn[T]) IsNotDistinctFrom(v T) Predicate  { return c.col.IsNotDistinctFrom(v) }
func (c NullReadOnlyColumn[T]) Between(lo, hi T) Predicate       { return c.col.Between(lo, hi) }
func (c NullReadOnlyColumn[T]) In(vs ...T) Predicate             { return c.col.In(vs...) }
func (c NullReadOnlyColumn[T]) NotIn(vs ...T) Predicate          { return c.col.NotIn(vs...) }
func (c NullReadOnlyColumn[T]) InQuery(q Subquery) Predicate     { return c.col.InQuery(q) }
func (c NullReadOnlyColumn[T]) NotInQuery(q Subquery) Predicate  { return c.col.NotInQuery(q) }
func (c NullReadOnlyColumn[T]) Asc() Order                       { return c.col.Asc() }
func (c NullReadOnlyColumn[T]) Desc() Order                      { return c.col.Desc() }

// CreateOnlyColumn is the descriptor of a column writable in INSERT only. Its Set returns an InsertAssignment, which Update.Set does not accept.
type CreateOnlyColumn[T any] struct{ col Column[T] }

// NewCreateOnlyColumn is used by generated code.
func NewCreateOnlyColumn[T any](table, name string, flags uint16) CreateOnlyColumn[T] {
	return CreateOnlyColumn[T]{col: NewColumn[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c CreateOnlyColumn[T]) As(alias string) CreateOnlyColumn[T] { c.col = c.col.As(alias); return c }

func (c CreateOnlyColumn[T]) AppendSQL(b *Builder)             { c.col.AppendSQL(b) }
func (c CreateOnlyColumn[T]) AsExpr() Expression[T]            { return c.col.AsExpr() }
func (c CreateOnlyColumn[T]) resultType() T                    { return c.col.resultType() }
func (c CreateOnlyColumn[T]) order() Order                     { return c.col.order() }
func (c CreateOnlyColumn[T]) ColumnName() string               { return c.col.ColumnName() }
func (c CreateOnlyColumn[T]) Table() string                    { return c.col.Table() }
func (c CreateOnlyColumn[T]) Flags() uint16                    { return c.col.Flags() }
func (c CreateOnlyColumn[T]) Eq(v T) Predicate                 { return c.col.Eq(v) }
func (c CreateOnlyColumn[T]) Ne(v T) Predicate                 { return c.col.Ne(v) }
func (c CreateOnlyColumn[T]) Lt(v T) Predicate                 { return c.col.Lt(v) }
func (c CreateOnlyColumn[T]) Le(v T) Predicate                 { return c.col.Le(v) }
func (c CreateOnlyColumn[T]) Gt(v T) Predicate                 { return c.col.Gt(v) }
func (c CreateOnlyColumn[T]) Ge(v T) Predicate                 { return c.col.Ge(v) }
func (c CreateOnlyColumn[T]) IsNull() Predicate                { return c.col.IsNull() }
func (c CreateOnlyColumn[T]) IsNotNull() Predicate             { return c.col.IsNotNull() }
func (c CreateOnlyColumn[T]) EqExpr(e Expression[T]) Predicate { return c.col.EqExpr(e) }
func (c CreateOnlyColumn[T]) NeExpr(e Expression[T]) Predicate { return c.col.NeExpr(e) }
func (c CreateOnlyColumn[T]) LtExpr(e Expression[T]) Predicate { return c.col.LtExpr(e) }
func (c CreateOnlyColumn[T]) LeExpr(e Expression[T]) Predicate { return c.col.LeExpr(e) }
func (c CreateOnlyColumn[T]) GtExpr(e Expression[T]) Predicate { return c.col.GtExpr(e) }
func (c CreateOnlyColumn[T]) GeExpr(e Expression[T]) Predicate { return c.col.GeExpr(e) }
func (c CreateOnlyColumn[T]) IsDistinctFrom(v T) Predicate     { return c.col.IsDistinctFrom(v) }
func (c CreateOnlyColumn[T]) IsNotDistinctFrom(v T) Predicate  { return c.col.IsNotDistinctFrom(v) }
func (c CreateOnlyColumn[T]) Between(lo, hi T) Predicate       { return c.col.Between(lo, hi) }
func (c CreateOnlyColumn[T]) In(vs ...T) Predicate             { return c.col.In(vs...) }
func (c CreateOnlyColumn[T]) NotIn(vs ...T) Predicate          { return c.col.NotIn(vs...) }
func (c CreateOnlyColumn[T]) InQuery(q Subquery) Predicate     { return c.col.InQuery(q) }
func (c CreateOnlyColumn[T]) NotInQuery(q Subquery) Predicate  { return c.col.NotInQuery(q) }
func (c CreateOnlyColumn[T]) Asc() Order                       { return c.col.Asc() }
func (c CreateOnlyColumn[T]) Desc() Order                      { return c.col.Desc() }

// Set assigns v on insert. The result is an InsertAssignment, which only
// Insert accepts.
func (c CreateOnlyColumn[T]) Set(v T) InsertAssignment { return InsertAssignment{c.col.Set(v)} }

// ImmutableColumn is the descriptor of a column that is set on insert and never changed. Its Set returns an InsertAssignment, which Update.Set does not accept.
type ImmutableColumn[T any] struct{ col Column[T] }

// NewImmutableColumn is used by generated code.
func NewImmutableColumn[T any](table, name string, flags uint16) ImmutableColumn[T] {
	return ImmutableColumn[T]{col: NewColumn[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c ImmutableColumn[T]) As(alias string) ImmutableColumn[T] { c.col = c.col.As(alias); return c }

func (c ImmutableColumn[T]) AppendSQL(b *Builder)             { c.col.AppendSQL(b) }
func (c ImmutableColumn[T]) AsExpr() Expression[T]            { return c.col.AsExpr() }
func (c ImmutableColumn[T]) resultType() T                    { return c.col.resultType() }
func (c ImmutableColumn[T]) order() Order                     { return c.col.order() }
func (c ImmutableColumn[T]) ColumnName() string               { return c.col.ColumnName() }
func (c ImmutableColumn[T]) Table() string                    { return c.col.Table() }
func (c ImmutableColumn[T]) Flags() uint16                    { return c.col.Flags() }
func (c ImmutableColumn[T]) Eq(v T) Predicate                 { return c.col.Eq(v) }
func (c ImmutableColumn[T]) Ne(v T) Predicate                 { return c.col.Ne(v) }
func (c ImmutableColumn[T]) Lt(v T) Predicate                 { return c.col.Lt(v) }
func (c ImmutableColumn[T]) Le(v T) Predicate                 { return c.col.Le(v) }
func (c ImmutableColumn[T]) Gt(v T) Predicate                 { return c.col.Gt(v) }
func (c ImmutableColumn[T]) Ge(v T) Predicate                 { return c.col.Ge(v) }
func (c ImmutableColumn[T]) IsNull() Predicate                { return c.col.IsNull() }
func (c ImmutableColumn[T]) IsNotNull() Predicate             { return c.col.IsNotNull() }
func (c ImmutableColumn[T]) EqExpr(e Expression[T]) Predicate { return c.col.EqExpr(e) }
func (c ImmutableColumn[T]) NeExpr(e Expression[T]) Predicate { return c.col.NeExpr(e) }
func (c ImmutableColumn[T]) LtExpr(e Expression[T]) Predicate { return c.col.LtExpr(e) }
func (c ImmutableColumn[T]) LeExpr(e Expression[T]) Predicate { return c.col.LeExpr(e) }
func (c ImmutableColumn[T]) GtExpr(e Expression[T]) Predicate { return c.col.GtExpr(e) }
func (c ImmutableColumn[T]) GeExpr(e Expression[T]) Predicate { return c.col.GeExpr(e) }
func (c ImmutableColumn[T]) IsDistinctFrom(v T) Predicate     { return c.col.IsDistinctFrom(v) }
func (c ImmutableColumn[T]) IsNotDistinctFrom(v T) Predicate  { return c.col.IsNotDistinctFrom(v) }
func (c ImmutableColumn[T]) Between(lo, hi T) Predicate       { return c.col.Between(lo, hi) }
func (c ImmutableColumn[T]) In(vs ...T) Predicate             { return c.col.In(vs...) }
func (c ImmutableColumn[T]) NotIn(vs ...T) Predicate          { return c.col.NotIn(vs...) }
func (c ImmutableColumn[T]) InQuery(q Subquery) Predicate     { return c.col.InQuery(q) }
func (c ImmutableColumn[T]) NotInQuery(q Subquery) Predicate  { return c.col.NotInQuery(q) }
func (c ImmutableColumn[T]) Asc() Order                       { return c.col.Asc() }
func (c ImmutableColumn[T]) Desc() Order                      { return c.col.Desc() }

// Set assigns v on insert. The result is an InsertAssignment, which only
// Insert accepts.
func (c ImmutableColumn[T]) Set(v T) InsertAssignment { return InsertAssignment{c.col.Set(v)} }
