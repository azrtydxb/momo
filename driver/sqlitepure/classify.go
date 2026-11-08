// SPDX-License-Identifier: Apache-2.0

package sqlitepure

import (
	"errors"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/sqlshared"
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
	extRowID    = 2579 // SQLITE_CONSTRAINT_ROWID
	extUnique   = 2067 // SQLITE_CONSTRAINT_UNIQUE
)

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
	case code == extUnique || code == extPK || code == extRowID:
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
		kind = sqlshared.ConstraintKind(msg)
	}
	if kind == nil {
		return err
	}
	if kind == momo.ErrUnsupported {
		return &momo.Error{Kind: kind, Op: op, Err: sqlshared.DriverError{Cause: err, Msg: msg}}
	}
	table, column := sqlshared.ConstraintTarget(msg)
	return &momo.Error{
		Kind:   kind,
		Table:  table,
		Column: column,
		Err:    sqlshared.DriverError{Cause: err, Msg: msg},
	}
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
