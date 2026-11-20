// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	iredact "github.com/azrtydxb/momo/internal/redact"
)

// redactedError shows err's message with URLs and passwords removed while
// Unwrap keeps errors.Is and errors.As working on the original chain.
type redactedError struct{ err error }

func (e redactedError) Error() string { return iredact.RedactURLs(e.err.Error()) }
func (e redactedError) Unwrap() error { return e.err }

// redactErr wraps err so that its message is safe to print or log. It returns
// nil for nil and leaves an already wrapped error alone.
func redactErr(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(redactedError); ok {
		return err
	}
	return redactedError{err}
}
