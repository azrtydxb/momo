// SPDX-License-Identifier: Apache-2.0

// Package sqlite is the SQLite migration dialect: it reads a live database
// into a schema.Schema through the SQLite catalog (sqlite_schema and the
// table-valued pragmas), renders a change plan as SQLite DDL, and rebuilds a
// table with the twelve-step procedure of
// https://www.sqlite.org/lang_altertable.html#otheralter when ALTER TABLE
// cannot make the change.
//
// The package has no cgo dependency of its own: it works on any
// momo.PinnedConn, and only the tests that open a real database need
// driver/sqlitedriver.
package sqlite

import "github.com/azrtydxb/momo/dialect"

// Dialect is the SQLite implementation of migrate.Dialect. It holds no state;
// every method works on the pinned connection it is given.
type Dialect struct{}

// New returns the SQLite dialect.
func New() Dialect { return Dialect{} }

// Dialect describes SQLite: identifier quoting, placeholders and capabilities.
func (Dialect) Dialect() *dialect.Dialect { return dialect.For(dialect.SQLite) }

// historyTable and lockTable are momo's own bookkeeping tables. Inspect leaves
// both out.
const (
	historyTable = "momo_migrations"
	lockTable    = "momo_migration_lock"
)
