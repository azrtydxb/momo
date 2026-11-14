// SPDX-License-Identifier: Apache-2.0

// Package dialect describes the SQL language differences between the
// databases Momo supports. A Dialect says what the SQL *language* can
// express; what a particular connection driver can do (COPY, batching,
// extension loading) is a separate, driver-level capability.
package dialect

import (
	"strconv"
	"strings"
)

// Name identifies a dialect.
type Name string

const (
	// Postgres names the PostgreSQL dialect.
	Postgres Name = "postgres"
	// SQLite names the SQLite dialect.
	SQLite Name = "sqlite"
	// MySQL names the MySQL and MariaDB dialect.
	MySQL Name = "mysql"
)

// Flavor distinguishes servers that share the MySQL dialect. It is set on
// MySQL dialects only.
type Flavor string

const (
	// FlavorMySQL is Oracle MySQL.
	FlavorMySQL Flavor = "mysql"
	// FlavorMariaDB is MariaDB.
	FlavorMariaDB Flavor = "mariadb"
)

// Dialect is immutable after construction and safe for concurrent use.
type Dialect struct {
	Name Name

	// Flavor is set on MySQL dialects only.
	Flavor Flavor

	// MaxParams is the largest number of bind parameters a single statement
	// may carry. PostgreSQL's wire protocol limit is 65535; SQLite's
	// default SQLITE_MAX_VARIABLE_NUMBER is 32766 since 3.32.0.
	MaxParams int

	// MaxInList caps the values in one typed IN or NOT IN list. Zero means no
	// cap. momo.WithMaxInList sets it on the per-DB copy of the dialect.
	MaxInList int

	Returning        bool // INSERT/UPDATE/DELETE ... RETURNING (SQLite >= 3.35)
	InsertReturning  bool // INSERT ... RETURNING only (MariaDB >= 10.5); Returning stays false there
	OnConflict       bool // INSERT ... ON CONFLICT
	OnDuplicateKey   bool // INSERT ... ON DUPLICATE KEY UPDATE
	Locking          bool // SELECT ... FOR UPDATE / FOR SHARE / SKIP LOCKED / NOWAIT
	NullsOrdering    bool // native ORDER BY ... NULLS FIRST|LAST (SQLite >= 3.30); MySQL emulates it with an IS NULL sort key
	DefaultInUpdate  bool // UPDATE ... SET col = DEFAULT
	DefaultInValues  bool // INSERT ... VALUES (DEFAULT)
	RecursiveCTE     bool
	RowValues        bool // (a, b) IN ((?, ?), ...) and (a, b) > (?, ?)
	Savepoints       bool
	IsolationLevels  bool // BEGIN with an isolation level
	TransactionalDDL bool // DDL can be rolled back
	ILike            bool // native ILIKE; SQLite and MySQL render LOWER(x) LIKE LOWER(?)
	Merge            bool // MERGE statement
	WindowFunctions  bool

	// Vector describes vector storage, distance metrics and ANN indexes.
	Vector VectorCaps
}

// For returns the built-in dialect description for name.
func For(n Name) *Dialect {
	switch n {
	case Postgres:
		return &postgres
	case SQLite:
		return &sqlite
	case MySQL:
		return MySQLFlavor(FlavorMySQL)
	}
	return nil
}

var postgres = Dialect{
	Name: Postgres, MaxParams: 65535,
	Returning: true, OnConflict: true, Locking: true, NullsOrdering: true,
	DefaultInUpdate: true, DefaultInValues: true, RecursiveCTE: true,
	RowValues: true, Savepoints: true, IsolationLevels: true,
	TransactionalDDL: true, ILike: true, Merge: true, WindowFunctions: true,
	Vector: VectorCaps{
		Storage: true,
		Metrics: []Metric{MetricL2, MetricInnerProduct, MetricCosine, MetricL1, MetricHamming, MetricJaccard},
		Indexes: []string{"hnsw", "ivfflat"},
	},
}

// SQLite capabilities assume SQLite >= 3.35 (RETURNING). Momo's SQLite
// adapter verifies the library version at connection time.
var sqlite = Dialect{
	Name: SQLite, MaxParams: 32766,
	Returning: true, OnConflict: true, NullsOrdering: true,
	RecursiveCTE: true, RowValues: true, Savepoints: true,
	TransactionalDDL: true, WindowFunctions: true, // no DEFAULT inside VALUES
	// The metric list is the set of scalar distance functions registered by
	// sqlite-vec v0.1.6 (aFunc[] in cgo/sqlite-vec.c of
	// github.com/asg017/sqlite-vec-go-bindings/cgo, read 2026-11-04). It has
	// no inner-product or Jaccard function and no ANN index.
	Vector: VectorCaps{
		Storage: true,
		Metrics: []Metric{MetricL2, MetricL1, MetricCosine, MetricHamming},
	},
}

// AppendPlaceholder appends the n-th (1-based) bind placeholder.
func (d *Dialect) AppendPlaceholder(b []byte, n int) []byte {
	if d.Name == Postgres {
		return strconv.AppendInt(append(b, '$'), int64(n), 10)
	}
	return append(b, '?')
}

// AppendIdent appends a quoted identifier. MySQL uses backticks; the other
// dialects use the SQL standard double-quote form. Embedded quote characters
// are doubled. Dotted names are NOT split: callers quote each part separately.
func (d *Dialect) AppendIdent(b []byte, ident string) []byte {
	q := byte('"')
	if d.Name == MySQL {
		q = '`'
	}
	b = append(b, q)
	b = append(b, strings.ReplaceAll(ident, string(q), string([]byte{q, q}))...)
	return append(b, q)
}

// QuoteIdent returns a quoted identifier.
func (d *Dialect) QuoteIdent(ident string) string { return string(d.AppendIdent(nil, ident)) }
