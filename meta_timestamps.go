// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"time"
)

// timestampValue converts the statement time into a value of the same Go
// type as sample, the column's field value. It reports false for field types
// automatic timestamps do not support (time.Time, *time.Time and the integer
// unix-seconds kinds int64 are supported).
func timestampValue(now time.Time, sample any) (any, bool) {
	switch sample.(type) {
	case time.Time:
		return now, true
	case *time.Time:
		return &now, true
	case int64:
		return now.Unix(), true
	case *int64:
		u := now.Unix()
		return &u, true
	}
	return nil, false
}

// timestampUnset reports whether a model's timestamp field value counts as
// not assigned by the caller: a zero time or a nil pointer.
func timestampUnset(v any) bool {
	switch t := v.(type) {
	case time.Time:
		return t.IsZero()
	case *time.Time:
		return t == nil
	case int64:
		return t == 0
	case *int64:
		return t == nil
	}
	return false
}

// autoTimestamp returns the value for the timestamp column at index ci
// (CreatedAt or UpdatedAt of m), read at now. ok is false when the model has
// no such column, it is not assignable, or its field type is unsupported.
// Insert and update share it, and so can upsert.
func autoTimestamp[T any](m *Meta[T], ci int, now time.Time) (any, bool) {
	if ci < 0 || ci >= len(m.Columns) || m.Value == nil {
		return nil, false
	}
	var zero T
	return timestampValue(now, m.Value(&zero, ci))
}

// checkTimestampField reports an error when the created-at or updated-at
// field of column ci has a Go type automatic timestamps do not support, so
// such a field fails loudly instead of being silently left unset. It is nil
// for a missing column.
func checkTimestampField[T any](m *Meta[T], ci int) error {
	if ci < 0 || ci >= len(m.Columns) || m.Value == nil {
		return nil
	}
	var zero T
	sample := m.Value(&zero, ci)
	if _, ok := timestampValue(time.Time{}, sample); ok {
		return nil
	}
	return fmt.Errorf("automatic timestamp column %q has unsupported field type %T; use time.Time, *time.Time, int64 or *int64", m.Columns[ci].Name, sample)
}
