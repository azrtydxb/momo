// SPDX-License-Identifier: Apache-2.0

// Package fakedriver is a database-free momo.Driver used by unit tests. It
// records every statement and replays scripted rows.
package fakedriver

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Call is one recorded statement.
type Call struct {
	SQL  string
	Args []any
}

type state struct {
	mu        sync.Mutex
	calls     []Call
	rows      [][]any
	closed    int
	commits   int
	rollbacks int
}

func (s *state) record(sql string, args []any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, Call{SQL: sql, Args: append([]any(nil), args...)})
}

func (s *state) query(sql string, args []any) momo.Rows {
	s.record(sql, args)
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([][]any, len(s.rows))
	for i, r := range s.rows {
		rows[i] = append([]any(nil), r...)
	}
	return &cursor{s: s, rows: rows, pos: -1}
}

// Driver implements momo.Driver without a database.
type Driver struct {
	d *dialect.Dialect
	s *state
}

var _ momo.Driver = (*Driver)(nil)

// New returns a fake driver for dialect d.
func New(d dialect.Name) *Driver {
	return &Driver{d: dialect.For(d), s: &state{}}
}

// Dialect returns the dialect the driver was created with.
func (d *Driver) Dialect() *dialect.Dialect { return d.d }

// Query records the call and returns a cursor over the scripted rows.
func (d *Driver) Query(_ context.Context, sql string, args []any) (momo.Rows, error) {
	return d.s.query(sql, args), nil
}

// Exec records the call and reports zero affected rows.
func (d *Driver) Exec(_ context.Context, sql string, args []any) (int64, error) {
	d.s.record(sql, args)
	return 0, nil
}

// Begin returns a transaction recording through the same counters.
func (d *Driver) Begin(context.Context, momo.TxOptions) (momo.DriverTx, error) {
	return &tx{s: d.s}, nil
}

// Calls returns a copy of the recorded statements, in order.
func (d *Driver) Calls() []Call {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	return append([]Call(nil), d.s.calls...)
}

// SetRows scripts the rows every subsequent Query returns.
func (d *Driver) SetRows(rows [][]any) {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	d.s.rows = rows
}

// Closed returns the number of Rows.Close calls.
func (d *Driver) Closed() int {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	return d.s.closed
}

// Commits returns the number of transaction commits.
func (d *Driver) Commits() int {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	return d.s.commits
}

// Rollbacks returns the number of transaction rollbacks.
func (d *Driver) Rollbacks() int {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	return d.s.rollbacks
}

type tx struct{ s *state }

var _ momo.DriverTx = (*tx)(nil)

func (t *tx) Query(_ context.Context, sql string, args []any) (momo.Rows, error) {
	return t.s.query(sql, args), nil
}

func (t *tx) Exec(_ context.Context, sql string, args []any) (int64, error) {
	t.s.record(sql, args)
	return 0, nil
}

func (t *tx) Commit(context.Context) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.s.commits++
	return nil
}

func (t *tx) Rollback(context.Context) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.s.rollbacks++
	return nil
}

type cursor struct {
	s    *state
	rows [][]any
	pos  int
}

var _ momo.Rows = (*cursor)(nil)

func (c *cursor) Next() bool {
	if c.pos+1 >= len(c.rows) {
		c.pos = len(c.rows)
		return false
	}
	c.pos++
	return true
}

func (c *cursor) Scan(dest ...any) error {
	if c.pos < 0 || c.pos >= len(c.rows) {
		return fmt.Errorf("fakedriver: Scan without a current row")
	}
	row := c.rows[c.pos]
	if len(dest) != len(row) {
		return fmt.Errorf("fakedriver: Scan got %d destinations for %d columns", len(dest), len(row))
	}
	for i, v := range row {
		dv := reflect.ValueOf(dest[i])
		if dv.Kind() != reflect.Pointer || dv.IsNil() {
			return fmt.Errorf("fakedriver: destination %d is not a non-nil pointer", i)
		}
		elem := dv.Elem()
		if v == nil {
			elem.SetZero()
			continue
		}
		sv := reflect.ValueOf(v)
		if !sv.Type().AssignableTo(elem.Type()) {
			return fmt.Errorf("fakedriver: cannot assign %s to %s in column %d", sv.Type(), elem.Type(), i)
		}
		elem.Set(sv)
	}
	return nil
}

func (c *cursor) Err() error { return nil }

func (c *cursor) Close() error {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	c.s.closed++
	return nil
}
