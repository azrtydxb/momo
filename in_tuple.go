// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"

	"github.com/azrtydxb/momo/dialect"
)

// inTuple renders "key IN (...)" for one or more columns. An empty key list
// renders the always-false predicate. Composite keys use row values on
// PostgreSQL and MySQL; SQLite uses an OR of ANDs (valid on every SQLite
// version Momo supports). Values are bound, never interpolated.
//
// NULL key elements never match: SQL "IN" is never true for NULL, so a NULL
// tuple element selects nothing, and NOT (inTuple) evaluates to NULL for rows
// whose column is NULL, so such rows are not selected either (relevant to
// unlink-except style deletes). Callers must not rely on NULL matching. A key
// tuple whose length differs from cols, or keys without cols, records a build
// error through Builder.SetErr and renders the always-false predicate.
type inTuple struct {
	table string
	cols  []string
	keys  [][]any
	// flags are optional column flags by position in cols; a sensitive column
	// keeps its key values out of hook events.
	flags []uint16
}

// flagAt returns the flags of the column at position i, or 0.
func (x inTuple) flagAt(i int) uint16 { return flagAt(x.flags, i) }

// keyFlags returns the flags of the named columns of m, or nil when m is nil.
func keyFlags[T any](m *Meta[T], cols []string) []uint16 {
	if m == nil {
		return nil
	}
	var out []uint16
	for i, c := range cols {
		ci := columnIndexOf(m, c)
		if ci < 0 || m.Columns[ci].Flags&FlagSensitive == 0 {
			continue
		}
		if out == nil {
			out = make([]uint16, len(cols))
		}
		out[i] = m.Columns[ci].Flags
	}
	return out
}

// flagAt returns fl[i], or 0 when fl is shorter.
func flagAt(fl []uint16, i int) uint16 {
	if i < len(fl) {
		return fl[i]
	}
	return 0
}

// parentKeyFlags are the flags of the parent columns the relation joins on.
func (r Relation[P, C]) parentKeyFlags() []uint16 { return keyFlags(r.ParentMeta, r.ParentCols) }

// childKeyFlags are the flags of the child columns the relation joins on.
func (r Relation[P, C]) childKeyFlags() []uint16 { return keyFlags(r.ChildMeta, r.ChildCols) }

// ownerFlags are the flags of the columns that hold the parent key: the join
// table columns referencing the parent for ManyToMany, else the child columns.
func (r Relation[P, C]) ownerFlags() []uint16 {
	if r.Through != nil {
		return r.parentKeyFlags()
	}
	return r.childKeyFlags()
}

func (x inTuple) AppendSQL(b *Builder) {
	if len(x.keys) == 0 {
		b.WriteString("1 = 0")
		return
	}
	if len(x.cols) == 0 {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("tuple IN on table %q has keys but no columns", x.table)})
		b.WriteString("1 = 0")
		return
	}
	for i, k := range x.keys {
		if len(k) != len(x.cols) {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("tuple IN on table %q: key %d has %d values for %d columns", x.table, i, len(k), len(x.cols))})
			b.WriteString("1 = 0")
			return
		}
	}
	if !b.checkBindCap(len(x.keys)*len(x.cols), "tuple IN") {
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
			b.argFor(x.flagAt(0), k[0])
		}
		b.WriteString(")")
		return
	}
	if b.Dialect().RowValues && b.Dialect().Name != dialect.SQLite {
		b.WriteString("(")
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
			b.WriteString("(")
			for j, v := range k {
				if j > 0 {
					b.WriteString(", ")
				}
				b.argFor(x.flagAt(j), v)
			}
			b.WriteString(")")
		}
		b.WriteString(")")
		return
	}
	b.WriteString("(")
	for i, k := range x.keys {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString("(")
		for j, c := range x.cols {
			if j > 0 {
				b.WriteString(" AND ")
			}
			b.QualifiedIdent(x.table, c)
			b.WriteString(" = ")
			b.argFor(x.flagAt(j), k[j])
		}
		b.WriteString(")")
	}
	b.WriteString(")")
}

func (inTuple) resultType() bool { return false }

// Composite-key tuple caps per statement. Long composite lists fail long before
// the bind limit: PostgreSQL parses a row-value IN list recursively and rejects
// it with "stack depth limit exceeded" (SQLSTATE 54001) at roughly 10000
// two-column tuples (the exact point depends on max_stack_depth; 32767 always
// fails), and SQLite renders composite keys as a chain of ORs whose expression
// depth is limited to 1000 by default ("Expression tree is too large").
// Single-column lists are flat and use the full bind budget.
const (
	// maxCompositeTuples caps composite tuples per statement on PostgreSQL,
	// MySQL and any dialect without a specific cap.
	maxCompositeTuples = 2000
	// maxCompositeTuplesSQLite is the cap for the SQLite OR-of-ANDs form.
	maxCompositeTuplesSQLite = 400
)

// keysPerStatement returns how many key tuples fit in one statement under the
// dialect's bind limit, leaving reserved parameters for other predicates.
// Composite keys (ncols > 1) are additionally capped (see maxCompositeTuples).
// A dialect without a bind limit (MaxParams <= 0) is limited only by the cap
// for composite keys and returns a large bound for single-column keys.
func keysPerStatement(d *dialect.Dialect, ncols, reserved int) int {
	if ncols < 1 {
		ncols = 1
	}
	n := 1 << 20
	if d.MaxParams > 0 {
		n = max((d.MaxParams-reserved)/ncols, 1)
	}
	if ncols > 1 {
		limit := maxCompositeTuples
		if d.Name == dialect.SQLite {
			limit = maxCompositeTuplesSQLite
		}
		n = min(n, limit)
	}
	return n
}
