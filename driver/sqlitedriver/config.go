// SPDX-License-Identifier: Apache-2.0

// Package sqlitedriver adapts mattn/go-sqlite3 and sqlite-vec to momo. It needs
// cgo; the pure-Go alternative is package sqlitepure, which has no vectors.
//
// Importing the package has no side effect. The first Open registers the
// database/sql driver "momo-sqlite3" and calls sqlite_vec.Auto, which installs
// sqlite-vec with sqlite3_auto_extension. That registration is process-wide:
// every SQLite connection opened afterwards by any package in the process
// that links the same SQLite library gets the vec functions.
package sqlitedriver

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
