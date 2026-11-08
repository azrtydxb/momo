// SPDX-License-Identifier: Apache-2.0

// Package sqlstub opens a database/sql pool over a stub driver that answers
// every query with one row holding one fixed value. Unit tests use it to get a
// real *sql.Rows without a database server.
package sqlstub

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
)

// Open returns a pool whose every query yields one row with one column, "c",
// holding v. v must be a valid driver.Value (string, int64, []byte, ...).
func Open(v driver.Value) *sql.DB { return sql.OpenDB(connector{v}) }

type connector struct{ v driver.Value }

func (c connector) Connect(context.Context) (driver.Conn, error) { return conn(c), nil }
func (c connector) Driver() driver.Driver                        { return stubDriver{} }

type stubDriver struct{}

func (stubDriver) Open(string) (driver.Conn, error) { return nil, errors.New("sqlstub: use Open") }

type conn struct{ v driver.Value }

func (c conn) Prepare(string) (driver.Stmt, error) { return stmt(c), nil }
func (conn) Close() error                          { return nil }
func (conn) Begin() (driver.Tx, error)             { return nil, errors.New("sqlstub: no transactions") }

type stmt struct{ v driver.Value }

func (stmt) Close() error  { return nil }
func (stmt) NumInput() int { return -1 }
func (stmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("sqlstub: no exec")
}
func (s stmt) Query([]driver.Value) (driver.Rows, error) { return &rows{v: s.v}, nil }

type rows struct {
	v    driver.Value
	done bool
}

func (*rows) Columns() []string { return []string{"c"} }
func (*rows) Close() error      { return nil }
func (r *rows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.v
	return nil
}
