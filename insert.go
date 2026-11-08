// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// InsertValue is an explicit column write accepted by Insert.Values: an
// Assignment, or an InsertAssignment from a create-only or immutable column.
type InsertValue interface{ insertAssignment() Assignment }

func (a Assignment) insertAssignment() Assignment       { return a }
func (a InsertAssignment) insertAssignment() Assignment { return a.a }

type conflictKind uint8

const (
	conflictNone conflictKind = iota
	conflictNothing
	conflictUpdate
)

type insertCore struct {
	assigns   []Assignment
	hasValues bool
	// rows holds the rows of ValuesRows; hasRows marks that call.
	rows      [][]Assignment
	hasRows   bool
	omit      []string
	returning returningSet
	target    []string
	conflict  conflictKind
	set       []Assignment
	// targetWhere is the conflict-target predicate (ON CONFLICT (cols) WHERE
	// ...) and actionWhere the DO UPDATE condition; each ANDs its parts.
	targetWhere []Predicate
	actionWhere []Predicate
	// upsert and withIdentity are owned by upsert.go: upsert marks an Upsert
	// query, withIdentity puts the identity column in a model INSERT list.
	upsert       bool
	withIdentity bool
	// sel and selCols are the row source and target columns of FromSelect.
	sel     Subquery
	selCols []string
	err     error
}

// InsertQuery builds and runs an INSERT into T's table.
//
// Columns come from T's generated metadata only, so no caller-supplied
// string becomes an identifier. Identity columns and FlagReadOnly columns
// are never written, and values are always bound. InsertQuery is an
// immutable value: every method returns a new query.
type InsertQuery[T any] struct {
	q      Querier
	meta   *Meta[T]
	i      insertCore
	models []T
	// ptr is the model of a single Model call, so a generated key can be
	// written back after the insert; nil for any other shape.
	ptr *T
	// clock, when set, is the statement time shared by every chunk of a
	// chunked bulk insert; otherwise the DB clock is read per statement.
	clock time.Time
	// pre holds the model values plan computed and validated, one row per
	// model for the planned columns, so chunks bind exactly what was checked
	// and a Go-side default is evaluated once per row.
	pre [][]any
}

// Insert starts an INSERT for model T. Use Model or Models to write whole
// models, or Values to write explicit columns.
func Insert[T any, PT Modeler[T]](q Querier) InsertQuery[T] {
	return InsertQuery[T]{q: q, meta: metaOf[T, PT]()}
}

func (q InsertQuery[T]) fail(err error) InsertQuery[T] {
	if q.i.err == nil {
		q.i.err = err
	}
	return q
}

func (q InsertQuery[T]) verr(col, format string, args ...any) error {
	return &Error{Kind: ErrValidation, Op: "insert", Table: q.meta.Table, Column: col, Err: fmt.Errorf(format, args...)}
}

// Model adds one model, writing every column except identity, read-only and
// omitted columns. Zero values are written, never skipped.
func (q InsertQuery[T]) Model(m *T) InsertQuery[T] {
	if m == nil {
		return q.fail(q.verr("", "Model(nil)"))
	}
	q.ptr = nil
	if len(q.models) == 0 {
		q.ptr = m
	}
	q.models = append(slices.Clip(q.models), *m)
	return q
}

// Models adds every element of ms as one row of a multi-row VALUES list.
// With an empty list the query fails with ErrValidation.
func (q InsertQuery[T]) Models(ms []T) InsertQuery[T] {
	if len(ms) == 0 {
		return q.fail(q.verr("", "Models: no rows"))
	}
	q.ptr = nil
	q.models = append(slices.Clip(q.models), ms...)
	return q
}

// Values adds one row of explicit column writes. A column that is not
// assigned is omitted from the statement, so its database default applies.
func (q InsertQuery[T]) Values(assigns ...InsertValue) InsertQuery[T] {
	out := slices.Clip(q.i.assigns)
	for _, a := range assigns {
		out = append(out, a.insertAssignment())
	}
	q.i.assigns = out
	q.i.hasValues = true
	return q
}

// ValuesRows adds one row per element to a multi-row VALUES list; each row is
// a list of assignments, like the argument of Values. The column list is the
// union of the columns the rows assign, in order of first appearance. A
// column a row does not assign takes its database default for that row
// (DEFAULT), and SetDefault renders DEFAULT inline. A dialect that cannot put
// DEFAULT inside VALUES (SQLite) omits a column that is default in every row
// and fails with ErrUnsupported when a column is default in some rows and
// given in others. ValuesRows cannot be combined with Values, Model, Models
// or Omit, and a statement over the dialect's bind-parameter limit fails with
// ErrLimitExceeded (DEFAULT and NULL bind no parameter).
func (q InsertQuery[T]) ValuesRows(rows ...[]Assignment) InsertQuery[T] {
	if len(rows) == 0 {
		return q.fail(q.verr("", "ValuesRows: no rows"))
	}
	out := slices.Clip(q.i.rows)
	for _, r := range rows {
		out = append(out, slices.Clone(r))
	}
	q.i.rows = out
	q.i.hasRows = true
	return q
}

