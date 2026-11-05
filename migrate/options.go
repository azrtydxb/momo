// SPDX-License-Identifier: Apache-2.0

package migrate

import "time"

const defaultLockTimeout = 5 * time.Second

type options struct {
	clock   func() time.Time
	config  Config
	dialect Dialect
}

func defaultOptions() *options {
	return &options{
		clock:  func() time.Time { return time.Now().UTC() },
		config: Config{LockTimeout: defaultLockTimeout},
	}
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
