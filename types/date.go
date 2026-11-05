// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

const dateLayout = "2006-01-02"

// Date is a calendar date (no time zone): date on PostgreSQL, TEXT
// 'YYYY-MM-DD' on SQLite.
type Date struct{ Time time.Time }

// SQLType returns the column type for d.
func (Date) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "date"
	}
	return "TEXT"
}

// Value encodes the date as 'YYYY-MM-DD' using the year, month and day of
// Time in its own location.
func (d Date) Value() (driver.Value, error) {
	return d.Time.Format(dateLayout), nil
}

// Scan accepts 'YYYY-MM-DD' as string or []byte, or a time.Time (as pgx
// returns for date columns), whose calendar day in its own location is kept.
func (d *Date) Scan(src any) error {
	if t, ok := src.(time.Time); ok {
		y, m, day := t.Date()
		d.Time = time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
		return nil
	}
	s, err := text("Date", src)
	if err != nil {
		return err
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("types: invalid Date %q: %w", s, err)
	}
	d.Time = t
	return nil
}

// TimeOfDay is a time of day: time on PostgreSQL, TEXT 'HH:MM:SS.uuuuuu'
// on SQLite.
type TimeOfDay struct{ Nanos int64 }

const nanosPerDay = int64(24 * time.Hour)

// SQLType returns the column type for d.
func (TimeOfDay) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "time"
	}
	return "TEXT"
}

// Value encodes the time as 'HH:MM:SS.uuuuuu', truncated to microseconds
// because PostgreSQL time has microsecond precision (rounding would turn the
// last microsecond of the day into 24:00:00). Nanos outside one day is an
// error.
func (t TimeOfDay) Value() (driver.Value, error) {
	if t.Nanos < 0 || t.Nanos >= nanosPerDay {
		return nil, fmt.Errorf("types: TimeOfDay %d out of range", t.Nanos)
	}
	n := t.Nanos
	h := n / int64(time.Hour)
	n %= int64(time.Hour)
	m := n / int64(time.Minute)
	n %= int64(time.Minute)
	return fmt.Sprintf("%02d:%02d:%02d.%06d", h, m, n/int64(time.Second), n%int64(time.Second)/int64(time.Microsecond)), nil
}

// Scan accepts 'HH:MM:SS' with an optional fraction of 1 to 9 digits (string
// or []byte). PostgreSQL's 24:00:00 is read as the last nanosecond of the
// day. Scan(nil) returns an error: use a pointer or a Null wrapper for
// nullable columns. Integer sources are rejected because drivers disagree on
// their unit.
func (t *TimeOfDay) Scan(src any) error {
	s, err := text("TimeOfDay", src)
	if err != nil {
		return err
	}
	nanos, err := parseTimeOfDay(s)
	if err != nil {
		return fmt.Errorf("types: invalid TimeOfDay %q: %w", s, err)
	}
	t.Nanos = nanos
	return nil
}

func parseTimeOfDay(s string) (int64, error) {
	clock, frac, hasFrac := strings.Cut(s, ".")
	if len(clock) != 8 || clock[2] != ':' || clock[5] != ':' {
		return 0, errors.New("want HH:MM:SS[.fraction]")
	}
	var parts [3]int64
	for i, seg := range []string{clock[0:2], clock[3:5], clock[6:8]} {
		for _, c := range seg {
			if c < '0' || c > '9' {
				return 0, errors.New("non-digit")
			}
			parts[i] = parts[i]*10 + int64(c-'0')
		}
	}
	var fracNanos int64
	if hasFrac {
		if len(frac) < 1 || len(frac) > 9 {
			return 0, errors.New("fraction must have 1 to 9 digits")
		}
		for i := 0; i < 9; i++ {
			fracNanos *= 10
			if i < len(frac) {
				if frac[i] < '0' || frac[i] > '9' {
					return 0, errors.New("non-digit in fraction")
				}
				fracNanos += int64(frac[i] - '0')
			}
		}
	}
	h, m, sec := parts[0], parts[1], parts[2]
	if h == 24 && m == 0 && sec == 0 && fracNanos == 0 {
		return nanosPerDay - 1, nil
	}
	if h > 23 || m > 59 || sec > 59 {
		return 0, errors.New("out of range")
	}
	return h*int64(time.Hour) + m*int64(time.Minute) + sec*int64(time.Second) + fracNanos, nil
}
