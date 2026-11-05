// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
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
	omit      []string
	returning returningSet
	target    []string
	conflict  conflictKind
	set       []Assignment
	// upsert and withIdentity are owned by upsert.go: upsert marks an Upsert
	// query, withIdentity puts the identity column in a model INSERT list.
	upsert       bool
	withIdentity bool
	err          error
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
	q.models = append(slices.Clip(q.models), *m)
	return q
}

// Models adds every element of ms as one row of a multi-row VALUES list.
// With an empty list the query fails with ErrValidation.
func (q InsertQuery[T]) Models(ms []T) InsertQuery[T] {
	if len(ms) == 0 {
		return q.fail(q.verr("", "Models: no rows"))
	}
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

// Omit excludes columns from a model insert, leaving their database default.
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

// DoNothing skips rows that conflict.
func (c ConflictQuery[T]) DoNothing() InsertQuery[T] {
	q := c.q
	q.i.target, q.i.conflict, q.i.set = c.target, conflictNothing, nil
	return q
}

// DoUpdate updates the existing row on conflict. Use SetExcluded to take the
// value proposed for insertion.
func (c ConflictQuery[T]) DoUpdate(assigns ...Assignment) InsertQuery[T] {
	q := c.q
	q.i.target, q.i.conflict, q.i.set = c.target, conflictUpdate, slices.Clone(assigns)
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

func (q InsertQuery[T]) render(b *Builder) {
	if q.i.err != nil {
		b.SetErr(q.i.err)
		return
	}
	if q.meta.Table == "" {
		b.SetErr(q.verr("", "%T has no table", *new(T)))
		return
	}
	q, err := q.resolveUpsert()
	if err != nil {
		b.SetErr(err)
		return
	}
	b.withScope([]string{q.meta.Table}, func() { q.renderStatement(b) })
}

func (q InsertQuery[T]) renderStatement(b *Builder) {
	d := b.Dialect()
	modelMode := len(q.models) > 0
	if modelMode == q.i.hasValues {
		if modelMode {
			b.SetErr(q.verr("", "Model/Models and Values cannot be combined"))
		} else {
			b.SetErr(q.verr("", "nothing to insert: call Model, Models or Values"))
		}
		return
	}
	var cols []int
	var assigns []Assignment
	now := q.q.conn().now() // read once per statement
	if modelMode {
		omit := map[int]struct{}{}
		for _, n := range q.i.omit {
			ci := q.column(n)
			if ci < 0 {
				b.SetErr(q.verr(n, "Omit: column %q is not mapped by %s", n, q.meta.Table))
				return
			}
			omit[ci] = struct{}{}
		}
		for i := range q.meta.Columns {
			if _, skip := omit[i]; !skip && (q.writable(i) || q.insertsIdentity(i)) {
				cols = append(cols, i)
			}
		}
	} else {
		seen := map[int]struct{}{}
		for _, a := range q.i.assigns {
			ci := q.column(a.col)
			if ci < 0 {
				b.SetErr(q.verr(a.col, "Values: column %q is not mapped by %s", a.col, q.meta.Table))
				return
			}
			if !q.writable(ci) {
				b.SetErr(q.verr(a.col, "Values: column %q is identity or read-only and is never written", a.col))
				return
			}
			if _, dup := seen[ci]; dup {
				b.SetErr(q.verr(a.col, "Values: column %q assigned twice", a.col))
				return
			}
			seen[ci] = struct{}{}
			if a.kind == setExcluded {
				b.SetErr(q.verr(a.col, "Values: SetExcluded is only valid in DoUpdate"))
				return
			}
			if a.kind == setDefault && !d.DefaultInValues {
				continue // omitting the column applies the default
			}
			cols = append(cols, ci)
			assigns = append(assigns, a)
		}
		tc, ta := q.timestampAssigns(seen, now)
		cols, assigns = append(cols, tc...), append(assigns, ta...)
	}
	if len(cols) == 0 {
		b.SetErr(q.verr("", "no writable columns to insert"))
		return
	}
	rows := 1
	if modelMode {
		rows = len(q.models)
	}
	if n := rows * len(cols); d.MaxParams > 0 && n > d.MaxParams && modelMode {
		b.SetErr(&Error{Kind: ErrLimitExceeded, Op: "insert", Table: q.meta.Table,
			Err: fmt.Errorf("%d bind parameters exceed the %s limit of %d", n, d.Name, d.MaxParams)})
		return
	}

	b.WriteString("INSERT INTO ")
	if q.meta.Schema != "" {
		b.Ident(q.meta.Schema)
		b.writeByte('.')
	}
	b.Ident(q.meta.Table)
	b.WriteString(" (")
	for i, ci := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(q.meta.Columns[ci].Name)
	}
	b.writeByte(')')
	if q.i.withIdentity && d.Name == dialect.Postgres {
		b.WriteString(" OVERRIDING SYSTEM VALUE")
	}
	b.WriteString(" VALUES ")
	if modelMode {
		for r := range q.models {
			if r > 0 {
				b.WriteString(", ")
			}
			b.writeByte('(')
			for i, ci := range cols {
				if i > 0 {
					b.WriteString(", ")
				}
				b.Arg(q.modelValue(&q.models[r], ci, now))
			}
			b.writeByte(')')
		}
	} else {
		b.writeByte('(')
		for i, a := range assigns {
			if i > 0 {
				b.WriteString(", ")
			}
			q.appendValue(b, a)
		}
		b.writeByte(')')
	}
	q.renderConflict(b)
	renderReturning(b, q.meta, q.i.returning, func(col string) error {
		return q.verr(col, "Returning: column %q is not mapped by %s", col, q.meta.Table)
	})
}

// timestampAssigns returns the automatic created-at and updated-at writes of
// a Values insert for the columns the caller did not assign.
func (q InsertQuery[T]) timestampAssigns(seen map[int]struct{}, now time.Time) ([]int, []Assignment) {
	var cols []int
	var assigns []Assignment
	for _, ci := range []int{q.meta.CreatedAt, q.meta.UpdatedAt} {
		if _, assigned := seen[ci]; assigned || ci < 0 || !q.writable(ci) {
			continue
		}
		if v, ok := autoTimestamp(q.meta, ci, now); ok {
			cols = append(cols, ci)
			assigns = append(assigns, Assignment{col: q.meta.Columns[ci].Name, kind: setValue, val: v})
		}
	}
	return cols, assigns
}

// modelValue is the value a model insert binds for column ci: the field value,
// except that an unset created-at or updated-at field takes the statement time.
func (q InsertQuery[T]) modelValue(m *T, ci int, now time.Time) any {
	v := q.meta.Value(m, ci)
	if (ci == q.meta.CreatedAt || ci == q.meta.UpdatedAt) && timestampUnset(v) {
		if t, ok := autoTimestamp(q.meta, ci, now); ok {
			return t
		}
	}
	return v
}

// appendValue renders one assigned value inside VALUES or SET.
func (q InsertQuery[T]) appendValue(b *Builder, a Assignment) {
	switch a.kind {
	case setValue:
		b.Arg(a.val)
	case setExpr:
		a.expr.AppendSQL(b)
	case setNull:
		b.WriteString("NULL")
	case setDefault:
		b.WriteString("DEFAULT")
	case setExcluded:
		b.WriteString("EXCLUDED.")
		b.Ident(a.col)
	}
}

func (q InsertQuery[T]) renderConflict(b *Builder) {
	if q.i.conflict == conflictNone {
		return
	}
	if !b.Dialect().OnConflict {
		b.unsupported("ON CONFLICT")
		return
	}
	for _, n := range q.i.target {
		if q.column(n) < 0 {
			b.SetErr(q.verr(n, "OnConflict: column %q is not mapped by %s", n, q.meta.Table))
			return
		}
	}
	b.WriteString(" ON CONFLICT")
	if len(q.i.target) > 0 {
		b.WriteString(" (")
		for i, n := range q.i.target {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Ident(n)
		}
		b.writeByte(')')
	}
	if q.i.conflict == conflictNothing {
		b.WriteString(" DO NOTHING")
		return
	}
	if len(q.i.target) == 0 {
		b.SetErr(q.verr("", "DoUpdate needs a conflict target: pass columns to OnConflict"))
		return
	}
	if len(q.i.set) == 0 {
		b.SetErr(q.verr("", "DoUpdate needs at least one assignment"))
		return
	}
	b.WriteString(" DO UPDATE SET ")
	for i, a := range q.i.set {
		ci := q.column(a.col)
		if ci < 0 || !q.writable(ci) {
			b.SetErr(q.verr(a.col, "DoUpdate: column %q is not mapped or not writable", a.col))
			return
		}
		if a.kind == setDefault {
			b.SetErr(q.verr(a.col, "DoUpdate: SetDefault is not supported"))
			return
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(a.col)
		b.WriteString(" = ")
		q.appendValue(b, a)
	}
}

// Exec runs the statement and reports the rows it inserted.
func (q InsertQuery[T]) Exec(ctx context.Context) (Result, error) {
	c, err := q.SQL()
	if err != nil {
		return Result{}, err
	}
	n, err := q.q.conn().exec(ctx, "insert", q.meta.Table, c)
	if err != nil {
		return Result{}, err
	}
	return Result{RowsAffected: n}, nil
}

// Rows runs the statement and returns its RETURNING rows. It fails with
// ErrValidation when no RETURNING list was set.
func (q InsertQuery[T]) Rows(ctx context.Context) (*ModelRows[T], error) {
	if !q.i.returning.active() {
		return nil, q.verr("", "Rows needs a Returning list")
	}
	c, err := q.SQL()
	if err != nil {
		return nil, err
	}
	return runReturning(ctx, q.q, "insert", q.meta, q.i.returning, c)
}
