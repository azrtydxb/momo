// SPDX-License-Identifier: Apache-2.0

package momo

// InsertAssignment is one explicit column write that is valid in an INSERT
// only. Create-only and immutable columns produce it from Set; it is not an
// Assignment, so Update.Set rejects it at compile time. Raw SQL is outside
// this guarantee.
type InsertAssignment struct{ a Assignment }

// ColumnName returns the assigned column.
func (a InsertAssignment) ColumnName() string { return a.a.col }

// ReadOnlyColumn is the descriptor of a read-only column (DB generated or
// computed). It has predicate and ordering methods only; no write method
// exists, so a Set on it does not compile.
type ReadOnlyColumn[T any] struct{ colCore[T] }

// NewReadOnlyColumn is used by generated code.
func NewReadOnlyColumn[T any](table, name string, flags uint16) ReadOnlyColumn[T] {
	return ReadOnlyColumn[T]{newColCore[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c ReadOnlyColumn[T]) As(alias string) ReadOnlyColumn[T] {
	return ReadOnlyColumn[T]{c.withAlias(alias)}
}

// NullReadOnlyColumn is the descriptor of a nullable read-only column. It
// has predicate and ordering methods only.
type NullReadOnlyColumn[T any] struct{ colCore[T] }

// NewNullReadOnlyColumn is used by generated code.
func NewNullReadOnlyColumn[T any](table, name string, flags uint16) NullReadOnlyColumn[T] {
	return NullReadOnlyColumn[T]{newColCore[T](table, name, flags|FlagNullable)}
}

// As returns the same column qualified by a table alias.
func (c NullReadOnlyColumn[T]) As(alias string) NullReadOnlyColumn[T] {
	return NullReadOnlyColumn[T]{c.withAlias(alias)}
}

// CreateOnlyColumn is the descriptor of a column writable in INSERT only. Its
// Set returns an InsertAssignment, which Update.Set does not accept.
type CreateOnlyColumn[T any] struct{ colCore[T] }

// NewCreateOnlyColumn is used by generated code.
func NewCreateOnlyColumn[T any](table, name string, flags uint16) CreateOnlyColumn[T] {
	return CreateOnlyColumn[T]{newColCore[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c CreateOnlyColumn[T]) As(alias string) CreateOnlyColumn[T] {
	return CreateOnlyColumn[T]{c.withAlias(alias)}
}

// Set assigns v on insert. The result is an InsertAssignment, which only
// Insert accepts.
func (c CreateOnlyColumn[T]) Set(v T) InsertAssignment {
	return InsertAssignment{Column[T]{c.colCore}.Set(v)}
}

// ImmutableColumn is the descriptor of a column that is set on insert and
// never changed. Its Set returns an InsertAssignment, which Update.Set does
// not accept.
type ImmutableColumn[T any] struct{ colCore[T] }

// NewImmutableColumn is used by generated code.
func NewImmutableColumn[T any](table, name string, flags uint16) ImmutableColumn[T] {
	return ImmutableColumn[T]{newColCore[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c ImmutableColumn[T]) As(alias string) ImmutableColumn[T] {
	return ImmutableColumn[T]{c.withAlias(alias)}
}

// Set assigns v on insert. The result is an InsertAssignment, which only
// Insert accepts.
func (c ImmutableColumn[T]) Set(v T) InsertAssignment {
	return InsertAssignment{Column[T]{c.colCore}.Set(v)}
}
