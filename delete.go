// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
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

// Model deletes the row identified by m's primary key.
func (d DeleteQuery[T]) Model(m *T) DeleteQuery[T] { d.model, d.modelSet = m, true; return d }

// AllRows confirms that a DELETE without WHERE is intended.
func (d DeleteQuery[T]) AllRows() DeleteQuery[T] { d.allRows = true; return d }

func (d DeleteQuery[T]) fail(kind error, err error) error {
	return &Error{Kind: kind, Op: "delete", Table: d.meta.Table, Err: err}
}

// plan resolves the WHERE predicates and applies the guardrail.
func (d DeleteQuery[T]) plan() ([]Predicate, error) {
	if d.meta.Table == "" {
		return nil, d.fail(ErrValidation, errors.New("model has no table"))
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
			pks = append(pks, updPKEq{d.meta.Table, d.meta.Columns[i].Name, d.meta.Value(d.model, i)})
		}
		where = append(pks, where...)
	}
	if !hasEffectiveFilter(where) && !d.allRows {
		return nil, d.fail(ErrUnfilteredMutation, nil)
	}
	return where, nil
}

// SQL renders the statement without executing it. Guardrail violations are
// reported here, before anything can reach the database.
func (d DeleteQuery[T]) SQL() (Compiled, error) {
	if d.q == nil {
		return Compiled{}, errors.New("momo: query has no Querier")
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
		})
	})
}

// Exec runs the DELETE.
func (d DeleteQuery[T]) Exec(ctx context.Context) (Result, error) {
	c, err := d.SQL()
	if err != nil {
		return Result{}, err
	}
	n, err := d.q.conn().exec(ctx, "delete", d.meta.Table, c)
	if err != nil {
		return Result{}, err
	}
	return Result{RowsAffected: n}, nil
}
