// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
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
// any other value fails with ErrValidation. Excluded() in the predicate
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
	valuesMode := q.i.hasValues || q.i.hasRows
	switch {
	case q.i.hasValues && q.i.hasRows:
		b.SetErr(q.verr("", "Values and ValuesRows cannot be combined"))
		return
	case modelMode && valuesMode:
		b.SetErr(q.verr("", "Model/Models and Values/ValuesRows cannot be combined"))
		return
	case !modelMode && !valuesMode:
		b.SetErr(q.verr("", "nothing to insert: call Model, Models, Values or ValuesRows"))
		return
	case valuesMode && len(q.i.omit) > 0:
		b.SetErr(q.verr("", "Omit cannot be combined with Values or ValuesRows: list only the columns to write"))
		return
	}
	var cols []int
	var rowAssigns []map[int]Assignment // values mode: the assignment of each column per row
	now := q.clock                      // shared by the chunks of a bulk insert
	if now.IsZero() {
		now = q.q.conn().now() // read once per statement
	}
	rows := len(q.models)
	fp, err := newFieldPlan(q.q, q.meta)
	if err != nil {
		b.SetErr(err)
		return
	}
	if modelMode {
		if cols, err = q.modelCols(fp); err != nil {
			b.SetErr(err)
			return
		}
	} else {
		if cols, rowAssigns, err = q.valueRows(d, now, fp); err != nil {
			b.SetErr(err)
			return
		}
		rows = len(rowAssigns)
	}
	for _, ci := range cols {
		if ci != q.meta.CreatedAt && ci != q.meta.UpdatedAt {
			continue
		}
		if err := checkTimestampField(q.meta, ci); err != nil {
			b.SetErr(q.verr(q.meta.Columns[ci].Name, "%v", err))
			return
		}
	}
	if len(cols) == 0 && rows != 1 {
		b.SetErr(q.verr("", "no writable columns to insert"))
		return
	}
	if n := rows * len(cols); d.MaxParams > 0 && n > d.MaxParams && modelMode {
		b.SetErr(&Error{Kind: ErrLimitExceeded, Op: "insert", Table: q.meta.Table,
			Err: fmt.Errorf("%d bind parameters exceed the %s limit of %d", n, d.Name, d.MaxParams)})
		return
	}

	// With field hooks, model values are computed once, validated, then bound
	// as computed, so a validator sees exactly what is written even for a
	// random default. plan may already have done that (pre). Without hooks the
	// values stream straight into the statement.
	var vals [][]any
	if modelMode && len(fp.hooks) > 0 {
		if vals = q.pre; vals == nil {
			vals = q.modelValues(fp, cols, now)
			if err := q.validateModelRows(fp, cols, vals); err != nil {
				b.SetErr(err)
				return
			}
		}
	}
	if err := q.validateConflictSet(fp); err != nil {
		b.SetErr(err)
		return
	}

	b.WriteString("INSERT INTO ")
	if q.meta.Schema != "" {
		b.Ident(q.meta.Schema)
		b.writeByte('.')
	}
	b.Ident(q.meta.Table)
	if len(cols) == 0 {
		// No column is written: every column takes its default.
		if d.Name == dialect.MySQL {
			b.WriteString(" () VALUES ()")
		} else {
			b.WriteString(" DEFAULT VALUES")
		}
		q.renderConflict(b)
		renderReturning(b, q.meta, q.i.returning, func(col string) error {
			return q.verr(col, "Returning: column %q is not mapped by %s", col, q.meta.Table)
		})
		return
	}
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
			for i := range cols {
				if i > 0 {
					b.WriteString(", ")
				}
				if vals != nil {
					b.Arg(vals[r][i])
				} else {
					b.Arg(q.modelValue(&q.models[r], cols[i], now, fp))
				}
			}
			b.writeByte(')')
		}
	} else {
		for r, m := range rowAssigns {
			if r > 0 {
				b.WriteString(", ")
			}
			b.writeByte('(')
			for i, ci := range cols {
				if i > 0 {
					b.WriteString(", ")
				}
				if a, ok := m[ci]; ok {
					q.appendValue(b, a)
				} else {
					b.WriteString("DEFAULT") // omitted in this row only
				}
			}
			b.writeByte(')')
		}
	}
	q.renderConflict(b)
	renderReturning(b, q.meta, q.i.returning, func(col string) error {
		return q.verr(col, "Returning: column %q is not mapped by %s", col, q.meta.Table)
	})
	if !modelMode && d.MaxParams > 0 && len(b.args) > d.MaxParams {
		b.SetErr(&Error{Kind: ErrLimitExceeded, Op: "insert", Table: q.meta.Table,
			Err: fmt.Errorf("%d bind parameters exceed the %s limit of %d", len(b.args), d.Name, d.MaxParams)})
	}
}

