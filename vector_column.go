// SPDX-License-Identifier: Apache-2.0

package momo

import "github.com/azrtydxb/momo/vector"

// VectorValue is the set of Go types a vector column can hold.
type VectorValue interface {
	vector.Vector | vector.HalfVector | vector.SparseVector | vector.Bits
}

// VectorColumn is a typed vector column descriptor. Its dimension is known
// at generation time so distance arguments are validated at build time.
type VectorColumn[V VectorValue] struct {
	table string
	name  string
	dim   int
	flags uint16
}

// NewVectorColumn is used by generated code.
func NewVectorColumn[V VectorValue](table, name string, dim int, flags uint16) VectorColumn[V] {
	return VectorColumn[V]{table: table, name: name, dim: dim, flags: flags}
}

// As returns the same column qualified by a table alias.
func (c VectorColumn[V]) As(alias string) VectorColumn[V] { c.table = alias; return c }

// AppendSQL renders the qualified, quoted column.
func (c VectorColumn[V]) AppendSQL(b *Builder) { b.QualifiedIdent(c.table, c.name) }

// ColumnName returns the unquoted column name.
func (c VectorColumn[V]) ColumnName() string { return c.name }

// Dim returns the declared dimension.
func (c VectorColumn[V]) Dim() int { return c.dim }

// IsNull is true when the column holds SQL NULL.
func (c VectorColumn[V]) IsNull() Predicate { return postfix{c, " IS NULL"} }

// Set assigns a value in INSERT or UPDATE.
func (c VectorColumn[V]) Set(v V) Assignment { return Assignment{col: c.name, kind: setValue, val: v} }
