// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
)

// Int64 returns a pointer to v.
//
// Deprecated: take the address of a local int64. Removed in the next release.
func Int64(v int64) *int64 { return schemautil.Int64(v) }

// FTS5Table is the name of the FTS5 virtual table that indexes table.
//
// Deprecated: the name is table + "_fts". Removed in the next release.
func FTS5Table(table string) string { return schemautil.FTS5Table(table) }

// TSVectorColumn is the name of the generated tsvector column of the
// full-text index named index.
//
// Deprecated: the name is index + "_tsv". Removed in the next release.
func TSVectorColumn(index string) string { return schemautil.TSVectorColumn(index) }

// SortedValues returns the values sorted and without duplicates, as a new slice.
//
// Deprecated: use slices.Sort and slices.Compact on a copy. Removed in the
// next release.
func SortedValues(values []string) []string { return schemautil.SortedValues(values) }

// CheckInExpr renders the expression of a value-list check for dialect d.
//
// Deprecated: use Check.ExprFor. Removed in the next release.
func CheckInExpr(d dialect.Name, column string, values []string) string {
	return schemautil.CheckInExpr(d, column, values)
}

// ParseCheckIn reads the expression of a CHECK constraint that restricts one
// column to a list of string literals.
//
// Deprecated: use Check.ValueList. Removed in the next release.
func ParseCheckIn(expr string) (column string, values []string, ok bool) {
	return schemautil.ParseCheckIn(expr)
}

// SameCheck reports whether two checks of one name are the same constraint.
//
// Deprecated: compare the checks with Check.ValueList and Check.Expr.
// Removed in the next release.
func SameCheck(a, b Check) bool { return sameCheck(a, b) }
