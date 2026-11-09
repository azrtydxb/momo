// SPDX-License-Identifier: Apache-2.0

package momo

// InsertAssignment is one explicit column write that is valid in an INSERT
// only. Create-only and immutable columns produce it from Set; it is not an
// Assignment, so Update.Set rejects it at compile time. Raw SQL is outside
// this guarantee.
type InsertAssignment struct{ a Assignment }

// ColumnName returns the assigned column.
func (a InsertAssignment) ColumnName() string { return a.a.col }

// The marker types below give each permission wrapper a distinct underlying
// type. Without them every wrapper would be struct{ colCore[T] }, so Go would
// allow converting a read-only or immutable column to Column and calling Set,
// which the wrappers exist to prevent.
type (
	readOnlyMark     struct{}
	nullReadOnlyMark struct{}
	createOnlyMark   struct{}
	immutableMark    struct{}
)

// ReadOnlyColumn is the descriptor of a read-only column (DB generated or
// computed). It has predicate and ordering methods only; no write method
// exists, so a Set on it does not compile.
type ReadOnlyColumn[T any] struct {
	colCore[T]
	_ readOnlyMark
}

// NewReadOnlyColumn is used by generated code.
func NewReadOnlyColumn[T any](table, name string, flags uint16) ReadOnlyColumn[T] {
	return ReadOnlyColumn[T]{colCore: newColCore[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c ReadOnlyColumn[T]) As(alias string) ReadOnlyColumn[T] {
	return ReadOnlyColumn[T]{colCore: c.withAlias(alias)}
}

// NullReadOnlyColumn is the descriptor of a nullable read-only column. It
// has predicate and ordering methods only.
type NullReadOnlyColumn[T any] struct {
	colCore[T]
	_ nullReadOnlyMark
}

// NewNullReadOnlyColumn is used by generated code.
func NewNullReadOnlyColumn[T any](table, name string, flags uint16) NullReadOnlyColumn[T] {
	return NullReadOnlyColumn[T]{colCore: newColCore[T](table, name, flags|FlagNullable)}
}

// As returns the same column qualified by a table alias.
func (c NullReadOnlyColumn[T]) As(alias string) NullReadOnlyColumn[T] {
	return NullReadOnlyColumn[T]{colCore: c.withAlias(alias)}
}

// CreateOnlyColumn is the descriptor of a column writable in INSERT only. Its
// Set returns an InsertAssignment, which Update.Set does not accept.
type CreateOnlyColumn[T any] struct {
	colCore[T]
	_ createOnlyMark
}

// NewCreateOnlyColumn is used by generated code.
func NewCreateOnlyColumn[T any](table, name string, flags uint16) CreateOnlyColumn[T] {
	return CreateOnlyColumn[T]{colCore: newColCore[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c CreateOnlyColumn[T]) As(alias string) CreateOnlyColumn[T] {
	return CreateOnlyColumn[T]{colCore: c.withAlias(alias)}
}

// Set assigns v on insert. The result is an InsertAssignment, which only
// Insert accepts.
func (c CreateOnlyColumn[T]) Set(v T) InsertAssignment {
	return InsertAssignment{Column[T]{c.colCore}.Set(v)}
}

// ImmutableColumn is the descriptor of a column that is set on insert and
// never changed. Its Set returns an InsertAssignment, which Update.Set does
// not accept.
type ImmutableColumn[T any] struct {
	colCore[T]
	_ immutableMark
}

// NewImmutableColumn is used by generated code.
func NewImmutableColumn[T any](table, name string, flags uint16) ImmutableColumn[T] {
	return ImmutableColumn[T]{colCore: newColCore[T](table, name, flags)}
}

// As returns the same column qualified by a table alias.
func (c ImmutableColumn[T]) As(alias string) ImmutableColumn[T] {
	return ImmutableColumn[T]{colCore: c.withAlias(alias)}
}

// Set assigns v on insert. The result is an InsertAssignment, which only
// Insert accepts.
func (c ImmutableColumn[T]) Set(v T) InsertAssignment {
	return InsertAssignment{Column[T]{c.colCore}.Set(v)}
}

// Excluded is the value proposed for insertion, for an upsert condition.
func (c ReadOnlyColumn[T]) Excluded() Expression[T] { return c.colCore.Excluded() }

// Excluded is the value proposed for insertion, for an upsert condition.
func (c NullReadOnlyColumn[T]) Excluded() Expression[T] { return c.colCore.Excluded() }

// Excluded is the value proposed for insertion, for an upsert condition.
func (c CreateOnlyColumn[T]) Excluded() Expression[T] { return c.colCore.Excluded() }

// Excluded is the value proposed for insertion, for an upsert condition.
func (c ImmutableColumn[T]) Excluded() Expression[T] { return c.colCore.Excluded() }
