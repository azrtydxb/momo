// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"slices"
)

// Operation is what an interceptor sees and may change before a typed
// statement compiles.
type Operation struct {
	// Kind is "select", "insert", "update" or "delete". A SoftDelete is a
	// "delete".
	Kind  string
	Table string
	// Where starts empty. An interceptor appends predicates to it; they are
	// ANDed to the predicates of a select, update or delete and are rendered
	// in the compiled SQL. Predicates an insert interceptor appends are
	// ignored. They do not count as a caller filter, so an update or delete
	// without a Where of its own is still refused with ErrUnfilteredMutation.
	Where []Predicate
}

// Interceptor may append predicates or return an error to deny the
// statement. It runs before SQL is built, in registration order. It is
// attached per handle with WithInterceptor and inherited by transactions
// begun from that handle only.
//
// Interceptors cover the typed statements of the handle: Select (All, One,
// First, Each, Iter, Count, Explain), Insert, Update, SoftDelete and Delete.
// They do not cover raw SQL (Exec, Scalars), relation loading (Load,
// Preload, Joined), the statements of the association helpers, or a
// subquery rendered inside another statement. An interceptor is an
// application-level guard, not a database security boundary; see
// docs/PRIVACY_RULES.md.
type Interceptor func(ctx context.Context, op *Operation) error

// WithInterceptor attaches i to the handle. Several calls accumulate and run
// in order. A handle made with New from the same driver does not inherit it.
func WithInterceptor(i Interceptor) Option {
	return func(db *DB) {
		if i != nil {
			db.interceptors = append(db.interceptors, i)
		}
	}
}

// runInterceptors runs the handle's interceptors for one statement and
// returns the predicates they added. A denial is returned as an *Error of
// kind ErrValidation that wraps the interceptor's error.
func runInterceptors(ctx context.Context, q Querier, kind, table string) ([]Predicate, error) {
	if q == nil {
		return nil, nil
	}
	e := q.conn()
	if e == nil || e.db == nil || len(e.db.interceptors) == 0 {
		return nil, nil
	}
	op := Operation{Kind: kind, Table: table}
	for _, i := range e.db.interceptors {
		if err := i(ctx, &op); err != nil {
			return nil, &Error{Kind: ErrValidation, Op: kind, Table: table, Err: err}
		}
	}
	return op.Where, nil
}

// intercept returns sq with the interceptors' predicates added to its WHERE.
func (sq SelectQuery[T]) intercept(ctx context.Context) (SelectQuery[T], error) {
	extra, err := runInterceptors(ctx, sq.q, "select", sq.meta.Table)
	if err != nil {
		return sq, err
	}
	if len(extra) > 0 {
		sq.s.where = append(slices.Clone(sq.s.where), extra...)
	}
	return sq, nil
}
