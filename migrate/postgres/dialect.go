// SPDX-License-Identifier: Apache-2.0

// Package postgres is the PostgreSQL migration dialect: it reads a live
// database into a schema.Schema through catalog queries.
package postgres

import "github.com/azrtydxb/momo/dialect"

// Dialect is the PostgreSQL implementation of migrate.Dialect. It holds no
// state; every method works on the pinned connection it is given.
type Dialect struct{}

// New returns the PostgreSQL dialect.
func New() Dialect { return Dialect{} }

// Dialect describes PostgreSQL: identifier quoting, placeholders and types.
func (Dialect) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }
