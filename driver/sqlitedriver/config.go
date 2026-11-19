// SPDX-License-Identifier: Apache-2.0

package sqlitedriver

import "time"

// Config holds the recommended SQLite settings. Zero values select the
// defaults: BusyTimeout 5s, Synchronous "NORMAL", TxLock "immediate" and
// MaxOpenConns 1 (the single-writer guidance).
//
// MaxOpenConns defaults to 1, the single-writer guidance for SQLite. With one
// connection, a goroutine that holds a transaction or a pinned connection and
// then runs a statement through the pool (instead of through the transaction
// or the pin) waits for itself and deadlocks; raise MaxOpenConns when code
// does that. An in-memory database is private to one connection, so Open
// refuses MaxOpenConns above 1 for one, and Pinned.Discard, which destroys the
// connection, destroys an in-memory database with it.
//
// Pass a Config to Open with WithConfig, or set single fields with
// WithBusyTimeout, WithSynchronous, WithTxLock and WithMaxOpenConns.
// sqlitepure.Config has the same fields, so one converts to the other:
// sqlitepure.Config(cfg).
type Config struct {
	BusyTimeout  time.Duration // default 5 * time.Second
	Synchronous  string        // "NORMAL" (default), "FULL", "EXTRA", "OFF"
	TxLock       string        // "immediate" (default), "deferred", "exclusive"
	MaxOpenConns int           // default 1 (single-writer guidance); 0 means 1
	// ReadOnly is refused by this driver with momo.ErrValidation: it cannot
	// open a database without writing its journal mode. Use driver/sqlitepure,
	// which honors it.
	ReadOnly bool
}

// Option configures how Open builds the pool. New takes the same type so that
// every driver constructor has one shape, but a pool that already exists has
// been configured, so New refuses an option that sets anything: the driver it
// returns fails every call with momo.ErrValidation.
type Option func(*Config)

// WithConfig sets every setting of Open to cfg, replacing the ones given by
// earlier options.
func WithConfig(cfg Config) Option { return func(c *Config) { *c = cfg } }

// WithBusyTimeout sets Config.BusyTimeout.
func WithBusyTimeout(d time.Duration) Option { return func(c *Config) { c.BusyTimeout = d } }

// WithSynchronous sets Config.Synchronous.
func WithSynchronous(mode string) Option { return func(c *Config) { c.Synchronous = mode } }

// WithTxLock sets Config.TxLock.
func WithTxLock(lock string) Option { return func(c *Config) { c.TxLock = lock } }

// WithMaxOpenConns sets Config.MaxOpenConns.
func WithMaxOpenConns(n int) Option { return func(c *Config) { c.MaxOpenConns = n } }

// config applies opts to the zero Config.
func config(opts []Option) Config {
	var c Config
	for _, o := range opts {
		o(&c)
	}
	return c
}
