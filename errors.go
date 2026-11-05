// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo/vector"
)

// Sentinel error kinds. Errors returned by Momo wrap one of these (match
// with errors.Is) and, when a database error caused them, also wrap the
// original driver error (reach it with errors.As, e.g. *pgconn.PgError).
var (
	// ErrNotFound: a single-row operation (One, First, Model update/delete)
	// matched no row. Multi-row reads (All, Each, Iter) never return it.
	ErrNotFound = errors.New("momo: not found")
	// ErrTooManyRows: One matched more than one row.
	ErrTooManyRows = errors.New("momo: more than one row")

	ErrUniqueViolation     = errors.New("momo: unique constraint violation")
	ErrForeignKeyViolation = errors.New("momo: foreign key constraint violation")
	ErrCheckViolation      = errors.New("momo: check constraint violation")
	ErrNotNullViolation    = errors.New("momo: not-null constraint violation")
	// ErrSerialization: the database aborted the transaction because of a
	// serialization failure or deadlock. Retrying the whole transaction
	// may succeed; Momo never retries on its own.
	ErrSerialization = errors.New("momo: serialization failure")

	// ErrOptimisticLock: an update or delete guarded by an expected
	// version matched no row because the version changed (or the row is
	// gone).
	ErrOptimisticLock = errors.New("momo: optimistic lock conflict")
	// ErrUnsupported: the dialect or driver cannot express the request.
	ErrUnsupported = errors.New("momo: unsupported by dialect or driver")
	// ErrUnfilteredMutation: an UPDATE or DELETE without a WHERE clause was
	// attempted without calling AllRows.
	ErrUnfilteredMutation = errors.New("momo: refusing UPDATE/DELETE without WHERE; call AllRows to confirm")
	// ErrTxDone: the transaction was already committed or rolled back.
	ErrTxDone = errors.New("momo: transaction already finished")

	// ErrValidation: a value failed validation before reaching the database.
	ErrValidation = errors.New("momo: validation failed")
	// ErrBusy: the database is busy or locked and could not serve the request.
	ErrBusy = errors.New("momo: database is busy")
	// ErrPinnedBusy: PinnedConn.Release was called while a statement was
	// still executing on the connection from another goroutine. Nothing was
	// released; call Release again once the statement has finished.
	ErrPinnedBusy = errors.New("momo: pinned connection is busy")
	// ErrLimitExceeded: a configured limit (rows, size, parameters) was exceeded.
	ErrLimitExceeded = errors.New("momo: limit exceeded")
	// ErrInvalidVector: a vector value failed validation.
	ErrInvalidVector = vector.ErrInvalidVector
)

// Error is a structured Momo error.
type Error struct {
	Kind       error  // one of the Err* sentinels
	Op         string // "select", "insert", "update", "delete", "exec", "build", ...
	Table      string // when known
	Column     string // when known
	Constraint string // when known
	Err        error  // underlying (often driver) error; may be nil
}

func (e *Error) Error() string {
	var sb strings.Builder
	sb.WriteString(e.Kind.Error())
	if e.Op != "" {
		fmt.Fprintf(&sb, " (op=%s", e.Op)
		if e.Table != "" {
			fmt.Fprintf(&sb, " table=%s", e.Table)
		}
		if e.Constraint != "" {
			fmt.Fprintf(&sb, " constraint=%s", e.Constraint)
		}
		if e.Column != "" {
			fmt.Fprintf(&sb, " column=%s", e.Column)
		}
		sb.WriteByte(')')
	}
	if e.Err != nil {
		sb.WriteString(": ")
		sb.WriteString(e.Err.Error())
	}
	return sb.String()
}

// Detail returns the underlying error text, or "" when there is none.
func (e *Error) Detail() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Is reports whether target is this error's kind.
func (e *Error) Is(target error) bool { return target == e.Kind }

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error { return e.Err }

// ErrorClassifier converts a raw driver error into a structured *Error when
// it recognizes it, and returns err unchanged otherwise. Drivers implement
// it; the root package stays free of driver imports.
type ErrorClassifier interface {
	ClassifyError(err error) error
}
