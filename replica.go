// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"sync/atomic"
)

// WithReplicas registers read-replica drivers. Replicas are used only through
// DB.Replica; the primary handle never reads from them.
func WithReplicas(replicas ...Driver) Option {
	return func(db *DB) { db.replicas = append(db.replicas, replicas...) }
}

// Replica returns a read handle whose reads go to replica i, unless the
// context is a unit of work that has written: then reads go to the primary.
// Writes and transactions always go to the primary. It returns ErrUnsupported
// when replica i is not configured.
func (db *DB) Replica(i int) (*DB, error) {
	if i < 0 || i >= len(db.replicas) {
		return nil, &Error{Kind: ErrUnsupported, Op: "replica", Err: errors.New("no such read replica configured")}
	}
	nd := new(DB)
	*nd = *db
	nd.replicas = nil // replicas do not chain
	nd.drv = routingDriver{Driver: db.drv, replica: db.replicas[i]}
	nd.ex = execer{db: nd, q: nd.drv, d: db.ex.d, classify: db.ex.classify, owner: nd}
	return nd, nil
}

type uowKey struct{}
type staleOKKey struct{}

// uowState records whether a write happened in one unit of work.
type uowState struct{ written atomic.Bool }

// NewUnitOfWork returns a context in which every write made through any
// handle is recorded. A Replica handle reads from the primary once a write has
// been recorded in that context, so a request reads its own writes.
func NewUnitOfWork(ctx context.Context) context.Context {
	return context.WithValue(ctx, uowKey{}, &uowState{})
}

// AllowStaleReads returns a context that opts reads of a Replica handle back
// to the replica even after a write in the same unit of work.
func AllowStaleReads(ctx context.Context) context.Context {
	return context.WithValue(ctx, staleOKKey{}, true)
}

// markWrite records a write in the context's unit of work, if it has one.
func markWrite(ctx context.Context) {
	if s, ok := ctx.Value(uowKey{}).(*uowState); ok {
		s.written.Store(true)
	}
}

func uowWritten(ctx context.Context) bool {
	s, ok := ctx.Value(uowKey{}).(*uowState)
	return ok && s.written.Load()
}

func staleOK(ctx context.Context) bool {
	v, _ := ctx.Value(staleOKKey{}).(bool)
	return v
}

// routingDriver sends reads to the replica and everything else to the
// embedded primary.
type routingDriver struct {
	Driver
	replica Driver
}

func (r routingDriver) Query(ctx context.Context, sql string, args []any) (Rows, error) {
	if uowWritten(ctx) && !staleOK(ctx) {
		return r.Driver.Query(ctx, sql, args)
	}
	return r.replica.Query(ctx, sql, args)
}
