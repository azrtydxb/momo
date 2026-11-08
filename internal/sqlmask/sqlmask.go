// SPDX-License-Identifier: Apache-2.0

// Package sqlmask hides the text of database/sql conversion failures. It
// imports only momo and the standard library, so every adapter that scans
// through database/sql can use it without linking another driver.
package sqlmask

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/azrtydxb/momo"
)

// Error carries a conversion failure while exposing only a fixed message. The
// cause stays reachable through errors.Unwrap.
type Error struct {
	Cause error
	Msg   string
}

func (e Error) Error() string { return e.Msg }
func (e Error) Unwrap() error { return e.Cause }

// ConversionErr hides the text of a database/sql conversion failure:
// database/sql quotes the stored value in it ("converting driver.Value type
// string ("...")"). c must already be classified by the driver. Only errors
// that look like a conversion failure are masked; cancellation, a closed Rows,
// a *momo.Error and errors for which isDriverErr reports true pass through.
// The cause stays reachable through errors.Unwrap, as with pgxdriver data
// exceptions. msg is the fixed text of the masked error.
func ConversionErr(c error, msg string, isDriverErr func(error) bool) error {
	if c == nil {
		return nil
	}
	var me *momo.Error
	if isDriverErr(c) || errors.As(c, &me) ||
		errors.Is(c, context.Canceled) || errors.Is(c, context.DeadlineExceeded) || errors.Is(c, sql.ErrNoRows) {
		return c
	}
	text := c.Error()
	if !strings.Contains(text, "converting") && !strings.Contains(text, "Scan error on column") {
		return c
	}
	return Error{Cause: c, Msg: msg}
}
