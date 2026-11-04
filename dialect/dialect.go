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
	Postgres Name = "postgres"
	SQLite   Name = "sqlite"
)

// Dialect is immutable after construction and safe for concurrent use.
type Dialect struct {
	Name Name

	// MaxParams is the largest number of bind parameters a single statement
	// may carry. PostgreSQL's wire protocol limit is 65535; SQLite's
	// default SQLITE_MAX_VARIABLE_NUMBER is 32766 since 3.32.0.
	MaxParams int

	Returning        bool // INSERT/UPDATE/DELETE ... RETURNING (SQLite >= 3.35)
	OnConflict       bool // INSERT ... ON CONFLICT
	Locking          bool // SELECT ... FOR UPDATE / FOR SHARE / SKIP LOCKED / NOWAIT
	NullsOrdering    bool // ORDER BY ... NULLS FIRST|LAST (SQLite >= 3.30)
	DefaultInUpdate  bool // UPDATE ... SET col = DEFAULT
	DefaultInValues  bool // INSERT ... VALUES (DEFAULT)
	RecursiveCTE     bool
	RowValues        bool // (a, b) IN ((?, ?), ...) and (a, b) > (?, ?)
	Savepoints       bool
	IsolationLevels  bool // BEGIN with an isolation level
	TransactionalDDL bool // DDL can be rolled back
	ILike            bool
}

// For returns the built-in dialect description for name.
func For(n Name) *Dialect {
	switch n {
	case Postgres:
		return &postgres
	case SQLite:
		return &sqlite
	}
	return nil
}

var postgres = Dialect{
	Name: Postgres, MaxParams: 65535,
	Returning: true, OnConflict: true, Locking: true, NullsOrdering: true,
	DefaultInUpdate: true, DefaultInValues: true, RecursiveCTE: true,
	RowValues: true, Savepoints: true, IsolationLevels: true,
	TransactionalDDL: true, ILike: true,
}

// SQLite capabilities assume SQLite >= 3.35 (RETURNING). Momo's SQLite
// adapter verifies the library version at connection time.
var sqlite = Dialect{
	Name: SQLite, MaxParams: 32766,
	Returning: true, OnConflict: true, NullsOrdering: true,
	RecursiveCTE: true, RowValues: true, Savepoints: true,
	TransactionalDDL: true,
}

// AppendPlaceholder appends the n-th (1-based) bind placeholder.
func (d *Dialect) AppendPlaceholder(b []byte, n int) []byte {
	if d.Name == Postgres {
		return strconv.AppendInt(append(b, '$'), int64(n), 10)
	}
	return append(b, '?')
}

// AppendIdent appends a quoted identifier. Both dialects use the SQL
// standard double-quote form; embedded quotes are doubled. Dotted names are
// NOT split: callers quote each part separately.
func (d *Dialect) AppendIdent(b []byte, ident string) []byte {
	b = append(b, '"')
	if strings.IndexByte(ident, '"') < 0 {
		b = append(b, ident...)
	} else {
		b = append(b, strings.ReplaceAll(ident, `"`, `""`)...)
	}
	return append(b, '"')
}

// QuoteIdent returns a quoted identifier.
func (d *Dialect) QuoteIdent(ident string) string { return string(d.AppendIdent(nil, ident)) }
