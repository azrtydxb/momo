// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

func (q InsertQuery[T]) render(b *Builder) {
	if err := viewWriteErr("insert", q.meta); err != nil {
		b.SetErr(err)
		return
	}
	if q.i.err != nil {
		b.SetErr(q.i.err)
		return
	}
	if q.meta.Table == "" {
		b.SetErr(q.verr("", "%T has no table", *new(T)))
		return
	}
	q, err := q.generateIDs(false) // a preview; plan has done this for Exec and Rows
	if err == nil {
		q, err = q.resolveUpsert()
	}
	if err != nil {
		b.SetErr(err)
		return
	}
	q = q.withDBD()
	if len(q.i.with) > 0 && b.d.Name == dialect.MySQL {
		// MySQL and MariaDB accept WITH only before SELECT, UPDATE and DELETE.
		b.unsupported("WITH before INSERT (put the WITH inside FromSelect's select)")
		return
	}
	b.withScope(append(cteNames(q.i.with), q.meta.Table), func() {
		writeWithClause(b, q.i.with)
		q.renderStatement(b)
	})
}

func (InsertQuery[T]) isDML() {}

// isSubquery lets an INSERT be the body of a WITH clause (PostgreSQL only).
func (InsertQuery[T]) isSubquery() {}

// AppendSQL renders the statement in parentheses as a WITH clause body, as
// SQL would, without running interceptors or the BeforeInsert hook. Only
// PostgreSQL accepts the result.
func (q InsertQuery[T]) AppendSQL(b *Builder) {
	if q.q == nil {
		b.SetErr(noQuerier("insert", q.meta.Table))
		return
	}
	b.writeByte('(')
	q.render(b)
	b.writeByte(')')
}

// insertMode is the row source of an INSERT, resolved once from the builder
// state so the rest of the renderer never re-derives it from flags.
type insertMode uint8

const (
	modeModels     insertMode = iota // Model or Models: whole structs
	modeValues                       // Values or ValuesRows: explicit assignments
	modeFromSelect                   // FromSelect: INSERT ... SELECT
)

// mode resolves the row source and rejects mixes of sources: the one place the
// mutually exclusive builder calls are cross-checked.
func (q InsertQuery[T]) mode() (insertMode, error) {
	if q.i.sel != nil {
		return modeFromSelect, nil
	}
	modelMode := len(q.models) > 0
	valuesMode := q.i.hasValues || q.i.hasRows
	switch {
	case q.i.hasValues && q.i.hasRows:
		return 0, q.verr("", "Values and ValuesRows cannot be combined")
	case modelMode && valuesMode:
		return 0, q.verr("", "Model/Models and Values/ValuesRows cannot be combined")
	case !modelMode && !valuesMode:
		return 0, q.verr("", "nothing to insert: call Model, Models, Values or ValuesRows")
	case valuesMode && len(q.i.omit) > 0:
		return 0, q.verr("", "Omit cannot be combined with Values or ValuesRows: list only the columns to write")
	}
	if modelMode {
		return modeModels, nil
	}
	return modeValues, nil
}

// insertPlan is a validated INSERT ready to be written: the resolved column
// list and the row values of the mode.
type insertPlan[T any] struct {
	mode       insertMode
	cols       []int
	rows       int
	rowAssigns []map[int]Assignment // values mode: the assignment of each column per row
	vals       [][]any              // models mode with field hooks: the validated values
	now        time.Time
	fp         fieldPlan[T]
}

