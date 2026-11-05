// SPDX-License-Identifier: Apache-2.0

package migrate

import "time"

const defaultLockTimeout = 5 * time.Second

// defaultLockWait is how long Apply waits for the migration lock when neither
// ApplyOptions.LockWait nor WithLockWait says otherwise.
const defaultLockWait = 30 * time.Second

type options struct {
	clock   func() time.Time
	config  Config
	dialect Dialect
	// lockWait is the engine default for waiting on the migration lock;
	// lockWaitSet distinguishes an explicit zero (do not wait) from unset.
	lockWait    time.Duration
	lockWaitSet bool
}

// Option configures an Engine.
type Option func(*options)

// WithClock sets the clock used to stamp new migration versions.
func WithClock(now func() time.Time) Option {
	return func(o *options) { o.clock = now }
}

// WithConfig sets the engine configuration. A zero LockTimeout keeps the 5s default.
func WithConfig(c Config) Option {
	return func(o *options) {
		if c.LockTimeout == 0 {
			c.LockTimeout = defaultLockTimeout
		}
		o.config = c
	}
}

// WithDialect sets the dialect; it is required by NewEngine.
func WithDialect(d Dialect) Option {
	return func(o *options) { o.dialect = d }
}

// WithLockWait sets how long Apply waits for the migration lock before it
// returns ErrLocked. Zero means do not wait. ApplyOptions.LockWait, when
// non-zero, overrides it for one call; without either the wait is 30s.
func WithLockWait(d time.Duration) Option {
	return func(o *options) { o.lockWait, o.lockWaitSet = d, true }
}
