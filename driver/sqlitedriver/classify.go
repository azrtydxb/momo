// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"errors"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/sqlshared"
	"github.com/mattn/go-sqlite3"
)

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
	case sqlite3.ErrConstraintUnique, sqlite3.ErrConstraintPrimaryKey, sqlite3.ErrConstraintRowID:
		kind = momo.ErrUniqueViolation
	case sqlite3.ErrConstraintForeignKey:
		kind = momo.ErrForeignKeyViolation
	case sqlite3.ErrConstraintCheck:
		kind = momo.ErrCheckViolation
	case sqlite3.ErrConstraintNotNull:
		kind = momo.ErrNotNullViolation
	case sqlite3.ErrBusySnapshot:
		// A read transaction could not upgrade to a write on a changed snapshot;
		// retrying the whole transaction is the cure, as for a serialization failure.
		kind = momo.ErrSerialization
	}
	if kind == nil {
		switch se.Code {
		case sqlite3.ErrBusy, sqlite3.ErrLocked:
			kind = momo.ErrBusy
		case sqlite3.ErrConstraint:
			// A bare SQLITE_CONSTRAINT, or an extended code not listed above, is
			// classified by its message prefix, as sqlitepure does.
			kind = sqlshared.ConstraintKind(se.Error())
		}
	}
	if kind == nil {
		return err
	}
	msg := se.Error()
	table, column := sqlshared.ConstraintTarget(msg)
	return &momo.Error{
		Kind:   kind,
		Table:  table,
		Column: column,
		Err:    sqlshared.DriverError{Cause: err, Msg: msg},
	}
}
