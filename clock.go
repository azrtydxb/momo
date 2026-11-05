// SPDX-License-Identifier: Apache-2.0

package momo

import "time"

// WithClock sets the clock used for automatic timestamps and soft-delete
// markers. The clock is read once per statement. A nil clock keeps the
// default, time.Now.
func WithClock(now func() time.Time) Option {
	return func(db *DB) { db.clock = now }
}

// now reads the owning DB's clock, or time.Now when none is configured, and
// normalizes it to UTC so every driver binds the same instant. The value is
// not truncated: PostgreSQL stores microseconds, so a sub-microsecond clock
// reads back truncated to the microsecond, while SQLite keeps what it is given.
func (e *execer) now() time.Time {
	if e.db != nil && e.db.clock != nil {
		return e.db.clock().UTC()
	}
	return time.Now().UTC()
}
