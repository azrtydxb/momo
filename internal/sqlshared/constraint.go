// SPDX-License-Identifier: Apache-2.0

package sqlshared

import (
	"strings"

	"github.com/azrtydxb/momo"
)

// ConstraintTarget parses "UNIQUE constraint failed: table.column" into its
// table and column. It returns empty strings for any other message; for a
// multi-column constraint (a comma-separated list) it returns the first.
func ConstraintTarget(msg string) (table, column string) {
	i := strings.LastIndex(msg, ": ")
	if i < 0 || !strings.Contains(msg[:i], "constraint failed") {
		return "", ""
	}
	target, _, _ := strings.Cut(msg[i+2:], ",")
	table, column, ok := strings.Cut(strings.TrimSpace(target), ".")
	if !ok {
		return "", ""
	}
	return table, column
}

// ConstraintKind classifies a constraint failure by its message prefix, for a
// SQLITE_CONSTRAINT whose extended code names no kind (a bare code, or one a
// later SQLite adds). It returns nil for any other message.
func ConstraintKind(msg string) error {
	switch {
	case strings.HasPrefix(msg, "UNIQUE constraint failed"), strings.HasPrefix(msg, "PRIMARY KEY constraint failed"):
		return momo.ErrUniqueViolation
	case strings.HasPrefix(msg, "FOREIGN KEY constraint failed"):
		return momo.ErrForeignKeyViolation
	case strings.HasPrefix(msg, "CHECK constraint failed"):
		return momo.ErrCheckViolation
	case strings.HasPrefix(msg, "NOT NULL constraint failed"):
		return momo.ErrNotNullViolation
	}
	return nil
}
