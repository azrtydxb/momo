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
	CreatedAt  int
	UpdatedAt  int
	PK         []int
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
type TableRef struct {
	Name   string
	Alias  string
	Schema string
}

func (t TableRef) appendFrom(b *Builder) {
	if t.Schema != "" {
		b.Ident(t.Schema)
		b.WriteByte('.')
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

// Source returns the table reference itself; it satisfies TableSource.
func (t TableRef) Source() TableRef { return t }

// TableSource is anything usable in FROM / JOIN.
type TableSource interface {
	appendFrom(b *Builder)
}
