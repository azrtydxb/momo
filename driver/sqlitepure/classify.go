// SPDX-License-Identifier: Apache-2.0

package sqlitepure

import (
	"errors"
	"strings"

	"github.com/azrtydxb/momo"
	"modernc.org/sqlite"
)

// SQLite result codes (https://sqlite.org/rescode.html). The primary code is
// the low byte of an extended code.
const (
	codeBusy    = 5
	codeLocked  = 6
	codeConstr  = 19
	extBusySnap = 517  // SQLITE_BUSY_SNAPSHOT
	extCheck    = 275  // SQLITE_CONSTRAINT_CHECK
	extFK       = 787  // SQLITE_CONSTRAINT_FOREIGNKEY
	extNotNull  = 1299 // SQLITE_CONSTRAINT_NOTNULL
	extPK       = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
	extUnique   = 2067 // SQLITE_CONSTRAINT_UNIQUE
)

// driverError carries a driver error while exposing only a fixed message. The
// cause stays reachable through errors.As and Unwrap.
type driverError struct {
	cause error
	msg   string
}

func (e driverError) Error() string { return e.msg }
func (e driverError) Unwrap() error { return e.cause }

// ClassifyError maps a modernc *sqlite.Error to *momo.Error. Other errors, and
// errors that are already *momo.Error, are returned unchanged. SQLite
// messages name tables and columns and never carry row values, so the message
// is safe to render; the *sqlite.Error stays reachable through errors.As.
//
// Besides result codes there is one message-based rule that is specific to
// this driver: a message containing "no such function: vec_" is
// momo.ErrUnsupported, because this driver is built without sqlite-vec. A
// constraint failure reported with the bare SQLITE_CONSTRAINT code instead of
// an extended one is classified by its message prefix.
func ClassifyError(err error) error { return classifyOp("", err) }

// classifyOp is ClassifyError with the operation recorded on an
// ErrUnsupported result, which has no other source for it.
func classifyOp(op string, err error) error {
	if err == nil {
		return nil
	}
	var me *momo.Error
	if errors.As(err, &me) {
		return err
	}
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return err
	}
	msg := cleanMessage(se.Error())
	var kind error
	switch code := se.Code(); {
	case strings.Contains(msg, "no such function: vec_"):
		kind = momo.ErrUnsupported
	case code == extUnique || code == extPK:
		kind = momo.ErrUniqueViolation
	case code == extFK:
		kind = momo.ErrForeignKeyViolation
	case code == extCheck:
		kind = momo.ErrCheckViolation
	case code == extNotNull:
		kind = momo.ErrNotNullViolation
	case code == extBusySnap:
		// A read transaction could not upgrade to a write on a changed snapshot;
		// retrying the whole transaction is the cure, as for a serialization failure.
		kind = momo.ErrSerialization
	case code&0xff == codeBusy || code&0xff == codeLocked:
		kind = momo.ErrBusy
	case code&0xff == codeConstr:
		kind = constraintKind(msg)
	}
	if kind == nil {
		return err
	}
	if kind == momo.ErrUnsupported {
		return &momo.Error{Kind: kind, Op: op, Err: driverError{cause: err, msg: msg}}
	}
	table, column := constraintTarget(msg)
	return &momo.Error{
		Kind:   kind,
		Table:  table,
		Column: column,
		Err:    driverError{cause: err, msg: msg},
	}
}

// constraintKind classifies a bare SQLITE_CONSTRAINT by its message prefix.
func constraintKind(msg string) error {
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

// cleanMessage drops the " (code)" suffix modernc appends to the message.
func cleanMessage(msg string) string {
	if strings.HasSuffix(msg, ")") {
		if i := strings.LastIndex(msg, " ("); i >= 0 {
			return msg[:i]
		}
	}
	return msg
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
