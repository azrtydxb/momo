// SPDX-License-Identifier: Apache-2.0

// Package momo is a typed Go ORM. A generator turns plain Go structs into
// typed column and relation descriptors, and this package builds and runs
// SQL from them: select, insert, update, delete, soft delete, upserts,
// subqueries and CTEs, relations with eager loading, transactions with
// savepoints, and vector search.
//
// Momo supports PostgreSQL 16 and 17 with pgvector through driver/pgxdriver,
// SQLite through driver/sqlitedriver and driver/sqlitepure, and MySQL and
// MariaDB through driver/mysqldriver. The module is pre-v1, with no
// compatibility promise between releases; docs/compatibility.md says what the
// v1 promise will cover.
//
// A program opens a database with New and a driver, then runs queries against
// the DB or a Tx, both of which implement Querier:
//
//	db := momo.New(pgxdriver.New(pool))
//	docs, err := momo.Select[models.Document](db).
//		Where(document.Published.Eq(true)).
//		All(ctx)
//
// Names that start with With follow one rule. A functional option is named
// With followed by what it sets, in every package (momo.WithHook,
// migrate.WithClock, momolog.WithLevel, advisory.WithHash64,
// pgxdriver.WithOnReconnect). The one other accepted shape is a context
// wrapper such as WithTenant and WithQueryScope, which keeps the shape of
// context.WithValue and context.WithCancel: it takes a context.Context and
// returns one. Anything else is named for what it does, so an error decorator
// is Annotate, not With.
//
// The descriptors (document.Published above) come from the momo command,
// which reads the model structs and writes a momo_gen.go file next to them.
// The same command manages versioned SQL migrations.
//
// Documentation for people and for AI coding agents:
//
//   - docs/ai-guide.md is the guide to read first: setup, modeling,
//     migrations, queries, relations, vectors, errors and the known pitfalls.
//   - docs/capabilities.md and docs/supported-versions.md say what is
//     supported and tested.
//   - docs/ARCHITECTURE.md describes the packages and their contracts, and
//     lists what is only planned.
//
// All paths are relative to the module root, https://github.com/azrtydxb/momo.
package momo
