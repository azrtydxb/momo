// SPDX-License-Identifier: Apache-2.0

// Package sqliteconf holds the settings type shared by driver/sqlitedriver
// (cgo, mattn/go-sqlite3) and driver/sqlitepure (pure Go, modernc). Both
// export it as an alias, so sqlitedriver.Config and sqlitepure.Config are one
// type, while the pure-Go package does not import the cgo one and therefore
// never links mattn/go-sqlite3 or sqlite-vec.
package sqliteconf

import "time"

// Config holds the recommended SQLite settings. Zero values select the defaults.
//
// MaxOpenConns defaults to 1, the single-writer guidance for SQLite. With one
// connection, a goroutine that holds a transaction or a pinned connection and
// then runs a statement through the pool (instead of through the transaction
// or the pin) waits for itself and deadlocks; raise MaxOpenConns when code
// does that. An in-memory database is private to one connection, so Open
// refuses MaxOpenConns above 1 for one, and Pinned.Discard, which destroys the
// connection, destroys an in-memory database with it.
type Config struct {
	BusyTimeout  time.Duration // default 5 * time.Second
	Synchronous  string        // "NORMAL" (default), "FULL", "EXTRA", "OFF"
	TxLock       string        // "immediate" (default), "deferred", "exclusive"
	MaxOpenConns int           // default 1 (single-writer guidance); 0 means 1
}