func (q InsertQuery[T]) renderStatement(b *Builder) {
	b.insertStmt = true
	mode, err := q.mode()
	if err == nil {
		err = q.checkExplicit()
	}
	if err != nil {
		b.SetErr(err)
		return
	}
	if mode == modeFromSelect && len(q.i.explicit) > 0 {
		b.SetErr(q.verr("", "Explicit cannot be combined with FromSelect: a select cannot supply identity values"))
		return
	}
	if mode == modeFromSelect {
		q.renderFromSelect(b)
		return
	}
	d := b.Dialect()
	if err := q.checkExplicitZero(d); err != nil {
		b.SetErr(err)
		return
	}
	if q.i.unnest {
		if err := q.unnestCheck(d, mode); err != nil {
			b.SetErr(err)
			return
		}
	}
	p, err := q.resolve(d, mode)
	if err != nil {
		b.SetErr(err)
		return
	}
	q.writeInsertHead(b)
	if len(p.cols) > 0 && q.i.unnest {
		q.writeUnnest(b, d, p)
	} else if len(p.cols) > 0 {
		q.writeColumnsAndRows(b, d, p)
	} else {
		// No column is written: every column takes its default.
		if d.Name == dialect.MySQL {
			b.WriteString(" () VALUES ()")
		} else {
			b.WriteString(" DEFAULT VALUES")
		}
	}
	written := make([]string, len(p.cols))
	for i, ci := range p.cols {
		written[i] = q.meta.Columns[ci].Name
	}
	q.renderConflict(b, written)
	renderReturning(b, q.meta, q.i.returning, func(col string) error {
		return q.verr(col, "Returning: column %q is not mapped by %s", col, q.meta.Table)
	})
	if len(p.cols) > 0 && mode == modeValues && d.MaxParams > 0 && len(b.args) > d.MaxParams {
		b.SetErr(&Error{Kind: ErrLimitExceeded, Op: "insert", Table: q.meta.Table,
			Err: fmt.Errorf("%d bind parameters exceed the %s limit of %d", len(b.args), d.Name, d.MaxParams)})
	}
}

// resolve does all the validation of a Values or Models insert and returns the
// plan to write: columns, timestamps, parameter limit, field validators.
func (q InsertQuery[T]) resolve(d *dialect.Dialect, mode insertMode) (insertPlan[T], error) {
	p := insertPlan[T]{mode: mode, rows: len(q.models)}
	p.now = q.clock // shared by the chunks of a bulk insert
	if p.now.IsZero() {
		p.now = q.q.conn().now() // read once per statement
	}
	var err error
	if p.fp, err = newFieldPlan(q.q, q.meta); err != nil {
		return p, err
	}
	if mode == modeModels {
		if p.cols, err = q.modelCols(p.fp); err != nil {
			return p, err
		}
	} else {
		if p.cols, p.rowAssigns, err = q.valueRowsSafe(d, p.now, p.fp); err != nil {
			return p, err
		}
		p.rows = len(p.rowAssigns)
	}
	for _, ci := range p.cols {
		if ci != q.meta.CreatedAt && ci != q.meta.UpdatedAt {
			continue
		}
		if err := checkTimestampField(q.meta, ci); err != nil {
			return p, q.verr(q.meta.Columns[ci].Name, "%v", err)
		}
	}
	if len(p.cols) == 0 && p.rows != 1 {
		return p, q.verr("", "no writable columns to insert")
	}
	if n := p.rows * len(p.cols); d.MaxParams > 0 && n > d.MaxParams && mode == modeModels && !q.i.unnest {
		return p, &Error{Kind: ErrLimitExceeded, Op: "insert", Table: q.meta.Table,
			Err: fmt.Errorf("%d bind parameters exceed the %s limit of %d", n, d.Name, d.MaxParams)}
	}

	// With field hooks, model values are computed once, validated, then bound
	// as computed, so a validator sees exactly what is written even for a
	// random default. plan may already have done that (pre). Without hooks the
	// values stream straight into the statement.
	if mode == modeModels && len(p.fp.hooks) > 0 {
		if p.vals = q.pre; p.vals == nil {
			p.vals = q.modelValues(p.fp, p.cols, p.now)
			if err := q.validateModelRows(p.fp, p.cols, p.vals); err != nil {
				return p, err
			}
		}
	}
	return p, q.validateConflictSet(p.fp)
}

// writeColumnsAndRows writes the column list and the VALUES rows of a plan.
func (q InsertQuery[T]) writeColumnsAndRows(b *Builder, d *dialect.Dialect, p insertPlan[T]) {
	b.WriteString(" (")
	for i, ci := range p.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(q.meta.Columns[ci].Name)
	}
	b.writeByte(')')
	if q.overriding(d, p.cols) {
		b.WriteString(" OVERRIDING SYSTEM VALUE")
	}
	b.WriteString(" VALUES ")
	if p.mode == modeModels {
		q.writeModelRows(b, p)
	} else {
		q.writeAssignRows(b, p)
	}
}

