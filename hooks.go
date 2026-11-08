// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"slices"
)

// HookSet holds the typed model hooks of Meta[T]. Generated code or
// hand-written metadata sets the fields; the builders call them with the
// statement's context.
//
// A hook runs only for the builder kind it names and only when it is set:
//   - BeforeInsert runs for each model of an Insert (Model, Models, Upsert)
//     before the statement is built. The hook receives a copy of the model,
//     so it may change the values that are written without touching the
//     caller's value.
//   - BeforeUpdate runs for a Model update, on the caller's model.
//   - BeforeDelete runs for a Model delete and a Model SoftDelete, on the
//     caller's model.
//   - AfterLoad runs for every model returned by All, One, First, Each and
//     Iter, and for every model a write scans from its RETURNING rows
//     (ModelRows.ScanModel), after the row is scanned.
//
// A returned error aborts the statement before any SQL runs (a Before hook)
// or ends the read (AfterLoad); it is wrapped in an *Error of kind
// ErrValidation that still reaches the hook's own error with errors.Is. A
// hook may set values through the *T pointer but cannot change the SQL. Hooks
// run on the execution methods (Exec, Rows, All and so on); SQL does not run
// them. The call path performs no reflection and no allocation.
//
// Relations loaded with Load or Preload and joined relations are scanned by
// the relation machinery and do not run the AfterLoad hook of the related
// type.
type HookSet[T any] struct {
	BeforeInsert func(ctx context.Context, m *T) error
	BeforeUpdate func(ctx context.Context, m *T) error
	BeforeDelete func(ctx context.Context, m *T) error
	AfterLoad    func(ctx context.Context, m *T) error
}

// runModelHook calls fn for m and wraps its error. It is a no-op without a hook.
func runModelHook[T any](ctx context.Context, op, table string, fn func(context.Context, *T) error, m *T) error {
	if fn == nil || m == nil {
		return nil
	}
	if err := fn(ctx, m); err != nil {
		return &Error{Kind: ErrValidation, Op: op, Table: table, Err: err}
	}
	return nil
}

// afterLoad runs the AfterLoad hook of meta for a scanned model.
func afterLoad[T any](ctx context.Context, meta *Meta[T], m *T) error {
	fn := meta.Hooks.AfterLoad
	if fn == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return runModelHook(ctx, "select", meta.Table, fn, m)
}

// beforeWrite runs BeforeInsert over a copy of the query's models and the
// insert interceptors. It returns the query to execute.
func (q InsertQuery[T]) beforeWrite(ctx context.Context) (InsertQuery[T], error) {
	if q.i.err != nil {
		return q, nil
	}
	if fn := q.meta.Hooks.BeforeInsert; fn != nil && len(q.models) > 0 {
		ms := slices.Clone(q.models)
		for i := range ms {
			if err := runModelHook(ctx, "insert", q.meta.Table, fn, &ms[i]); err != nil {
				return q, err
			}
		}
		q.models = ms
	}
	if _, err := runInterceptors(ctx, q.q, "insert", q.meta.Table); err != nil {
		return q, err
	}
	return q, nil
}

// beforeWrite runs BeforeUpdate (BeforeDelete for SoftDelete) on the model.
func (u UpdateQuery[T]) beforeWrite(ctx context.Context) error {
	if u.model == nil {
		return nil
	}
	if u.softDelete {
		return runModelHook(ctx, "delete", u.meta.Table, u.meta.Hooks.BeforeDelete, u.model)
	}
	return runModelHook(ctx, "update", u.meta.Table, u.meta.Hooks.BeforeUpdate, u.model)
}

// beforeWrite runs BeforeDelete on the model.
func (d DeleteQuery[T]) beforeWrite(ctx context.Context) error {
	return runModelHook(ctx, "delete", d.meta.Table, d.meta.Hooks.BeforeDelete, d.model)
}
