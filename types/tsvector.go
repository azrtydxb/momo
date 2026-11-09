// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql/driver"

	"github.com/azrtydxb/momo/dialect"
)

// TSVector is a PostgreSQL full-text document: tsvector on PostgreSQL, TEXT
// on SQLite (which has no full-text search through Momo). Its value is the
// text form PostgreSQL prints, for example 'cat':3 'fat':2, with the
// lexemes sorted and normalized by the server.
//
// A column of this type is usually maintained by a trigger declared through
// MomoConfig and is read-only for the application; the full-text expression
// builders (fts.Match, fts.ToVector, fts.RankCD, ...) take it as an operand.
type TSVector string

// SQLType returns the column type for d.
func (TSVector) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "tsvector"
	}
	return "TEXT"
}

// Value returns the tsvector text form.
func (v TSVector) Value() (driver.Value, error) { return string(v), nil }

// Scan accepts the tsvector text form as string or []byte. Scan(nil) returns
// an error: use a *TSVector for a nullable column.
func (v *TSVector) Scan(src any) error {
	s, err := text("TSVector", src)
	if err != nil {
		return err
	}
	*v = TSVector(s)
	return nil
}

// TSQuery is a PostgreSQL full-text query: tsquery on PostgreSQL, TEXT on
// SQLite. It is mostly the result type of the query builders (fts.ToQuery,
// fts.WebsearchToQuery, ...), but a column can hold one too (stored searches).
type TSQuery string

// SQLType returns the column type for d.
func (TSQuery) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "tsquery"
	}
	return "TEXT"
}

// Value returns the tsquery text form.
func (q TSQuery) Value() (driver.Value, error) { return string(q), nil }

// Scan accepts the tsquery text form as string or []byte. Scan(nil) returns
// an error: use a *TSQuery for a nullable column.
func (q *TSQuery) Scan(src any) error {
	s, err := text("TSQuery", src)
	if err != nil {
		return err
	}
	*q = TSQuery(s)
	return nil
}
