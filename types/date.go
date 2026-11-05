// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql/driver"
	"fmt"
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

// TimeOfDay is a time of day: time on PostgreSQL, TEXT 'HH:MM:SS.nnnnnnnnn'
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

// Value encodes the time as 'HH:MM:SS.nnnnnnnnn', which both PostgreSQL time
// and the SQLite TEXT column accept. Nanos outside one day is an error.
func (t TimeOfDay) Value() (driver.Value, error) {
	if t.Nanos < 0 || t.Nanos >= nanosPerDay {
		return nil, fmt.Errorf("types: TimeOfDay %d out of range", t.Nanos)
	}
	n := t.Nanos
	h := n / int64(time.Hour)
	n %= int64(time.Hour)
	m := n / int64(time.Minute)
	n %= int64(time.Minute)
	return fmt.Sprintf("%02d:%02d:%02d.%09d", h, m, n/int64(time.Second), n%int64(time.Second)), nil
}

// Scan accepts 'HH:MM:SS[.fraction]' as string or []byte, or int64
// nanoseconds since midnight.
func (t *TimeOfDay) Scan(src any) error {
	if n, ok := src.(int64); ok {
		if n < 0 || n >= nanosPerDay {
			return fmt.Errorf("types: TimeOfDay %d out of range", n)
		}
		t.Nanos = n
		return nil
	}
	s, err := text("TimeOfDay", src)
	if err != nil {
		return err
	}
	layout := "15:04:05"
	if len(s) > 8 {
		layout = "15:04:05.999999999"
	}
	p, err := time.Parse(layout, s)
	if err != nil {
		return fmt.Errorf("types: invalid TimeOfDay %q: %w", s, err)
	}
	t.Nanos = int64(p.Hour())*int64(time.Hour) + int64(p.Minute())*int64(time.Minute) +
		int64(p.Second())*int64(time.Second) + int64(p.Nanosecond())
	return nil
}
