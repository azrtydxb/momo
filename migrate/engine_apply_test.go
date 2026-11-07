// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"errors"
	"sync"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// recorder wraps the PostgreSQL dialect and logs the order of reset and unlock.
type recorder struct {
	inner    mpg.Dialect
	mu       *sync.Mutex
	log      *[]string
	resetErr error
	locked   chan struct{} // receives a value each time the lock was found held
}

func (r recorder) Dialect() *dialect.Dialect { return r.inner.Dialect() }

func (r recorder) Inspect(ctx context.Context, c momo.PinnedConn, o migrate.InspectOptions) (*schema.Schema, error) {
	return r.inner.Inspect(ctx, c, o)
}

func (r recorder) Render(p []migrate.Change) ([]migrate.Statement, error) { return r.inner.Render(p) }

func (r recorder) HistoryDDL() []string { return r.inner.HistoryDDL() }

func (r recorder) note(s string) { r.mu.Lock(); *r.log = append(*r.log, s); r.mu.Unlock() }

func (r recorder) Lock(ctx context.Context, c momo.PinnedConn, key string) (func() error, func(context.Context) (string, error), error) {
	unlock, holder, err := r.inner.Lock(ctx, c, key)
	if err != nil {
		if r.locked != nil {
			select {
			case r.locked <- struct{}{}:
			default:
			}
		}
		return unlock, holder, err
	}
	return func() error { r.note("unlock"); return unlock() }, holder, nil
}

func (r recorder) Setup(ctx context.Context, c momo.PinnedConn, s migrate.SessionOptions) (func() error, error) {
	reset, err := r.inner.Setup(ctx, c, s)
	if err != nil {
		return nil, err
	}
	return func() error { r.note("reset"); return errors.Join(reset(), r.resetErr) }, nil
}
