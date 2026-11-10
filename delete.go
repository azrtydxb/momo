// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/azrtydxb/momo/dialect"
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
	// with holds the WITH clause; using the USING source (DELETE ... USING).
	with  []cte
	using TableSource
}

// Delete starts a DELETE from T's table.
func Delete[T any, PT Modeler[T]](q Querier) DeleteQuery[T] {
	return DeleteQuery[T]{q: q, meta: metaOf[T, PT]()}
}

// With prefixes the statement with a common table expression; see
// UpdateQuery.With for the allowed bodies and the dialect rules (MariaDB
// refuses the clause with ErrUnsupported).
func (d DeleteQuery[T]) With(name string, q Subquery, cols ...string) DeleteQuery[T] {
	d.with = addCTE(d.with, name, q, cols, false)
	return d
}

// WithRecursive is With for a recursive CTE.
func (d DeleteQuery[T]) WithRecursive(name string, q Subquery, cols ...string) DeleteQuery[T] {
	d.with = addCTE(d.with, name, q, cols, true)
	return d
}

// Using adds a second source, rendering DELETE FROM t USING src WHERE ...: the
// WHERE may reference src's columns. src is a table, aliased table, CTE or
// FromSubquery, as in a join; join it to the target in Where. Calling Using
// again replaces the source.
//
// PostgreSQL renders DELETE ... USING. MySQL and MariaDB render the
// multi-table form DELETE t FROM t, src WHERE .... SQLite has no such form and
// fails with ErrUnsupported (use a subquery in Where). Soft-deleted rows of
// src are not filtered: add that predicate to Where. Interceptors apply to the
// target table only, not to src.
//
// With a Using source the unfiltered-mutation guard is stricter: at least one
// Where predicate must reference a column of the target table, otherwise the
// statement joins every target row to src and fails with ErrUnfilteredMutation
// unless AllRows confirms it. A raw SQL fragment does not count as such a
// reference.
func (d DeleteQuery[T]) Using(src TableSource) DeleteQuery[T] { d.using = src; return d }

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
	if d.using != nil && !d.allRows && !referencesTable(d.q.Dialect(), where, d.meta.Table) {
		// Without a condition on the target, USING is a cross join and every
		// target row is deleted.
		return nil, d.fail(ErrUnfilteredMutation, errors.New("with Using, Where must reference the target table (or confirm with AllRows)"))
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
	return compile(d.q.Dialect(), d.render)
}

func (DeleteQuery[T]) isDML() {}

// isSubquery lets a DELETE be the body of a WITH clause (PostgreSQL only).
func (DeleteQuery[T]) isSubquery() {}

// AppendSQL renders the statement in parentheses as a WITH clause body. It
// runs the handle's interceptors with a background context, as SQL does, but
// not the beforeWrite checks of Exec. Only PostgreSQL accepts the result.
func (d DeleteQuery[T]) AppendSQL(b *Builder) {
	if d.q == nil {
		b.SetErr(noQuerier("delete", d.meta.Table))
		return
	}
	d, err := d.intercepted(context.Background())
	if err != nil {
		b.SetErr(err)
		return
	}
	b.writeByte('(')
	d.render(b)
	b.writeByte(')')
}

// render writes the DELETE statement; interceptors already ran.
func (d DeleteQuery[T]) render(b *Builder) {
	where, err := d.plan()
	if err != nil {
		b.SetErr(err)
		return
	}
	if d.using != nil && b.d.Name == dialect.SQLite {
		b.unsupported("DELETE ... USING")
		return
	}
	target := TableRef{Name: d.meta.Table, Schema: d.meta.Schema}
	names := append(cteNames(d.with), target.Qualifier())
	if q, ok := d.using.(interface{ Qualifier() string }); ok {
		names = append(names, q.Qualifier())
	}
	mysql := b.d.Name == dialect.MySQL
	// The target table and USING source are the statement's scope, so a
	// correlated subquery in WHERE may reference them.
	b.withScope(names, func() {
		if len(d.with) > 0 && b.d.Flavor == dialect.FlavorMariaDB {
			// Not verified against MariaDB, so refused rather than claimed.
			b.unsupported("WITH before DELETE on MariaDB")
			return
		}
		writeWithClause(b, d.with)
		b.WriteString("DELETE ")
		if d.using != nil && mysql {
			// Multi-table DELETE names the rows' table before FROM.
			b.Ident(target.Qualifier())
			b.writeByte(' ')
		}
		b.WriteString("FROM ")
		target.appendFrom(b)
		if d.using != nil {
			if mysql {
				b.WriteString(", ")
			} else {
				b.WriteString(" USING ")
			}
			d.using.appendFrom(b)
		}
		if len(where) > 0 {
			b.WriteString(" WHERE ")
			appendAnd(b, where)
		}
		renderReturning(b, d.meta, d.returning, func(col string) error {
			return d.fail(ErrValidation, fmt.Errorf("Returning: column %q is not mapped by %s", col, d.meta.Table))
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