// valueRows validates the rows of Values or ValuesRows and returns the INSERT
// column list (the union of the assigned columns in order of first
// appearance, automatic timestamps included) and each row's assignment per
// column. On a dialect without DEFAULT inside VALUES, a column that is
// default in every row leaves the list, and a column that is default in some
// rows only fails with ErrUnsupported.
func (q InsertQuery[T]) valueRows(d *dialect.Dialect, now time.Time, fp fieldPlan[T]) ([]int, []map[int]Assignment, error) {
	src := q.i.rows
	if !q.i.hasRows {
		src = [][]Assignment{q.i.assigns}
	}
	var order []int
	inOrder := map[int]struct{}{}
	add := func(ci int) {
		if _, ok := inOrder[ci]; !ok {
			inOrder[ci] = struct{}{}
			order = append(order, ci)
		}
	}
	rows := make([]map[int]Assignment, len(src))
	for r, row := range src {
		m := map[int]Assignment{}
		for _, a := range row {
			ci := q.column(a.col)
			if ci < 0 {
				return nil, nil, q.verr(a.col, "Values: column %q is not mapped by %s", a.col, q.meta.Table)
			}
			if !q.writable(ci) {
				return nil, nil, q.verr(a.col, "Values: column %q is identity or read-only and is never written", a.col)
			}
			if _, dup := m[ci]; dup {
				return nil, nil, q.verr(a.col, "Values: column %q assigned twice", a.col)
			}
			if a.kind == setExcluded {
				return nil, nil, q.verr(a.col, "Values: SetExcluded is only valid in DoUpdate")
			}
			m[ci] = a
			add(ci)
		}
		// Go-side defaults fill the columns this row leaves out; an explicit
		// assignment, including SetDefault and SetNull, wins.
		for _, ci := range slices.Sorted(maps.Keys(fp.def)) {
			if _, assigned := m[ci]; assigned || !q.writable(ci) {
				continue
			}
			var zero T
			m[ci] = Assignment{col: q.meta.Columns[ci].Name, kind: setValue, val: fp.def[ci](&zero)}
			add(ci)
		}
		seen := make(map[int]struct{}, len(m))
		for ci := range m {
			seen[ci] = struct{}{}
		}
		tc, ta, err := q.timestampAssigns(seen, now)
		if err != nil {
			return nil, nil, err
		}
		for k, ci := range tc {
			m[ci] = ta[k]
			add(ci)
		}
		rows[r] = m
		err = fp.validate("insert", q.meta, func(ci int) (any, bool) {
			if a, ok := m[ci]; ok {
				return assignedValue(a)
			}
			return nil, q.writable(ci)
		})
		if err != nil {
			return nil, nil, err
		}
	}
	if d.DefaultInValues {
		return order, rows, nil
	}
	var cols []int
	for _, ci := range order {
		defaults := 0
		for _, m := range rows {
			if a, ok := m[ci]; !ok || a.kind == setDefault {
				defaults++
			}
		}
		switch {
		case defaults == len(rows):
			continue // omitting the column applies the default
		case defaults > 0:
			return nil, nil, &Error{Kind: ErrUnsupported, Op: "insert", Table: q.meta.Table, Column: q.meta.Columns[ci].Name,
				Err: fmt.Errorf("a per-row DEFAULT in a multi-row VALUES list is not supported by %s", d.Name)}
		}
		cols = append(cols, ci)
	}
	return cols, rows, nil
}

