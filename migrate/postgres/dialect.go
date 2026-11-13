// SPDX-License-Identifier: Apache-2.0

// Package postgres is the PostgreSQL migration dialect: it reads a live
// database into a schema.Schema through catalog queries.
package postgres

import "github.com/azrtydxb/momo/dialect"

// Dialect is the PostgreSQL implementation of migrate.Dialect. It holds only
// its options; every method works on the pinned connection it is given.
type Dialect struct {
	historySchema string
}

// Option configures New.
type Option func(*Dialect)

// WithHistorySchema pins the momo_migrations history table to schema. The
// table is then looked up as "schema".momo_migrations and not through the
// search_path, and a new one is created in schema, not in current_schema().
// Without it a table of that name in any schema on the search_path, such as
// public, is found and used, and a test schema that only sits in front of
// public never gets its own history or its own tables (momotest sets this
// option). The schema must exist when the history table is created. An empty
// schema keeps the default behaviour.
func WithHistorySchema(schema string) Option {
	return func(d *Dialect) { d.historySchema = schema }
}

// New returns the PostgreSQL dialect.
func New(opts ...Option) Dialect {
	var d Dialect
	for _, o := range opts {
		o(&d)
	}
	return d
}

// Dialect describes PostgreSQL: identifier quoting, placeholders and types.
func (Dialect) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }
