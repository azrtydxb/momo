// SPDX-License-Identifier: Apache-2.0

package sqlitedriver

import "github.com/azrtydxb/momo/internal/sqliteconf"

// Config holds the recommended SQLite settings. Zero values select the
// defaults: BusyTimeout 5s, Synchronous "NORMAL", TxLock "immediate" and
// MaxOpenConns 1 (the single-writer guidance; a goroutine that holds a
// transaction or pinned connection and then uses the pool deadlocks with one
// connection). It is the same type as sqlitepure.Config; see
// internal/sqliteconf for the field list.
type Config = sqliteconf.Config