// modelCols returns the columns a model insert writes, in metadata order.
func (q InsertQuery[T]) modelCols(fp fieldPlan[T]) ([]int, error) {
	omit := map[int]struct{}{}
	for _, n := range q.i.omit {
		ci := q.column(n)
		if ci < 0 {
			return nil, q.verr(n, "Omit: column %q is not mapped by %s", n, q.meta.Table)
		}
		omit[ci] = struct{}{}
	}
	var cols []int
	for i := range q.meta.Columns {
		_, skip := omit[i]
		_, hasDefault := fp.def[i]
		switch {
		case skip && hasDefault && q.writable(i):
			cols = append(cols, i) // omitted, so the Go-side default is written
		case !skip && (q.writable(i) || q.insertsIdentity(i)):
			cols = append(cols, i)
		}
	}
	return cols, nil
}

// timestampAssigns returns the automatic created-at and updated-at writes of
// a Values insert for the columns the caller did not assign.
func (q InsertQuery[T]) timestampAssigns(seen map[int]struct{}, now time.Time) ([]int, []Assignment, error) {
	var cols []int
	var assigns []Assignment
	for _, ci := range []int{q.meta.CreatedAt, q.meta.UpdatedAt} {
		if _, assigned := seen[ci]; assigned || ci < 0 || !q.writable(ci) {
			continue
		}
		if err := checkTimestampField(q.meta, ci); err != nil {
			return nil, nil, q.verr(q.meta.Columns[ci].Name, "%v", err)
		}
		if v, ok := autoTimestamp(q.meta, ci, now); ok {
			cols = append(cols, ci)
			assigns = append(assigns, Assignment{col: q.meta.Columns[ci].Name, kind: setValue, val: v})
		}
	}
	return cols, assigns, nil
}

