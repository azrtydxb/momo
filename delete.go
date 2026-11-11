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
	using []TableSource
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
// again replaces the sources.
//
// Using takes more than one source (DELETE FROM t USING a, b WHERE ...), which
// render as a comma list; the guard then also requires Where to reference
// every source (see below). Returning may list columns of a source, as for
// UpdateQuery.From; scan it with ScanDelete.
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
// reference. An Or counts only when every branch references the target, because a
// branch off the target can be true on its own; an And counts when any term does.
// Not swaps the two rules (De Morgan): a negated And needs every term on the
// target, a negated Or any term. JoinSQL with AND or OR follows the same rules
// as And and Or; with another separator it does not count. The guard checks
// that Where references the target, not that it restricts it: a constant-true
// predicate such as t.id = t.id passes. With two or more sources the same rule
// applies to each of them: a source that no Where predicate references is
// cross-joined to the target, and the statement fails with ErrUnfilteredMutation
// unless AllRows confirms it. A single source is not checked this way.
func (d DeleteQuery[T]) Using(srcs ...TableSource) DeleteQuery[T] {
	d.using = slices.Clone(srcs)
	return d
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
	if len(d.using) > 0 && !d.allRows && !referencesTable(d.q.Dialect(), where, d.meta.Table) {
		// Without a condition on the target, USING is a cross join and every
		// target row is deleted.
		return nil, d.fail(ErrUnfilteredMutation, errors.New("with Using, Where must reference the target table (or confirm with AllRows)"))
	}
	if !d.allRows {
		if q := unconstrainedSource(d.q.Dialect(), where, d.using); q != "" {
			return nil, d.fail(ErrUnfilteredMutation, fmt.Errorf("with several Using sources, Where must reference every source, and %q is not referenced (or confirm with AllRows)", q))
		}
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

func (d DeleteQuery[T]) hasReturning() bool { return d.returning.active() }

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
	if len(d.using) > 0 && b.d.Name == dialect.SQLite {
		b.unsupported("DELETE ... USING")
		return
	}
	target := TableRef{Name: d.meta.Table, Schema: d.meta.Schema}
	names := append(cteNames(d.with), target.Qualifier())
	names = append(names, sourceQualifiers(d.using)...)
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
		if len(d.using) > 0 && mysql {
			// Multi-table DELETE names the rows' table before FROM.
			b.Ident(target.Qualifier())
			b.writeByte(' ')
		}
		b.WriteString("FROM ")
		target.appendFrom(b)
		if len(d.using) > 0 {
			if mysql {
				b.WriteString(", ")
			} else {
				b.WriteString(" USING ")
			}
			appendSources(b, d.using)
		}
		if len(where) > 0 {
			b.WriteString(" WHERE ")
			appendAnd(b, where)
		}
		renderWriteReturning(b, d.meta, d.returning, len(d.using) > 0, func(col string) error {
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
	if d.returning.hasSource(d.meta.Table) {
		return nil, d.fail(ErrValidation, errors.New("Rows cannot scan Returning columns of a Using source into the model; use ScanDelete"))
	}
	d, c, err := d.prepareReturning(ctx, "Rows")
	if err != nil {
		return nil, err
	}
	return runReturning(ctx, d.q, "delete", d.meta, d.returning, c)
}

// prepareReturning runs the steps a RETURNING statement shares before it
// reaches the database; caller names the method in errors.
func (d DeleteQuery[T]) prepareReturning(ctx context.Context, caller string) (DeleteQuery[T], Compiled, error) {
	if !d.returning.active() {
		return d, Compiled{}, d.fail(ErrValidation, errors.New(caller+" needs a Returning list"))
	}
	d, err := d.intercepted(ctx)
	if err != nil {
		return d, Compiled{}, err
	}
	if err := d.beforeWrite(ctx); err != nil {
		return d, Compiled{}, err
	}
	c, err := d.compileSQL()
	return d, c, err
}

// ScanDelete runs the DELETE and scans every RETURNING row into an R, as
// ScanUpdate does for an UPDATE: use it when Returning lists columns of a
// Using source or CTE. R is a projection, a tuple row or a plain struct,
// matched by result column name.
func ScanDelete[R, T any](ctx context.Context, d DeleteQuery[T]) ([]R, error) {
	d, c, err := d.prepareReturning(ctx, "ScanDelete")
	if err != nil {
		return nil, err
	}
	return scanCompiled[R](ctx, d.q, "delete", d.meta.Table, c)
}
