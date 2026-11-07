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
	noSum       bool
	// goMigrations are the migrations registered with WithGoMigrations.
	goMigrations []GoMigration
	// lockTimeout and statementTimeout, when non-nil, were given explicitly
	// (possibly as zero) and override the Config values.
	lockTimeout, statementTimeout *time.Duration
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

// WithLockTimeout sets the lock_timeout of the migration session. Unlike
// Config.LockTimeout, an explicit zero is kept and disables the timeout
// (PostgreSQL treats 0 as off), for example for a long index build.
func WithLockTimeout(d time.Duration) Option {
	return func(o *options) { o.lockTimeout = &d }
}

// WithStatementTimeout sets the statement_timeout of the migration session.
// Unlike Config.StatementTimeout, an explicit zero is sent to the server and
// disables the timeout.
func WithStatementTimeout(d time.Duration) Option {
	return func(o *options) { o.statementTimeout = &d }
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

// WithoutSum turns off the momo.sum requirement. Without it Apply and Status
// refuse a directory that has no momo.sum, because an edited migration file
// could not be detected. It exists for tests and for tools that check files
// by other means; production code keeps momo.sum.
func WithoutSum() Option {
	return func(o *options) { o.noSum = true }
}
