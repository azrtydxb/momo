// SPDX-License-Identifier: Apache-2.0

package momo

import (
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
	// bumped records that resolveUpsert has already checked the DO UPDATE
	// list and appended updated_at, so a resolved copy is not checked again
	// (the appended assignment would look like an explicit one).
	bumped bool
	// unnest renders a Models insert as one statement with an array parameter
	// per column (Unnest, PostgreSQL only).
	unnest bool
	// noRowDefault is set by Copy: a COPY row cannot say DEFAULT and Copy
	// reads nothing back, so an unset dbdefault value is refused.
	noRowDefault bool
	// fixedCols, when set by plan, is the column list every chunk of a bulk
	// insert writes, so the values computed once for the whole set line up
	// with each chunk even when a chunk alone would drop a dbdefault column.
	fixedCols []int
	// sel and selCols are the row source and target columns of FromSelect.
	sel     Subquery
	selCols []string
	// with holds the WITH clause.
	with []cte
	err  error
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
	// back holds, per model, the caller's value that generated ids are
	// written back to (nil for none); see generateIDs.
	back []*T
	// idsDone marks that generateIDs has run, and reconcile that Exec reads
	// the stored ids back after a conflict that may keep an existing row.
	idsDone, reconcile bool
	// pend collects the write-backs of execReconcile, applied once the
	// statement (all chunks) has committed.
	pend *[]func()
	// clock, when set, is the statement time shared by every chunk of a
	// chunked bulk insert; otherwise the DB clock is read per statement.
	clock time.Time
	// pre holds the model values plan computed and validated, one row per
	// model for the planned columns, so chunks bind exactly what was checked
	// and a Go-side default is evaluated once per row.
	pre [][]any
	// dbd is the dbdefault plan; see withDBD.
	dbd *dbdPlan
}

// Insert starts an INSERT for model T. Use Model or Models to write whole
// models, or Values to write explicit columns.
func Insert[T any, PT Modeler[T]](q Querier) InsertQuery[T] {
	return InsertQuery[T]{q: q, meta: metaOf[T, PT]()}
}

// With prefixes the INSERT with a common table expression that FromSelect's
// select can read: WITH x AS (...) INSERT INTO t (cols) SELECT ... FROM x.
// cols optionally names the CTE's columns. The body may be a SelectQuery, an
// ExprSelect, a Union or (PostgreSQL only) another INSERT, UPDATE or DELETE
// with RETURNING; a data-modifying body on another dialect fails with
// ErrUnsupported. MySQL and MariaDB do not accept WITH before INSERT and fail
// with ErrUnsupported; put the WITH inside the select there. A WITH clause
// cannot be combined with a bulk insert that is split into chunks at the
// parameter limit (ErrValidation), since it would run once per chunk.
func (q InsertQuery[T]) With(name string, body Subquery, cols ...string) InsertQuery[T] {
	q.i.with = addCTE(q.i.with, name, body, cols, false)
	return q
}

// WithRecursive is With for a recursive CTE.
func (q InsertQuery[T]) WithRecursive(name string, body Subquery, cols ...string) InsertQuery[T] {
	q.i.with = addCTE(q.i.with, name, body, cols, true)
	return q
}

func (q InsertQuery[T]) fail(err error) InsertQuery[T] {
	if q.i.err == nil {
		q.i.err = err
	}
	return q
}

func (q InsertQuery[T]) verr(col, format string, args ...any) error {
	e := validation("insert", q.meta.Table, format, args...)
	e.Column = col
	return e
}

// Model adds one model, writing every column except identity, read-only and
// omitted columns. Zero values are written, never skipped, except in a column
// tagged dbdefault: when that field holds its zero value (or a nil pointer)
// the column is left out of the INSERT so the database default applies, and
// Exec reads the stored value back into the model with RETURNING. See Models
// for several rows and the dialects that cannot do this.
func (q InsertQuery[T]) Model(m *T) InsertQuery[T] {
	if m == nil {
		return q.fail(q.verr("", "Model(nil)"))
	}
	q.models = append(slices.Clip(q.models), *m)
	q.back = append(slices.Clip(q.back), m)
	return q
}

// Models adds every element of ms as one row of a multi-row VALUES list.
// With an empty list the query fails with ErrValidation.
//
// A dbdefault column that is zero in every row leaves the column list; when
// some rows set it and others do not, the unset rows write DEFAULT. SQLite
// and Unnest cannot put DEFAULT in a row and fail with ErrUnsupported for such
// a mix; MySQL has no RETURNING to read the value back and fails with
// ErrUnsupported when a dbdefault column is zero. Exec writes the stored
// values of the unset rows back into the caller's elements after the
// statement committed. A single Model that a conflict action skips reports 0
// rows affected and stays unchanged. A bulk insert that returns fewer rows than
// models is matched by key (conflict target, else primary key) when the key is
// written from the model and not a zero dbdefault column; unmatched models stay
// untouched, and Exec fails only when DoNothing left a dbdefault column
// unread. Without a usable key a short result fails with ErrUnsupported and
// writes nothing back; the insert is rolled back (to a savepoint inside a Tx;
// a pinned OnConn handle keeps the rows and says so). A caller's own Returning list
// turns the automatic read-back off: Rows returns the columns it names.
func (q InsertQuery[T]) Models(ms []T) InsertQuery[T] {
	if len(ms) == 0 {
		return q.fail(q.verr("", "Models: no rows"))
	}
	q.models = append(slices.Clip(q.models), ms...)
	for i := range ms {
		q.back = append(slices.Clip(q.back), &ms[i])
	}
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
//
// When the model has an updated_at column the conflict branch also sets it to
// the statement clock, as Update does, whatever the INSERT carries (Omit and a
// FromSelect without the column affect only the insert branch). An explicit
// assignment to updated_at is refused with ErrValidation ("cannot be set
// directly"), as on Update. A plain Insert with DoUpdate() and no assignment
// stays an error ("needs at least one assignment"); Upsert renders a
// touch-only SET updated_at instead when its default set is empty.
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
		return Compiled{}, noQuerier("insert", q.meta.Table)
	}
	return compile(q.q.Dialect(), q.render)
}
