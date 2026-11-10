// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"slices"
)

// Operation is what an interceptor sees and may change before a typed
// statement compiles.
type Operation struct {
	// Kind is "select", "insert", "upsert", "update", "delete" or "truncate".
	// Every write path reports one kind:
	//
	//	Insert (and Copy)                       insert
	//	Insert with OnConflict DoNothing        insert
	//	Insert with OnConflict DoUpdate, Upsert upsert
	//	Update, BulkUpdate                      update
	//	SoftDelete, Delete                      delete
	//	Merge with WhenMatchedUpdate            upsert
	//	Merge without WhenMatchedUpdate         insert
	//	Truncate                                truncate
	//
	// ON CONFLICT DO UPDATE and a matched MERGE can overwrite an existing
	// row, so a tenant rule must deny an "upsert" or constrain it (for
	// example by requiring the tenant column).
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
// An interceptor is an application-level guard, not a database security
// boundary. It covers only the primary table of these typed statements of the
// handle: Select (All, One, First, Each, Iter, Count, Explain), Insert,
// Upsert, Update, BulkUpdate, SoftDelete, Delete, Merge, Truncate and
// InsertQuery.Copy. It does NOT cover: relation loading
// (Load, Preload, LoadCounts); the tables of joined relations (only the
// primary table is filtered); the association helpers (Link, Unlink, Replace
// and the like); subqueries and other sources rendered inside a statement
// (subquery operands, UPDATE ... FROM, the FromSelect source of an insert);
// raw SQL of any kind (Exec, MapRows, Scalars, Query, QueryRows, SQL fragments and RawSubquery);
// the raw driver CopyFrom (InsertQuery.Copy does run the insert interceptors,
// deny only); and anything run on the driver directly. A predicate an
// interceptor adds to an insert, a Copy or a Truncate is ignored, and one added
// to a Merge fails it with ErrValidation. SQL() runs the interceptors with a
// background context for Select, Update (BulkUpdate, SoftDelete), Delete and
// Merge; InsertQuery.SQL (also for an Upsert) runs neither interceptors nor
// hooks. The internal count that tells a stale version from a missing row
// after a version-checked update is a Select, so select interceptors apply to it. See docs/PRIVACY_RULES.md.
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
