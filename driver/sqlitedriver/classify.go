// SPDX-License-Identifier: Apache-2.0

package sqlitedriver

import (
	"errors"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/mattn/go-sqlite3"
)

// driverError carries a driver error while exposing only a fixed message. The
// cause stays reachable through errors.As and Unwrap.
type driverError struct {
	cause error
	msg   string
}

func (e driverError) Error() string { return e.msg }
func (e driverError) Unwrap() error { return e.cause }

// ClassifyError maps a mattn sqlite3.Error to *momo.Error. Other errors, and
// errors that are already *momo.Error, are returned unchanged. SQLite
// messages name tables and columns and never carry row values, so the message
// is safe to render; the sqlite3.Error stays reachable through errors.As.
func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	var me *momo.Error
	if errors.As(err, &me) {
		return err
	}
	var se sqlite3.Error
	if !errors.As(err, &se) {
		return err
	}
	var kind error
	switch se.ExtendedCode {
	case sqlite3.ErrConstraintUnique, sqlite3.ErrConstraintPrimaryKey:
		kind = momo.ErrUniqueViolation
	case sqlite3.ErrConstraintForeignKey:
		kind = momo.ErrForeignKeyViolation
	case sqlite3.ErrConstraintCheck:
		kind = momo.ErrCheckViolation
	case sqlite3.ErrConstraintNotNull:
		kind = momo.ErrNotNullViolation
	}
	if kind == nil {
		switch se.Code {
		case sqlite3.ErrBusy, sqlite3.ErrLocked:
			kind = momo.ErrBusy
		}
	}
	if kind == nil {
		return err
	}
	msg := se.Error()
	table, column := constraintTarget(msg)
	return &momo.Error{
		Kind:   kind,
		Table:  table,
		Column: column,
		Err:    driverError{cause: err, msg: msg},
	}
}

// constraintTarget parses "UNIQUE constraint failed: table.column" into its
// table and column. It returns empty strings for any other message; for a
// multi-column constraint (a comma-separated list) it returns the first.
func constraintTarget(msg string) (table, column string) {
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