// modelValue is the value a model insert binds for column ci: the field value,
// except that an unset created-at or updated-at field takes the statement time
// and an omitted column takes its Go-side default.
func (q InsertQuery[T]) modelValue(m *T, ci int, now time.Time, fp fieldPlan[T]) any {
	if f, ok := fp.def[ci]; ok && slices.Contains(q.i.omit, q.meta.Columns[ci].Name) {
		return f(m)
	}
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

// renderWhere writes ` WHERE ` and the predicates ANDed.
func renderWhere(b *Builder, preds []Predicate) {
	b.WriteString(" WHERE ")
	And(preds...).AppendSQL(b)
}

func (q InsertQuery[T]) renderConflict(b *Builder) {
	if len(q.i.actionWhere) > 0 && q.i.conflict != conflictUpdate {
		b.SetErr(q.verr("", "Where is only valid after DoUpdate (or on Upsert): there is no DO UPDATE to condition"))
		return
	}
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
	if len(q.i.targetWhere) > 0 {
		if len(q.i.target) == 0 {
			b.SetErr(q.verr("", "OnConflict(...).Where needs a conflict target: pass columns to OnConflict"))
			return
		}
		prev := b.unqualified
		b.unqualified, b.conflictTarget = true, true
		renderWhere(b, q.i.targetWhere)
		b.unqualified, b.conflictTarget = prev, false
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
	if len(q.i.actionWhere) > 0 {
		renderWhere(b, q.i.actionWhere)
	}
}

// Exec runs the statement and reports the rows it inserted.
//
// A Models call whose rows would exceed the dialect's bind-parameter limit
// is split into several multi-row statements, run in order, and the rows
// affected are summed. The chunks are atomic: on a DB they run in one
// transaction, on a Tx inside a savepoint (when the dialect has them), so a
// failing chunk undoes the earlier ones. A Tx on a dialect without
// savepoints runs the chunks directly in the caller's transaction, which
// stays atomic as a whole. A DB pinned to one connection by OnConn cannot
// begin a transaction, so a chunked insert there fails with ErrUnsupported
// before any statement runs. A panic during a chunk rolls the scope back and
// is re-raised. All chunks share one clock read, so every row gets the same automatic
// timestamp. A single row with more bound columns than the limit fails with
// ErrLimitExceeded.
func (q InsertQuery[T]) Exec(ctx context.Context) (Result, error) {
	chunks, err := q.plan()
	if err != nil {
		return Result{}, err
	}
	if len(chunks) == 1 {
		return chunks[0].execOne(ctx)
	}
	sc, err := q.beginScope(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = sc.finish(context.WithoutCancel(ctx), fmt.Errorf("momo: panic during chunked insert: %v", p))
			panic(p)
		}
	}()
	var total int64
	for _, c := range chunks {
		c.q = sc.q
		r, err := c.execOne(ctx)
		if err != nil {
			return Result{}, sc.finish(ctx, err)
		}
		total += r.RowsAffected
	}
	if err := sc.finish(ctx, nil); err != nil {
		return Result{}, err
	}
	return Result{RowsAffected: total}, nil
}

func (q InsertQuery[T]) execOne(ctx context.Context) (Result, error) {
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
// ErrValidation when no RETURNING list was set. A chunked bulk insert (see
// Exec) returns the rows of all chunks in insertion order. The first chunk
// runs inside Rows, so its errors are returned here; the remaining chunks run
// as the rows are consumed, and Close runs any that are left so the insert
// stays whole. The caller MUST check the error from Close (and Err): a later
// chunk can fail there, which rolls the whole insert back.
func (q InsertQuery[T]) Rows(ctx context.Context) (*ModelRows[T], error) {
	if !q.i.returning.active() {
		return nil, q.verr("", "Rows needs a Returning list")
	}
	chunks, err := q.plan()
	if err != nil {
		return nil, err
	}
	if len(chunks) == 1 {
		return chunks[0].rowsOne(ctx)
	}
	sc, err := q.beginScope(ctx)
	if err != nil {
		return nil, err
	}
	idx, _ := returningIndexes(q.meta, q.i.returning)
	for i := range chunks {
		chunks[i].q = sc.q
	}
	cr := &chunkRows[T]{ctx: ctx, chunks: chunks, sc: sc}
	if err := cr.open(); err != nil {
		cr.done = true
		return nil, sc.finish(context.WithoutCancel(ctx), err)
	}
	return &ModelRows[T]{rows: cr, meta: q.meta, idx: idx}, nil
}

func (q InsertQuery[T]) rowsOne(ctx context.Context) (*ModelRows[T], error) {
	c, err := q.SQL()
	if err != nil {
		return nil, err
	}
	return runReturning(ctx, q.q, "insert", q.meta, q.i.returning, c)
}

// plan resolves the query once and splits a bulk model insert into chunks
// that each stay under the dialect's bind-parameter limit. It returns the
// query itself when no split is needed (or when rendering will report the
// error), so validation errors keep their single-statement form.
func (q InsertQuery[T]) plan() ([]InsertQuery[T], error) {
	one := []InsertQuery[T]{q}
	if q.q == nil || q.i.err != nil || q.meta.Table == "" || len(q.models) < 2 {
		return one, nil
	}
	d := q.q.Dialect()
	if d.MaxParams <= 0 {
		return one, nil
	}
	r, err := q.resolveUpsert()
	if err != nil {
		return nil, err
	}
	r.i.upsert = false // resolved: chunks keep target, set and identity decisions
	fp, err := newFieldPlan(q.q, q.meta)
	if err != nil {
		return one, nil // rendering reports it
	}
	cols, err := r.modelCols(fp)
	if err != nil || len(cols) == 0 {
		return one, nil
	}
	r.clock = q.q.conn().now()
	// Compute and validate every row before the first chunk runs, so a bad
	// row late in a bulk insert executes nothing. The computed values are
	// kept and bound by the chunks, so a non-deterministic default cannot
	// pass here and change afterwards.
	if len(fp.hooks) > 0 {
		r.pre = r.modelValues(fp, cols, r.clock)
		if err := r.validateModelRows(fp, cols, r.pre); err != nil {
			return nil, err
		}
		one[0].pre = r.pre // a fallback to the single statement binds the same values
		one[0].clock = r.clock
	}
	probe := r
	probe.models = r.models[:1]
	if r.pre != nil {
		probe.pre = r.pre[:1]
	}
	c1, err := compile(d, probe.render)
	if err != nil {
		return one, nil
	}
	fixed := len(c1.Args) - len(cols) // parameters outside the VALUES rows
	per := (d.MaxParams - fixed) / len(cols)
	if per < 1 {
		return nil, &Error{Kind: ErrLimitExceeded, Op: "insert", Table: q.meta.Table,
			Err: fmt.Errorf("one row binds %d parameters, over the %s limit of %d", len(cols)+fixed, d.Name, d.MaxParams)}
	}
	if len(r.models) <= per {
		return []InsertQuery[T]{r}, nil
	}
	var out []InsertQuery[T]
	for lo := 0; lo < len(r.models); lo += per {
		c := r
		hi := min(lo+per, len(r.models))
		c.models = r.models[lo:hi]
		if r.pre != nil {
			c.pre = r.pre[lo:hi]
		}
		out = append(out, c)
	}
	return out, nil
}

// chunkScope is the atomic scope a chunked insert runs in.
type chunkScope struct {
	q      Querier
	finish func(ctx context.Context, err error) error
}

// beginScope opens a transaction on a DB or a savepoint on a Tx, and returns
// the Querier the chunks run on. On a Tx without savepoints it opens nothing:
// the caller's transaction is atomic as a whole. A pinned connection cannot
// begin a transaction, so it fails with ErrUnsupported.
func (q InsertQuery[T]) beginScope(ctx context.Context) (chunkScope, error) {
	noop := chunkScope{q: q.q, finish: func(_ context.Context, err error) error { return err }}
	switch h := q.q.(type) {
	case *DB:
		if h.pinned {
			return chunkScope{}, &Error{Kind: ErrUnsupported, Op: "insert", Table: q.meta.Table,
				Err: errors.New("chunked insert needs a transaction; wrap the call in db.Tx or call on a Tx")}
		}
		tx, err := h.Begin(ctx, TxOptions{})
		if err != nil {
			return chunkScope{}, err
		}
		return chunkScope{q: tx, finish: func(ctx context.Context, err error) error {
			if err != nil {
				if rb := tx.Rollback(context.WithoutCancel(ctx)); rb != nil && !errors.Is(rb, ErrTxDone) {
					return errors.Join(err, rb)
				}
				return err
			}
			return tx.Commit(ctx)
		}}, nil
	case *Tx:
		if !h.ex.d.Savepoints {
			return noop, nil
		}
		name := "momo_chunk_" + strconv.Itoa(h.depth+1)
		if err := h.Savepoint(ctx, name); err != nil {
			return chunkScope{}, err
		}
		return chunkScope{q: h, finish: func(ctx context.Context, err error) error {
			if err != nil {
				ctx = context.WithoutCancel(ctx)
				if rb := h.RollbackTo(ctx, name); rb != nil {
					return errors.Join(err, rb)
				}
			}
			if rel := h.Release(ctx, name); rel != nil && err == nil {
				return rel
			}
			return err
		}}, nil
	}
	return noop, nil
}

// chunkRows is the Rows of a chunked insert with RETURNING: it queries the
// chunks one after another and finishes the atomic scope at the end.
type chunkRows[T any] struct {
	ctx    context.Context
	chunks []InsertQuery[T]
	sc     chunkScope
	cur    Rows
	next   int
	err    error
	done   bool
}

func (r *chunkRows[T]) finish(err error) {
	if r.done {
		return
	}
	r.done = true
	if err = r.sc.finish(r.ctx, err); err != nil && r.err == nil {
		r.err = err
	}
}

// open queries the next chunk and makes it the current cursor.
func (r *chunkRows[T]) open() error {
	c := r.chunks[r.next]
	r.next++
	cmp, err := c.SQL()
	if err == nil {
		r.cur, err = c.q.conn().query(r.ctx, "insert", c.meta.Table, "", cmp)
	}
	return err
}

func (r *chunkRows[T]) Next() bool {
	for !r.done {
		if r.cur == nil {
			if r.next >= len(r.chunks) {
				r.finish(nil)
				return false
			}
			if err := r.open(); err != nil {
				r.err = err
				r.finish(err)
				return false
			}
		}
		if r.cur.Next() {
			return true
		}
		err := r.cur.Err()
		_ = r.cur.Close()
		r.cur = nil
		if err != nil {
			r.err = err
			r.finish(err)
			return false
		}
	}
	return false
}

func (r *chunkRows[T]) Scan(dest ...any) error {
	if r.cur == nil {
		return errors.New("momo: Scan without a current row")
	}
	return r.cur.Scan(dest...)
}

func (r *chunkRows[T]) Err() error { return r.err }

// Close runs the chunks that were not read yet, so the insert is whole, and
// ends the atomic scope.
func (r *chunkRows[T]) Close() error {
	if r.done {
		return r.err
	}
	if r.cur != nil {
		if err := r.cur.Close(); err != nil && r.err == nil {
			r.err = err
		}
		r.cur = nil
	}
	for r.err == nil && r.next < len(r.chunks) {
		c := r.chunks[r.next]
		r.next++
		_, r.err = c.execOne(r.ctx)
	}
	r.finish(r.err)
	return r.err
}

// modelValues computes the bound value of every column in cols for every
// model: row r, column cols[i] is at [r][i].
func (q InsertQuery[T]) modelValues(fp fieldPlan[T], cols []int, now time.Time) [][]any {
	vals := make([][]any, len(q.models))
	for r := range q.models {
		row := make([]any, len(cols))
		for i, ci := range cols {
			row[i] = q.modelValue(&q.models[r], ci, now, fp)
		}
		vals[r] = row
	}
	return vals
}

// validateModelRows runs the field validators over the computed values of a
// model insert: vals[r][i] is the value of column cols[i] in row r. A column
// the statement leaves out validates as nil when it is writable.
func (q InsertQuery[T]) validateModelRows(fp fieldPlan[T], cols []int, vals [][]any) error {
	if len(fp.hooks) == 0 {
		return nil
	}
	for _, row := range vals {
		err := fp.validate("insert", q.meta, func(ci int) (any, bool) {
			if i := slices.Index(cols, ci); i >= 0 {
				return row[i], true
			}
			return nil, q.writable(ci)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// validateConflictSet runs the field validators over the values a DO UPDATE
// list assigns, since an upsert writes them to the existing row.
func (q InsertQuery[T]) validateConflictSet(fp fieldPlan[T]) error {
	if q.i.conflict != conflictUpdate || len(fp.hooks) == 0 {
		return nil
	}
	return fp.validate("insert", q.meta, func(ci int) (any, bool) {
		for _, a := range q.i.set {
			if a.col == q.meta.Columns[ci].Name {
				return assignedValue(a)
			}
		}
		return nil, false
	})
}