// Omit excludes columns from a model insert, leaving their database default.
// Combined with Values or ValuesRows the query fails with ErrValidation: those
// already list exactly the columns they write.
func (q InsertQuery[T]) Omit(cols ...ColumnRef) InsertQuery[T] {
	names := slices.Clip(q.i.omit)
	for _, c := range cols {
		names = append(names, c.ColumnName())
	}
	q.i.omit = names
	return q
}

// Returning appends a RETURNING list; read the rows with Rows.
func (q InsertQuery[T]) Returning(cols ...ColumnRef) InsertQuery[T] {
	q.i.returning = q.i.returning.with(cols...)
	return q
}

// ReturningAll returns every column mapped by T, in metadata order.
func (q InsertQuery[T]) ReturningAll() InsertQuery[T] {
	q.i.returning = q.i.returning.withAll()
	return q
}

// ConflictQuery is an INSERT awaiting its ON CONFLICT action.
type ConflictQuery[T any] struct {
	q      InsertQuery[T]
	target []string
	where  []Predicate
}

// OnConflict starts an ON CONFLICT clause with the given conflict target.
// With no columns the target is omitted, which only DoNothing allows.
func (q InsertQuery[T]) OnConflict(cols ...ColumnRef) ConflictQuery[T] {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.ColumnName()
	}
	return ConflictQuery[T]{q: q, target: names}
}

// Where adds predicates to the conflict target, rendered as
// ON CONFLICT (cols) WHERE <preds>. PostgreSQL and SQLite need it to infer a
// partial unique index: the predicate must imply the index predicate, and
// without it the database rejects the statement. Several calls AND together.
// It needs target columns in OnConflict (otherwise the build fails with
// ErrValidation) and works with both DoNothing and DoUpdate. Columns render
// without a table qualifier, as the database requires in an index
// predicate. On SQLite, bool, integer and string values render inline
// (SQLite matches the predicate against the index text, so a bound value
// never matches), which also puts them in logs and traces. Only bool,
// integer, finite float and string values (after Value()) can be inlined;
// any other value fails with ErrValidation. SQLite compares the literal
// text with the index, so write index literals as Momo renders them:
// integers as 2, floats in plain decimal with .0 for whole numbers (2.0) for magnitudes from
// 1e-6 up to 1e21, and exponent form (1e-07, 1e+21) outside that range;
// otherwise use momo.SQL. Excluded() in the predicate
// fails with ErrValidation. MySQL has no ON CONFLICT and fails with
// ErrUnsupported.
func (c ConflictQuery[T]) Where(preds ...Predicate) ConflictQuery[T] {
	c.where = append(slices.Clip(c.where), preds...)
	return c
}

// DoNothing skips rows that conflict.
func (c ConflictQuery[T]) DoNothing() InsertQuery[T] {
	q := c.q
	q.i.target, q.i.conflict, q.i.set = c.target, conflictNothing, nil
	q.i.targetWhere = slices.Clone(c.where)
	return q
}

// DoUpdate updates the existing row on conflict. Use SetExcluded to take the
// value proposed for insertion.
func (c ConflictQuery[T]) DoUpdate(assigns ...Assignment) InsertQuery[T] {
	q := c.q
	q.i.target, q.i.conflict, q.i.set = c.target, conflictUpdate, slices.Clone(assigns)
	q.i.targetWhere = slices.Clone(c.where)
	return q
}

// Where adds a condition to the DO UPDATE action: the existing row is
// updated only when the predicates hold, rendered as
// DO UPDATE SET ... WHERE <preds>. When they do not, the conflicting row is
// left untouched and the statement affects 0 rows (with Returning, no row
// comes back). Several calls AND together.
//
// Columns are qualified by the table name, so they mean the existing row;
// use Column.Excluded for the value proposed for insertion, for example
// until.LtExpr(until.Excluded()). It is valid only when the conflict action
// is DoUpdate, including Upsert[T], whose default update set is kept; after
// DoNothing, or without any conflict clause, the build fails with
// ErrValidation. MySQL fails with ErrUnsupported.
func (q InsertQuery[T]) Where(preds ...Predicate) InsertQuery[T] {
	q.i.actionWhere = append(slices.Clip(q.i.actionWhere), preds...)
	return q
}

// column returns the index of the mapped column name, or -1.
func (q InsertQuery[T]) column(name string) int {
	for i, c := range q.meta.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}

// writable reports whether the column at i may appear in an INSERT.
func (q InsertQuery[T]) writable(i int) bool {
	return columnAssignable(writeInsert, q.meta.Columns[i].Flags)
}

// SQL compiles the statement without executing it.
func (q InsertQuery[T]) SQL() (Compiled, error) {
	if q.q == nil {
		return Compiled{}, errors.New("momo: query has no Querier")
	}
	return compile(q.q.Dialect(), q.render)
}
