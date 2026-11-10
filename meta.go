// SPDX-License-Identifier: Apache-2.0

package momo

import "github.com/azrtydxb/momo/schema"

// Model is embedded in a struct to declare it a Momo model. The struct tag
// on the embedded field carries table-level options:
//
//	type Document struct {
//	    momo.Model `momo:"table:documents"`
//	    ...
//	}
//
// It is zero-sized and has no behavior at runtime.
type Model struct{}

// View is embedded next to Model in a struct that maps a database view or
// materialized view. The generator records it in Meta.View, and every write
// builder (Insert, Upsert, Update, SoftDelete, BulkUpdate, Delete, Merge)
// then fails with ErrValidation naming the model; reads work as for any
// model. Declare the view itself in the model's MomoConfig with c.View.
//
//	type ActiveUser struct {
//	    momo.Model `momo:"table:active_users"`
//	    momo.View
//	    ID int64 `momo:"pk"`
//	}
//
// It is zero-sized and has no behavior at runtime.
type View struct{}

// Projection is embedded in a struct to declare it a result type for typed
// projections. Its fields are scanned positionally from the select list.
type Projection struct{}

// Column flags carried by generated column descriptors and ColumnMeta.
const (
	FlagPK uint16 = 1 << iota
	FlagUnique
	FlagNullable
	FlagIdentity // generated identity; omitted from inserts
	FlagReadOnly // DB-generated or computed; never written
	FlagVersion  // optimistic-lock version column
	FlagSoftDelete
	FlagHasDefault
	FlagCreateOnly
	FlagImmutable
	FlagSensitive
	FlagCreatedAt
	FlagUpdatedAt
	// FlagDBDefault: a Model or Models insert leaves the column out when the
	// field holds its zero value, so the database default applies, and reads
	// the stored value back. Set by the dbdefault tag.
	FlagDBDefault
)

// ColumnMeta describes one mapped field.
type ColumnMeta struct {
	Name  string
	Flags uint16
}

// Meta is the generated, reflection-free description of a model or
// projection type T. Generated code returns a pointer to a package-level
// Meta value; Momo never mutates it.
type Meta[T any] struct {
	// Table is empty for projections.
	Table string
	// Schema is the database schema qualifying Table; empty for the default.
	Schema  string
	Columns []ColumnMeta
	// Ptr returns the address of the field for column i, for scanning.
	Ptr func(m *T, i int) any
	// Value returns the value of the field for column i, for binding.
	Value func(m *T, i int) any
	// TableSchema is the normalized table definition; nil for projections.
	TableSchema *schema.Table

	// Derived column indexes, filled by generated code. -1 means absent for
	// Identity, Version, SoftDelete, CreatedAt and UpdatedAt.
	Identity   int
	Version    int
	SoftDelete int
	// SoftDeleteKind is the type of the soft-delete marker column; the zero
	// value is a nullable timestamp.
	SoftDeleteKind SoftDeleteKind
	CreatedAt      int
	UpdatedAt      int
	PK             []int

	// Fields are Go-side defaults and validators per column, run by the
	// write builders before SQL is built. See FieldHook.
	Fields []FieldHook[T]

	// IDGens declares Go-side id generators per column; an insert fills the
	// column when its field holds the zero value. See WithIDGenerator.
	IDGens []IDGen
	// Hooks are the typed model hooks the write and read builders call. See
	// HookSet.
	Hooks HookSet[T]
	// View marks a model backed by a database view. Generated code sets it
	// for a model that embeds View. Insert, Upsert, Update, SoftDelete,
	// BulkUpdate, Delete and Merge refuse such a model with ErrValidation.
	View bool
}

// viewWriteErr is the ErrValidation error of a write to a view model, or nil
// when meta is not a view.
func viewWriteErr[T any](op string, meta *Meta[T]) error {
	if meta == nil || !meta.View {
		return nil
	}
	return validation(op, meta.Table, "%T is a view model and cannot be written", *new(T))
}

// Modeler is implemented (by generated code) on pointers to model and
// projection types. It ties T to its metadata at compile time.
type Modeler[T any] interface {
	*T
	MomoMeta() *Meta[T]
}

func metaOf[T any, PT Modeler[T]]() *Meta[T] {
	var p PT // nil pointer; generated MomoMeta never dereferences its receiver
	return p.MomoMeta()
}

// TableRef names a table source in FROM and JOIN clauses, optionally
// aliased. Generated per-model packages embed it in their Table type.
//
// Generated code sets SoftDelete and SoftDeleteKind on a model's Table when
// the model has a soft-delete column. A projection read FROM such a table
// then excludes soft-deleted rows like a model does. Use As to alias it.
type TableRef struct {
	Name   string
	Alias  string
	Schema string
	// SoftDelete is the name of the table's soft-delete marker column; empty
	// when the table has none.
	SoftDelete string
	// SoftDeleteKind is the type of that marker column.
	SoftDeleteKind SoftDeleteKind
}

func (t TableRef) appendFrom(b *Builder) {
	if t.Schema != "" {
		b.Ident(t.Schema)
		b.writeByte('.')
	}
	b.Ident(t.Name)
	if t.Alias != "" && t.Alias != t.Name {
		b.WriteString(" AS ")
		b.Ident(t.Alias)
	}
}

// Qualifier returns the name columns of this source are qualified with.
// An unaliased schema-qualified table ("tenant"."documents") is exposed
// under its bare name, so its columns render as "documents"."id"; alias
// one side to join two same-named tables from different schemas.
func (t TableRef) Qualifier() string {
	if t.Alias != "" {
		return t.Alias
	}
	return t.Name
}

// As returns a copy of t under the alias, keeping everything else, including
// the soft-delete column that a projection read FROM t is filtered by.
func (t TableRef) As(alias string) TableRef {
	t.Alias = alias
	return t
}

// Source returns the table reference itself; it satisfies TableSource.
func (t TableRef) Source() TableRef { return t }

// TableSource is anything usable in FROM / JOIN.
type TableSource interface {
	appendFrom(b *Builder)
}
