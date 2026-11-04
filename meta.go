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
	Table   string
	Columns []ColumnMeta
	// Ptr returns the address of the field for column i, for scanning.
	Ptr func(m *T, i int) any
	// Value returns the value of the field for column i, for binding.
	Value func(m *T, i int) any
	// Schema is the normalized table definition; nil for projections.
	Schema *schema.Table

	// Derived indexes, filled by generated code (-1 when absent).
	Identity   int
	Version    int
	SoftDelete int
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
	Name  string
	Alias string
}

func (t TableRef) appendFrom(b *Builder) {
	b.Ident(t.Name)
	if t.Alias != "" && t.Alias != t.Name {
		b.WriteString(" AS ")
		b.Ident(t.Alias)
	}
}

// Qualifier returns the name columns of this source are qualified with.
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
