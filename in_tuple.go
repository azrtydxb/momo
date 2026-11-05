// SPDX-License-Identifier: Apache-2.0

package momo

import "github.com/azrtydxb/momo/dialect"

// inTuple renders "key IN (...)" for one or more columns. An empty key list
// renders the always-false predicate. Composite keys use row values on
// PostgreSQL and MySQL; SQLite uses an OR of ANDs (valid on every SQLite
// version Momo supports). Values are bound, never interpolated.
type inTuple struct {
	table string
	cols  []string
	keys  [][]any
}

func (x inTuple) AppendSQL(b *Builder) {
	if len(x.keys) == 0 {
		b.WriteString("1 = 0")
		return
	}
	if len(x.cols) == 1 {
		b.QualifiedIdent(x.table, x.cols[0])
		b.WriteString(" IN (")
		for i, k := range x.keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Arg(k[0])
		}
		b.WriteByte(')')
		return
	}
	if b.Dialect().RowValues && b.Dialect().Name != dialect.SQLite {
		b.WriteByte('(')
		for i, c := range x.cols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(x.table, c)
		}
		b.WriteString(") IN (")
		for i, k := range x.keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteByte('(')
			for j, v := range k {
				if j > 0 {
					b.WriteString(", ")
				}
				b.Arg(v)
			}
			b.WriteByte(')')
		}
		b.WriteByte(')')
		return
	}
	b.WriteByte('(')
	for i, k := range x.keys {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteByte('(')
		for j, c := range x.cols {
			if j > 0 {
				b.WriteString(" AND ")
			}
			b.QualifiedIdent(x.table, c)
			b.WriteString(" = ")
			b.Arg(k[j])
		}
		b.WriteByte(')')
	}
	b.WriteByte(')')
}

func (inTuple) resultType() bool { return false }

// keysPerStatement returns how many key tuples fit in one statement under the
// dialect's bind limit, leaving reserved parameters for other predicates. A
// dialect without a limit (MaxParams <= 0) returns a large bound.
func keysPerStatement(d *dialect.Dialect, ncols, reserved int) int {
	if d.MaxParams <= 0 {
		return 1 << 20
	}
	if ncols < 1 {
		ncols = 1
	}
	n := (d.MaxParams - reserved) / ncols
	if n < 1 {
		return 1
	}
	return n
}