// writeModelRows writes one tuple per model.
func (q InsertQuery[T]) writeModelRows(b *Builder, p insertPlan[T]) {
	for r := range q.models {
		if r > 0 {
			b.WriteString(", ")
		}
		b.writeByte('(')
		for i, ci := range p.cols {
			if i > 0 {
				b.WriteString(", ")
			}
			if q.dbDefaultRow(r, ci) {
				b.WriteString("DEFAULT") // a dbdefault column of a row that sets others
			} else if p.vals != nil {
				b.argFor(q.meta.Columns[ci].Flags, p.vals[r][i])
			} else {
				b.argFor(q.meta.Columns[ci].Flags, q.modelValue(r, ci, p.now, p.fp))
			}
		}
		b.writeByte(')')
	}
}

// writeAssignRows writes one tuple per Values or ValuesRows row; a column a row
// leaves out is DEFAULT.
func (q InsertQuery[T]) writeAssignRows(b *Builder, p insertPlan[T]) {
	for r, m := range p.rowAssigns {
		if r > 0 {
			b.WriteString(", ")
		}
		b.writeByte('(')
		for i, ci := range p.cols {
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
			m[ci] = Assignment{col: q.meta.Columns[ci].Name, kind: setValue, val: fp.def[ci](&zero), flags: q.meta.Columns[ci].Flags}
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
	if q.i.fixedCols != nil {
		return q.i.fixedCols, nil
	}
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
			drop, err := q.dbDefaultDecision(i)
			if err != nil {
				return nil, err
			}
			if !drop {
				cols = append(cols, i)
			}
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
			assigns = append(assigns, Assignment{col: q.meta.Columns[ci].Name, kind: setValue, val: v, flags: q.meta.Columns[ci].Flags})
		}
	}
	return cols, assigns, nil
}

// modelValue is the value a model insert binds for column ci: the field value,
// except that an unset created-at or updated-at field takes the statement time
// and an omitted column takes its Go-side default.
func (q InsertQuery[T]) modelValue(r, ci int, now time.Time, fp fieldPlan[T]) any {
	m := &q.models[r]
	if f, ok := fp.def[ci]; ok && slices.Contains(q.i.omit, q.meta.Columns[ci].Name) {
		return f(m)
	}
	if q.dbDefaultRow(r, ci) {
		return nil // DEFAULT: validators see the same nil as for an omitted column
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
		b.argFor(a.flags, a.val)
	case setExpr:
		b.exprFor(a.flags, a.expr)
	case setNull:
		b.WriteString("NULL")
	case setDefault:
		b.WriteString("DEFAULT")
	case setExcluded:
		b.writeExcluded(a.col)
	}
}

// renderWhere writes ` WHERE ` and the predicates ANDed.
func renderWhere(b *Builder, preds []Predicate) {
	b.WriteString(" WHERE ")
	And(preds...).AppendSQL(b)
}

// renderConflict writes the conflict clause. written names the columns the
// statement writes, which MySQL's conflict-target rule needs.
func (q InsertQuery[T]) renderConflict(b *Builder, written []string) {
	if len(q.i.actionWhere) > 0 && q.i.conflict != conflictUpdate {
		b.SetErr(q.verr("", "Where is only valid after DoUpdate (or on Upsert): there is no DO UPDATE to condition"))
		return
	}
	if q.i.conflict == conflictNone {
		return
	}
	if b.Dialect().OnDuplicateKey {
		q.renderMySQLConflict(b, written)
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

// modelValues computes the bound value of every column in cols for every
// model: row r, column cols[i] is at [r][i].
func (q InsertQuery[T]) modelValues(fp fieldPlan[T], cols []int, now time.Time) [][]any {
	vals := make([][]any, len(q.models))
	for r := range q.models {
		row := make([]any, len(cols))
		for i, ci := range cols {
			row[i] = q.modelValue(r, ci, now, fp)
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

// writeInsertHead writes INSERT INTO [schema.]table.
func (q InsertQuery[T]) writeInsertHead(b *Builder) {
	b.WriteString("INSERT INTO ")
	if q.meta.Schema != "" {
		b.Ident(q.meta.Schema)
		b.writeByte('.')
	}
	b.Ident(q.meta.Table)
}
