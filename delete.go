// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// DeleteQuery builds a DELETE of one model table. It is a value: every
// method returns a copy, so a base query can be reused safely.
//
// A statement without an effective WHERE clause is refused with
// ErrUnfilteredMutation unless AllRows confirms it. An empty And or Or is not
// an effective filter.
type DeleteQuery[T any] struct {
	q    Querier
	meta *Meta[T]
	// where holds the caller's predicates.
	where []Predicate
	model *T
	// modelSet records that Model was called, so Model(nil) is an error and
	// not an unfiltered statement.
	modelSet bool
	allRows  bool
	// force is set by ForceDelete: the hard delete of a soft-delete model.
	force bool
	// returning is the RETURNING list, read with Rows.
	returning returningSet
	// extra holds the predicates interceptors added; plan appends them after
	// the unfiltered-mutation guard, so they never count as a caller filter.
	extra []Predicate
}

// Delete starts a DELETE from T's table.
func Delete[T any, PT Modeler[T]](q Querier) DeleteQuery[T] {
	return DeleteQuery[T]{q: q, meta: metaOf[T, PT]()}
}

// Where adds predicates combined with AND.
func (d DeleteQuery[T]) Where(ps ...Predicate) DeleteQuery[T] {
	d.where = append(slices.Clip(d.where), ps...)
	return d
}

// Model deletes the row identified by m's primary key. The version column of
// a versioned model is not checked: unlike a Model update, Model delete
// removes the row whatever version m carries. Add a Where on the version
// column for a version-checked delete.
func (d DeleteQuery[T]) Model(m *T) DeleteQuery[T] { d.model, d.modelSet = m, true; return d }

// AllRows confirms that a DELETE without WHERE is intended.
func (d DeleteQuery[T]) AllRows() DeleteQuery[T] { d.allRows = true; return d }

// Returning appends a RETURNING list; read the rows with Rows.
func (d DeleteQuery[T]) Returning(cols ...ColumnRef) DeleteQuery[T] {
	d.returning = d.returning.with(cols...)
	return d
}

// ReturningAll returns every column mapped by T, in metadata order.
func (d DeleteQuery[T]) ReturningAll() DeleteQuery[T] {
	d.returning = d.returning.withAll()
	return d
}

func (d DeleteQuery[T]) fail(kind error, err error) error {
	return newError(kind, "delete", d.meta.Table, err)
}

// plan resolves the WHERE predicates and applies the guardrail.
func (d DeleteQuery[T]) plan() ([]Predicate, error) {
	if err := viewWriteErr("delete", d.meta); err != nil {
		return nil, err
	}
	if d.meta.Table == "" {
		return nil, d.fail(ErrValidation, errors.New("model has no table"))
	}
	if d.meta.SoftDelete >= 0 && !d.force {
		return nil, d.fail(ErrValidation, errors.New("model has a soft-delete column: use SoftDelete or ForceDelete"))
	}
	where := slices.Clone(d.where)
	if d.modelSet && d.model == nil {
		return nil, d.fail(ErrValidation, errors.New("Model(nil)"))
	}
	if d.model != nil {
		if len(d.meta.PK) == 0 {
			return nil, d.fail(ErrValidation, errors.New("model has no primary key"))
		}
		pks := make([]Predicate, 0, len(d.meta.PK)+len(where))
		for _, i := range d.meta.PK {
			pks = append(pks, updPKEq{d.meta.Table, d.meta.Columns[i].Name, d.meta.Value(d.model, i), d.meta.Columns[i].Flags})
		}
		where = append(pks, where...)
	}
	if !hasEffectiveFilter(where) && !d.allRows {
		return nil, d.fail(ErrUnfilteredMutation, nil)
	}
	return append(where, d.extra...), nil
}

// SQL renders the statement without executing it. Guardrail violations are
// reported here, before anything can reach the database.
//
// Interceptors run with a background context here; the execution methods pass
// the statement's own context.
func (d DeleteQuery[T]) SQL() (Compiled, error) {
	d, err := d.intercepted(context.Background())
	if err != nil {
		return Compiled{}, err
	}
	return d.compileSQL()
}

// intercepted runs the handle's interceptors with ctx and returns the query
// carrying their predicates.
func (d DeleteQuery[T]) intercepted(ctx context.Context) (DeleteQuery[T], error) {
	extra, err := runInterceptors(ctx, d.q, "delete", d.meta.Table)
	if err != nil {
		return d, err
	}
	d.extra = extra
	return d, nil
}

// compileSQL renders the statement; interceptors already ran (intercepted).
func (d DeleteQuery[T]) compileSQL() (Compiled, error) {
	if d.q == nil {
		return Compiled{}, noQuerier("delete", d.meta.Table)
	}
	where, err := d.plan()
	if err != nil {
		return Compiled{}, err
	}
	target := TableRef{Name: d.meta.Table, Schema: d.meta.Schema}
	return compile(d.q.Dialect(), func(b *Builder) {
		// The target table is the statement's scope, so a correlated
		// subquery in WHERE may reference it.
		b.withScope([]string{target.Qualifier()}, func() {
			b.WriteString("DELETE FROM ")
			target.appendFrom(b)
			if len(where) > 0 {
				b.WriteString(" WHERE ")
				appendAnd(b, where)
			}
			renderReturning(b, d.meta, d.returning, func(col string) error {
				return d.fail(ErrValidation, fmt.Errorf("Returning: column %q is not mapped by %s", col, d.meta.Table))
			})
		})
	})
}

// Exec runs the DELETE.
func (d DeleteQuery[T]) Exec(ctx context.Context) (Result, error) {
	d, err := d.intercepted(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := d.beforeWrite(ctx); err != nil {
		return Result{}, err
	}
	c, err := d.compileSQL()
	if err != nil {
		return Result{}, err
	}
	n, err := d.q.conn().exec(ctx, "delete", d.meta.Table, c)
	if err != nil {
		return Result{}, err
	}
	return Result{RowsAffected: n}, nil
}

// Rows runs the DELETE and returns its RETURNING rows. It fails with
// ErrValidation when no Returning list was set.
func (d DeleteQuery[T]) Rows(ctx context.Context) (*ModelRows[T], error) {
	if !d.returning.active() {
		return nil, d.fail(ErrValidation, errors.New("Rows needs a Returning list"))
	}
	d, err := d.intercepted(ctx)
	if err != nil {
		return nil, err
	}
	if err := d.beforeWrite(ctx); err != nil {
		return nil, err
	}
	c, err := d.compileSQL()
	if err != nil {
		return nil, err
	}
	return runReturning(ctx, d.q, "delete", d.meta, d.returning, c)
}
