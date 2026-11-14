// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"slices"
	"time"

	"github.com/azrtydxb/momo/dialect"
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
	rows    [][]Assignment
	hasRows bool
	omit    []string
	// explicit names the identity, serial or auto-increment columns whose
	// values the statement writes; see Explicit.
	explicit  []string
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
// string becomes an identifier. Identity, serial and auto-increment columns
// and FlagReadOnly columns are never written, and values are always bound.
// InsertQuery is an immutable value: every method returns a new query.
//
// Identity values are dropped silently: a non-zero value in the struct field
// of an identity, serial or auto-increment column is left out of the INSERT
// and the database generates a new one. To keep the value (an import, a
// backfill, a non-key serial such as a receipt number) name the column in
// Explicit. The caller then has to advance the sequence (setval on
// PostgreSQL).
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

// Explicit opts identity, serial and auto-increment columns into the INSERT:
// the named columns are written with the value the source carries (the
// struct field of Model and Models, the assignment of Values and ValuesRows)
// instead of being left to the database.
//
// Without Explicit an Insert never writes such a column. A value in the
// struct field is dropped silently and the database generates a new one, so
// an import or backfill that must keep its original numbers has to call
// Explicit for the column. On PostgreSQL and SQLite a zero value is written
// as-is: naming a column in Explicit says the caller owns it, so a zero field
// inserts 0. On MySQL and MariaDB a zero value in an AUTO_INCREMENT column is
// refused with ErrValidation, because the server would generate a fresh value
// instead of storing 0 (unless NO_AUTO_VALUE_ON_ZERO is set); every row is
// checked before the first statement is sent. Leave the column out of Explicit
// to get a generated value.
//
// On PostgreSQL a GENERATED ALWAYS AS IDENTITY column renders
// OVERRIDING SYSTEM VALUE; serial and GENERATED BY DEFAULT columns need no
// clause. On MySQL, MariaDB and SQLite the value is an ordinary column.
// Explicit works with Model, Models, Values, ValuesRows, Unnest, chunked bulk
// inserts, Upsert and OnConflict. It fails with ErrValidation for a column the
// model does not map, a column that is not identity, serial or auto-increment,
// and with FromSelect or Copy, which cannot carry explicit identity values.
//
// An explicit value does not advance the sequence. On PostgreSQL the caller
// must run setval (or restart the identity) so that later generated values do
// not collide with the inserted ones; Momo never calls setval. MySQL and
// MariaDB AUTO_INCREMENT and SQLite rowid or AUTOINCREMENT move past a larger
// explicit value on their own.
//
// A ColumnRef of another table is ErrValidation, and so is the same column in
// Omit.
//
// With Upsert or OnConflict the explicit column is written by the INSERT
// only. A serial or identity column that is not the conflict-target key is
// never updated on conflict: the default update set skips it, and
// DoUpdate(SetExcluded(col)) for it is refused with ErrValidation.
func (q InsertQuery[T]) Explicit(cols ...ColumnRef) InsertQuery[T] {
	names := slices.Clip(q.i.explicit)
	for _, c := range cols {
		if t, ok := c.(interface{ Table() string }); ok && t.Table() != "" && t.Table() != q.meta.Table {
			return q.fail(q.verr(c.ColumnName(), "Explicit: column %q belongs to table %q, not %s", c.ColumnName(), t.Table(), q.meta.Table))
		}
		names = append(names, c.ColumnName())
	}
	q.i.explicit = names
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
	return columnAssignable(writeInsert, q.meta.Columns[i].Flags) || q.explicitCol(i)
}

// explicitCol reports whether column i is an identity column the caller named
// in Explicit, so the statement writes its value.
func (q InsertQuery[T]) explicitCol(i int) bool {
	if len(q.i.explicit) == 0 || i < 0 || i >= len(q.meta.Columns) {
		return false
	}
	c := q.meta.Columns[i]
	return c.Flags&FlagIdentity != 0 && c.Flags&FlagReadOnly == 0 && slices.Contains(q.i.explicit, c.Name)
}

// checkExplicit validates the Explicit list.
func (q InsertQuery[T]) checkExplicit() error {
	for _, n := range q.i.explicit {
		ci := q.column(n)
		switch {
		case ci < 0:
			return q.verr(n, "Explicit: column %q is not mapped by %s", n, q.meta.Table)
		case slices.Contains(q.i.omit, n):
			return q.verr(n, "Explicit: column %q is also in Omit; use one or the other", n)
		case q.meta.Columns[ci].Flags&FlagIdentity == 0 || q.meta.Columns[ci].Flags&FlagReadOnly != 0:
			return q.verr(n, "Explicit: column %q is not an identity, serial or auto-increment column", n)
		}
	}
	return nil
}

// checkExplicitZero refuses a zero value in an explicit AUTO_INCREMENT column
// on MySQL and MariaDB, where the server would generate a value instead of
// storing 0. It looks at every row so that nothing runs before the refusal.
func (q InsertQuery[T]) checkExplicitZero(d *dialect.Dialect) error {
	if len(q.i.explicit) == 0 || !d.OnDuplicateKey {
		return nil
	}
	refuse := func(name string) error {
		return q.verr(name, "Explicit: column %q holds a zero value and MySQL would generate a value instead of storing 0; leave the column out of Explicit to get a generated one", name)
	}
	for _, n := range q.i.explicit {
		ci := q.column(n)
		if !q.explicitCol(ci) {
			continue
		}
		for r := range q.models {
			if dbDefaultZero(q.meta.Value(&q.models[r], ci)) {
				return refuse(n)
			}
		}
	}
	zeroAssign := func(as []Assignment) error {
		for _, a := range as {
			if a.kind == setValue && slices.Contains(q.i.explicit, a.col) && dbDefaultZero(a.val) && q.explicitCol(q.column(a.col)) {
				return refuse(a.col)
			}
		}
		return nil
	}
	if err := zeroAssign(q.i.assigns); err != nil {
		return err
	}
	for _, row := range q.i.rows {
		if err := zeroAssign(row); err != nil {
			return err
		}
	}
	return nil
}

// identityAlways reports whether column ci is GENERATED ALWAYS AS IDENTITY, so
// PostgreSQL needs OVERRIDING SYSTEM VALUE to accept a value for it. Without a
// table schema it answers true: the clause is harmless on the other kinds.
func (q InsertQuery[T]) identityAlways(ci int) bool {
	if q.meta.TableSchema != nil {
		for _, c := range q.meta.TableSchema.Columns {
			if c.Name == q.meta.Columns[ci].Name {
				return c.Identity == "always"
			}
		}
	}
	return true
}

// overriding reports whether the INSERT needs OVERRIDING SYSTEM VALUE: an
// upsert writes the identity key, or an explicit column is GENERATED ALWAYS.
func (q InsertQuery[T]) overriding(d *dialect.Dialect, cols []int) bool {
	if d.Name != dialect.Postgres {
		return false
	}
	if q.i.withIdentity {
		return true
	}
	for _, ci := range cols {
		if q.explicitCol(ci) && q.identityAlways(ci) {
			return true
		}
	}
	return false
}

// SQL compiles the statement without executing it.
func (q InsertQuery[T]) SQL() (Compiled, error) {
	if q.q == nil {
		return Compiled{}, noQuerier("insert", q.meta.Table)
	}
	return compile(q.q.Dialect(), q.render)
}
