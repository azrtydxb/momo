// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
)

// OnConn returns a handle whose statements all run on pc, a connection pinned
// with Pinner.Pin. It is a copy of db: it keeps db's hooks, error classifier
// and options (WithHook, WithArgLogging, ...). Begin and Tx on it return
// ErrUnsupported with Op "begin": a pinned connection is not a pool, so
// transactions are not started on it. Non-transactional Go migrations and
// session-scoped work use it. The caller still owns pc and releases it.
func (db *DB) OnConn(pc PinnedConn) *DB {
	nd := new(DB)
	*nd = *db
	nd.drv = pinnedDriver{Driver: db.drv, pc: pc}
	nd.pinned = true
	nd.ex = execer{db: nd, q: pc, d: db.ex.d, classify: db.ex.classify, owner: nd}
	return nd
}

// pinnedDriver is the Driver() of an OnConn handle. Everything that reaches
// the database, including the optional Copier and Batcher paths, goes through
// the pinned connection and never through the pool. Pool-level optional
// interfaces (Pinner, PoolStater) are deliberately not exposed.
type pinnedDriver struct {
	Driver
	pc PinnedConn
}

func (p pinnedDriver) Query(ctx context.Context, sql string, args []any) (Rows, error) {
	return p.pc.Query(ctx, sql, args)
}

func (p pinnedDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	return p.pc.Exec(ctx, sql, args)
}

func (p pinnedDriver) Begin(context.Context, TxOptions) (DriverTx, error) {
	return nil, &Error{Kind: ErrUnsupported, Op: "begin", Err: errors.New("transactions are not available on a pinned connection")}
}

func (p pinnedDriver) ClassifyError(err error) error {
	if c, ok := p.Driver.(ErrorClassifier); ok {
		return c.ClassifyError(err)
	}
	return err
}

func (p pinnedDriver) Capabilities() Capabilities {
	var caps Capabilities
	if c, ok := p.Driver.(CapabilityReporter); ok {
		caps = c.Capabilities()
	}
	_, caps.Copy = p.pc.(Copier)
	_, caps.Batch = p.pc.(Batcher)
	return caps
}

func (p pinnedDriver) CopyFrom(ctx context.Context, table string, cols []string, rows [][]any) (int64, error) {
	c, ok := p.pc.(Copier)
	if !ok {
		return 0, &Error{Kind: ErrUnsupported, Op: "copy", Err: errors.New("the pinned connection has no COPY path")}
	}
	return c.CopyFrom(ctx, table, cols, rows)
}

func (p pinnedDriver) SendBatch(ctx context.Context, stmts []Compiled) ([]Rows, error) {
	b, ok := p.pc.(Batcher)
	if !ok {
		return nil, &Error{Kind: ErrUnsupported, Op: "batch", Err: errors.New("the pinned connection has no batch path")}
	}
	return b.SendBatch(ctx, stmts)
}
