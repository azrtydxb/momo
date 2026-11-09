// SPDX-License-Identifier: Apache-2.0

package momo

// Result reports the outcome of a write statement.
type Result struct {
	RowsAffected int64
	// InsertID is the auto-increment key a single-row insert generated, on
	// dialects whose driver reports it (MySQL and MariaDB); zero otherwise.
	InsertID int64
}

// writeKind selects the rules of columnAssignable.
type writeKind uint8

const (
	writeInsert writeKind = iota
	writeUpdate
)

// columnAssignable reports whether a write of the given kind may assign a
// column with flags f. Identity and read-only columns are never written;
// create-only and immutable columns are written by INSERT only.
func columnAssignable(kind writeKind, f uint16) bool {
	if f&(FlagIdentity|FlagReadOnly) != 0 {
		return false
	}
	return kind == writeInsert || f&(FlagCreateOnly|FlagImmutable) == 0
}
