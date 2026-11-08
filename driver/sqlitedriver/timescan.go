// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SQLite has no time type: momo stores time.Time as text, and mattn hands
// back text unless the column was declared TIMESTAMP, DATETIME or DATE. Scan
// therefore swaps *time.Time and **time.Time destinations for scanners that
// parse the text forms SQLite and mattn produce.

// timeLayouts are tried in order. A value without a zone is UTC.
var timeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

var errUnparsableTime = errors.New("sqlitedriver: cannot parse the stored text as a time")

func parseTime(s string) (time.Time, error) {
	for _, l := range timeLayouts {
		if t, err := time.ParseInLocation(l, s, time.UTC); err == nil {
			return t, nil
		}
	}
	// The stored text is not echoed: it may be user data.
	return time.Time{}, errUnparsableTime
}

// timeValue converts a driver value to a time. ok is false for NULL.
func timeValue(src any) (t time.Time, ok bool, err error) {
	switch v := src.(type) {
	case nil:
		return time.Time{}, false, nil
	case time.Time:
		return v, true, nil
	case string:
		t, err = parseTime(v)
		return t, err == nil, err
	case []byte:
		t, err = parseTime(string(v))
		return t, err == nil, err
	case int64:
		return time.Unix(v, 0).UTC(), true, nil
	}
	return time.Time{}, false, fmt.Errorf("sqlitedriver: cannot scan %T into a time", src)
}

type timeScanner struct{ dst *time.Time }

func (s timeScanner) Scan(src any) error {
	t, ok, err := timeValue(src)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("sqlitedriver: NULL cannot be scanned into time.Time")
	}
	*s.dst = t
	return nil
}

type timePtrScanner struct{ dst **time.Time }

func (s timePtrScanner) Scan(src any) error {
	t, ok, err := timeValue(src)
	if err != nil {
		return err
	}
	if !ok {
		*s.dst = nil
		return nil
	}
	*s.dst = &t
	return nil
}

type nullTimeScanner struct{ dst *sql.NullTime }

func (s nullTimeScanner) Scan(src any) error {
	t, ok, err := timeValue(src)
	if err != nil {
		return err
	}
	s.dst.Time, s.dst.Valid = t, ok
	return nil
}

type nullGenericTimeScanner struct{ dst *sql.Null[time.Time] }

func (s nullGenericTimeScanner) Scan(src any) error {
	t, ok, err := timeValue(src)
	if err != nil {
		return err
	}
	s.dst.V, s.dst.Valid = t, ok
	return nil
}

// wrapDests replaces time destinations with scanners. It returns dest itself
// when there is nothing to replace.
func wrapDests(dest []any) []any {
	var out []any
	for i, d := range dest {
		var w any
		switch p := d.(type) {
		case *time.Time:
			w = timeScanner{p}
		case **time.Time:
			w = timePtrScanner{p}
		case *sql.NullTime:
			w = nullTimeScanner{p}
		case *sql.Null[time.Time]:
			w = nullGenericTimeScanner{p}
		default:
			continue
		}
		if out == nil {
			out = append([]any(nil), dest...)
		}
		out[i] = w
	}
	if out == nil {
		return dest
	}
	return out
}
