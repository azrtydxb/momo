// SPDX-License-Identifier: Apache-2.0

// Package pgxdriver adapts jackc/pgx v5 to momo.
package pgxdriver

import (
	"errors"
	"regexp"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5/pgconn"
)

// driverError carries a driver error while exposing only a redacted message.
// The cause stays reachable through errors.As / Unwrap, but its text (which
// for PostgreSQL may include key values in Detail) is never rendered.
type driverError struct {
	cause error
	msg   string
}

func (e driverError) Error() string { return e.msg }
func (e driverError) Unwrap() error { return e.cause }

// ClassifyError maps *pgconn.PgError (found with errors.As) to *momo.Error.
// Errors that are not recognized, or are already *momo.Error, are returned
// unchanged. The PgError Detail field, which can contain row values, is never
// copied into any string; it remains available on the unwrapped cause.
// Data exceptions (SQLSTATE class 22) are returned with a redacted message.
func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	var me *momo.Error
	if errors.As(err, &me) {
		return err
	}
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return redactEncodeError(err)
	}
	var kind error
	switch pe.Code {
	case "23505":
		kind = momo.ErrUniqueViolation
	case "23503":
		kind = momo.ErrForeignKeyViolation
	case "23514":
		kind = momo.ErrCheckViolation
	case "23502":
		kind = momo.ErrNotNullViolation
	case "40001", "40P01":
		kind = momo.ErrSerialization
	case "55P03":
		kind = momo.ErrBusy
	default:
		// Data exceptions (class 22, e.g. 22P02) quote the offending bound
		// value in Message, so keep the server text out of the rendered error.
		// The PgError stays reachable through errors.As.
		if strings.HasPrefix(pe.Code, "22") {
			return driverError{cause: err, msg: "pgx: data exception (SQLSTATE " + pe.Code + ")"}
		}
		return err
	}
	return &momo.Error{
		Kind:       kind,
		Table:      pe.TableName,
		Column:     pe.ColumnName,
		Constraint: pe.ConstraintName,
		Err:        driverError{cause: pe, msg: pe.Message},
	}
}

// encodeArgRe finds the argument position in pgx's "failed to encode args[N]"
// prefix.
var encodeArgRe = regexp.MustCompile(`failed to encode args\[(\d+)\]`)

// redactEncodeError withholds the text of a pgx client-side encode failure.
// pgx formats the offending bound value into those messages, so rendering
// them would leak data a caller bound. The original error stays reachable
// through errors.Is, errors.As and Unwrap; the redacted message keeps the
// argument position when pgx reported one. Other errors are returned as is.
func redactEncodeError(err error) error {
	var de driverError
	if errors.As(err, &de) {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, "failed to encode args[") && !strings.Contains(msg, "unable to encode ") {
		return err
	}
	out := "pgx: failed to encode argument (value withheld)"
	if m := encodeArgRe.FindStringSubmatch(msg); m != nil {
		out = "pgx: failed to encode args[" + m[1] + "] (value withheld)"
	}
	return driverError{cause: err, msg: out}
}
