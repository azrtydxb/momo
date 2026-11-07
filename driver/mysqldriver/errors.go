// SPDX-License-Identifier: Apache-2.0

package mysqldriver

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/go-sql-driver/mysql"
)

// driverError carries a server error while exposing only a redacted message.
// MySQL quotes the offending value in messages such as ER_DUP_ENTRY, so
// Error omits the server text; Detail (read by momo.Error.Detail) returns it.
// The cause stays reachable through errors.As and Unwrap.
type driverError struct {
	cause  error
	msg    string
	detail string
}

func (e driverError) Error() string  { return e.msg }
func (e driverError) Detail() string { return e.detail }
func (e driverError) Unwrap() error  { return e.cause }

// kinds maps server error numbers to momo sentinels. MySQL and MariaDB share
// the numbers except the check constraint failure, which MariaDB reports as
// 4025 where MySQL reports 3819.
var kinds = map[uint16]error{
	1062: momo.ErrUniqueViolation,     // ER_DUP_ENTRY
	1451: momo.ErrForeignKeyViolation, // ER_ROW_IS_REFERENCED_2
	1452: momo.ErrForeignKeyViolation, // ER_NO_REFERENCED_ROW_2
	1216: momo.ErrForeignKeyViolation, // ER_NO_REFERENCED_ROW
	1217: momo.ErrForeignKeyViolation, // ER_ROW_IS_REFERENCED
	1048: momo.ErrNotNullViolation,    // ER_BAD_NULL_ERROR
	1364: momo.ErrNotNullViolation,    // ER_NO_DEFAULT_FOR_FIELD
	3819: momo.ErrCheckViolation,      // ER_CHECK_CONSTRAINT_VIOLATED (MySQL)
	4025: momo.ErrCheckViolation,      // ER_CONSTRAINT_FAILED (MariaDB)
	1213: momo.ErrSerialization,       // ER_LOCK_DEADLOCK
	1205: momo.ErrBusy,                // ER_LOCK_WAIT_TIMEOUT
	3572: momo.ErrBusy,                // ER_LOCK_NOWAIT
}

// valueErrors are data errors whose message quotes a bound value. They keep
// their driver type but render without the server text.
var valueErrors = map[uint16]bool{
	1264: true, // ER_WARN_DATA_OUT_OF_RANGE
	1265: true, // WARN_DATA_TRUNCATED
	1292: true, // ER_TRUNCATED_WRONG_VALUE
	1366: true, // ER_TRUNCATED_WRONG_VALUE_FOR_FIELD
	1411: true, // ER_WRONG_VALUE_FOR_TYPE
}

var (
	reDupKey     = regexp.MustCompile(`for key '([^']+)'`)
	reConstraint = regexp.MustCompile("CONSTRAINT `([^`]+)`")
	reCheckName  = regexp.MustCompile(`(?i)check constraint '([^']+)'|CONSTRAINT [` + "`" + `']?([^` + "`" + `' ]+)[` + "`" + `']? failed`)
	reFKTable    = regexp.MustCompile("fails \\(`[^`]+`\\.`([^`]+)`")
	reStmtVerb   = regexp.MustCompile(`(?is)^\s*(insert|replace|update|delete|select)\b`)
	reStmtTable  = regexp.MustCompile("(?is)^\\s*(?:INSERT\\s+(?:IGNORE\\s+)?INTO|REPLACE\\s+INTO|UPDATE|DELETE\\s+FROM)\\s+`([^`]+)`")
)

type classifier struct{}

// ClassifyError implements momo.ErrorClassifier.
func (classifier) ClassifyError(err error) error { return classify(err, "") }

// classify maps a *mysql.MySQLError to a *momo.Error. stmt, when not empty, is
// the statement that failed; it supplies the table when the server message
// does not name one (MariaDB names only the index). Errors that are not server
// errors, or are already *momo.Error, are returned unchanged.
func classify(err error, stmt string) error {
	if err == nil {
		return nil
	}
	var me *momo.Error
	if errors.As(err, &me) {
		return err
	}
	var se *mysql.MySQLError
	if !errors.As(err, &se) {
		return err
	}
	kind, ok := kinds[se.Number]
	if !ok {
		if valueErrors[se.Number] {
			return driverError{cause: err, msg: fmt.Sprintf("mysql: data error %d", se.Number), detail: se.Message}
		}
		return err
	}
	out := &momo.Error{
		Kind: kind,
		Err:  driverError{cause: se, msg: fmt.Sprintf("mysql: error %d", se.Number), detail: se.Message},
	}
	switch se.Number {
	case 1062:
		if m := reDupKey.FindStringSubmatch(se.Message); m != nil {
			out.Constraint = m[1]
		}
	case 1451, 1452:
		if m := reConstraint.FindStringSubmatch(se.Message); m != nil {
			out.Constraint = m[1]
		}
		if m := reFKTable.FindStringSubmatch(se.Message); m != nil {
			out.Table = m[1]
		}
	case 3819, 4025:
		if m := reCheckName.FindStringSubmatch(se.Message); m != nil {
			out.Constraint = m[1] + m[2]
		}
	}
	if out.Table == "" {
		out.Table = tableOf(stmt)
	}
	out.Op = opOf(stmt)
	return out
}

// tableOf returns the table a statement writes to, or "" when it cannot tell.
// Momo quotes every identifier with backticks, which the pattern relies on.
func tableOf(stmt string) string {
	if m := reStmtTable.FindStringSubmatch(stmt); m != nil {
		return m[1]
	}
	return ""
}

// opOf names the operation of a statement for momo.Error.Op, or "" when the
// statement is unknown.
func opOf(stmt string) string {
	if m := reStmtVerb.FindStringSubmatch(stmt); m != nil {
		switch strings.ToLower(m[1]) {
		case "replace":
			return "insert"
		default:
			return strings.ToLower(m[1])
		}
	}
	return ""
}
