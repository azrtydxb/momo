// SPDX-License-Identifier: Apache-2.0

// Package sqlitedriver adapts mattn/go-sqlite3 and sqlite-vec to momo.
//
// # Requirements
//
// The package needs cgo and a C toolchain, and it is not built for Windows,
// where the sqlite-vec C sources cannot be compiled. In a build without cgo,
// or for Windows, Open reports momo.ErrUnsupported. The cgo-free alternative
// is package sqlitepure (modernc.org/sqlite), which has no vector support.
//
// # Process-wide effects
//
// Importing the package has no side effect. The first Open registers the
// database/sql driver "momo-sqlite3" and calls sqlite_vec.Auto, which installs
// sqlite-vec through sqlite3_auto_extension. That registration is
// process-wide: every SQLite connection opened afterwards by any package in
// the process that links the same SQLite library gets the vec_* functions, not
// only the connections momo opens.
//
// # Hardening
//
// Every connection momo opens verifies that load_extension is disabled and
// that SQLite is at least 3.35.0. For defence in depth, build the program with
// -tags sqlite_omit_load_extension, which removes extension loading from the
// SQLite library altogether.
//
// # Defaults
//
// Config is optional. Its zero values select a 5s busy timeout, synchronous
// NORMAL, immediate transactions and one open connection (the single-writer
// guidance for SQLite). The adapter also sets WAL journalling and foreign keys
// on every connection; a DSN cannot switch those off.
package sqlitedriver
