// SPDX-License-Identifier: Apache-2.0

// Package histtest is the test support the history readers share: a pinned
// connection that fails every query, and a check that a reader hands its
// context to that query.
package histtest

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

type ctxKey struct{}

// cancelRows is a result set whose context is cancelled while it is read: the
// first Next cancels and ends the iteration, and Err reports the context's
// error, as a driver does when its query is cancelled mid-stream.
type cancelRows struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (r *cancelRows) Next() bool {
	r.cancel()
	return false
}
func (*cancelRows) Scan(...any) error { return errors.New("histtest: no row to scan") }
func (r *cancelRows) Err() error      { return r.ctx.Err() }
func (*cancelRows) Close() error      { return nil }

// midStreamConn succeeds the query and hands back cancelRows.
type midStreamConn struct {
	conn
	cancel context.CancelFunc
}

func (c *midStreamConn) Query(ctx context.Context, _ string, _ []any) (momo.Rows, error) {
	return &cancelRows{ctx: ctx, cancel: c.cancel}, nil
}

var errReached = errors.New("histtest: the query was reached")

// conn records the context of the last query and fails it: with the context's
// error when it is done, with errReached otherwise.
type conn struct{ got context.Context }

func (c *conn) Query(ctx context.Context, _ string, _ []any) (momo.Rows, error) {
	c.got = ctx
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errReached
}

func (*conn) Exec(context.Context, string, []any) (int64, error) { return 0, errReached }

func (*conn) Begin(context.Context, momo.TxOptions) (momo.DriverTx, error) {
	return nil, errReached
}

func (*conn) Release() error { return nil }

// CheckReadContext proves read passes its context to the query it runs: a
// cancelled context surfaces as context.Canceled, a value on the context
// reaches the connection, and a context cancelled while the rows are read
// surfaces as context.Canceled through the rows' error.
func CheckReadContext(t *testing.T, read func(ctx context.Context, c momo.PinnedConn) error) {
	t.Helper()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := read(cancelled, &conn{}); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context: err = %v, want context.Canceled", err)
	}
	c := &conn{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	if err := read(ctx, c); !errors.Is(err, errReached) {
		t.Errorf("a live context: err = %v, want the query to be reached", err)
	}
	if c.got == nil || c.got.Value(ctxKey{}) != "marker" {
		t.Errorf("the query did not receive the caller's context")
	}
	mid, midCancel := context.WithCancel(context.Background())
	defer midCancel()
	if err := read(mid, &midStreamConn{cancel: midCancel}); !errors.Is(err, context.Canceled) {
		t.Errorf("a context cancelled while reading rows: err = %v, want context.Canceled", err)
	}
}
