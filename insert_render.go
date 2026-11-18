// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"slices"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

// The renderer of InsertQuery[T] lives on insertRef (non-generic) and reaches
// the models and the field hooks through an insAccess, for the reason given in
// select_core.go: a method body on InsertQuery[T] is compiled again for every
// model type. The methods here are the one-line wrappers that build the
// insAccess.

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
		b.SetErr(insertVerr("", "", "%T has no table", *new(T)))
		return
	}
	ms, back, meta := q.models, q.back, q.meta
	fp, ferr := newFieldPlan(q.q, meta)
	a := insAccess{
		value:   func(row, ci int) any { return meta.Value(&ms[row], ci) },
		zero:    func(ci int) (any, bool) { return zeroValue(meta, ci) },
		def:     func(ci, row int) any { return fieldDefault(&fp, ms, ci, row) },
		ptr:     func(row, ci int) any { return meta.Ptr(&ms[row], ci) },
		hasBack: func(row int) bool { return row < len(back) && back[row] != nil },
		backPtr: func(row, ci int) any { return meta.Ptr(back[row], ci) },
		clone:   func() { ms = slices.Clone(ms) },
		fields:  fp.ref,
	}
	r := q.ref()
	r.ferr = ferr
	r.render(b, a)
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

// zeroValue is the value of column ci of a zero T; ok is false when the Meta has
// no Value function.
func zeroValue[T any](meta *Meta[T], ci int) (any, bool) {
	if meta.Value == nil {
		return nil, false
	}
	var z T
	return meta.Value(&z, ci), true
}

// fieldDefault runs the Go-side default of column ci of fp for ms[row], or for
// a zero T when row is negative.
func fieldDefault[T any](fp *fieldPlan[T], ms []T, ci, row int) any {
	if row < 0 {
		var z T
		return fp.def[ci](&z)
	}
	return fp.def[ci](&ms[row])
}

// render writes the statement: the ids and the upsert defaults are resolved
// first, so the text is the one Exec runs. The checks of a missing table and
// of a view model stay in the generic wrapper, which can name T.
func (r insertRef) render(b *Builder, a insAccess) {
	r, err := r.generateIDs(false, a) // a preview; plan has done this for Exec and Rows
	if err == nil {
		r, err = r.resolveUpsert(a)
	}
	if err != nil {
		b.SetErr(err)
		return
	}
	r = r.withDBD(a)
	if len(r.i.with) > 0 && b.d.Name == dialect.MySQL {
		// MySQL and MariaDB accept WITH only before SELECT, UPDATE and DELETE.
		b.unsupported("WITH before INSERT (put the WITH inside FromSelect's select)")
		return
	}
	b.withScope(append(cteNames(r.i.with), r.m.Table), func() {
		writeWithClause(b, r.i.with)
		r.renderStatement(b, a)
	})
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
func (r insertRef) mode() (insertMode, error) {
	if r.i.sel != nil {
		return modeFromSelect, nil
	}
	modelMode := r.n > 0
	valuesMode := r.i.hasValues || r.i.hasRows
	switch {
	case r.i.hasValues && r.i.hasRows:
		return 0, r.verr("", "Values and ValuesRows cannot be combined")
	case modelMode && valuesMode:
		return 0, r.verr("", "Model/Models and Values/ValuesRows cannot be combined")
	case !modelMode && !valuesMode:
		return 0, r.verr("", "nothing to insert: call Model, Models, Values or ValuesRows")
	case valuesMode && len(r.i.omit) > 0:
		return 0, r.verr("", "Omit cannot be combined with Values or ValuesRows: list only the columns to write")
	}
	if modelMode {
		return modeModels, nil
	}
	return modeValues, nil
}

// insertPlan is a validated INSERT ready to be written: the resolved column
// list and the row values of the mode.
type insertPlan struct {
	mode       insertMode
	cols       []int
	rows       int
	rowAssigns []map[int]Assignment // values mode: the assignment of each column per row
	vals       [][]any              // models mode with field hooks: the validated values
	now        time.Time
}

func (r insertRef) renderStatement(b *Builder, a insAccess) {
	b.insertStmt = true
	mode, err := r.mode()
	if err == nil {
		err = r.checkExplicit()
	}
	if err != nil {
		b.SetErr(err)
		return
	}
	if mode == modeFromSelect && len(r.i.explicit) > 0 {
		b.SetErr(r.verr("", "Explicit cannot be combined with FromSelect: a select cannot supply identity values"))
		return
	}
	if mode == modeFromSelect {
		r.renderFromSelect(b, a)
		return
	}
	d := b.Dialect()
	if err := r.checkExplicitZero(d, a); err != nil {
		b.SetErr(err)
		return
	}
	if r.i.unnest {
		if err := r.unnestCheck(d, mode); err != nil {
			b.SetErr(err)
			return
		}
	}
	p, err := r.resolve(d, mode, a)
	if err != nil {
		b.SetErr(err)
		return
	}
	r.writeInsertHead(b)
	if len(p.cols) > 0 && r.i.unnest {
		r.writeUnnest(b, d, p, a)
	} else if len(p.cols) > 0 {
		r.writeColumnsAndRows(b, d, p, a)
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
		written[i] = r.m.Columns[ci].Name
	}
	r.renderConflict(b, written)
	renderReturningCols(b, r.m.Columns, r.i.returning, func(col string) error {
		return r.verr(col, "Returning: column %q is not mapped by %s", col, r.m.Table)
	})
	if len(p.cols) > 0 && mode == modeValues && d.MaxParams > 0 && len(b.args) > d.MaxParams {
		b.SetErr(&Error{Kind: ErrLimitExceeded, Op: "insert", Table: r.m.Table,
			Err: fmt.Errorf("%d bind parameters exceed the %s limit of %d", len(b.args), d.Name, d.MaxParams)})
	}
}

// resolve does all the validation of a Values or Models insert and returns the
// plan to write: columns, timestamps, parameter limit, field validators.
func (r insertRef) resolve(d *dialect.Dialect, mode insertMode, a insAccess) (insertPlan, error) {
	p := insertPlan{mode: mode, rows: r.n}
	p.now = r.clock // shared by the chunks of a bulk insert
	if p.now.IsZero() {
		p.now = r.q.conn().now() // read once per statement
	}
	if r.ferr != nil {
		return p, r.ferr
	}
	var err error
	if mode == modeModels {
		if p.cols, err = r.modelCols(a); err != nil {
			return p, err
		}
	} else {
		if p.cols, p.rowAssigns, err = r.valueRowsSafe(d, p.now, a); err != nil {
			return p, err
		}
		p.rows = len(p.rowAssigns)
	}
	for _, ci := range p.cols {
		if ci != r.m.CreatedAt && ci != r.m.UpdatedAt {
			continue
		}
		if err := r.checkTimestamp(ci, a); err != nil {
			return p, r.verr(r.m.Columns[ci].Name, "%v", err)
		}
	}
	if len(p.cols) == 0 && p.rows != 1 {
		return p, r.verr("", "no writable columns to insert")
	}
	if n := p.rows * len(p.cols); d.MaxParams > 0 && n > d.MaxParams && mode == modeModels && !r.i.unnest {
		return p, &Error{Kind: ErrLimitExceeded, Op: "insert", Table: r.m.Table,
			Err: fmt.Errorf("%d bind parameters exceed the %s limit of %d", n, d.Name, d.MaxParams)}
	}

	// With field hooks, model values are computed once, validated, then bound
	// as computed, so a validator sees exactly what is written even for a
	// random default. plan may already have done that (pre). Without hooks the
	// values stream straight into the statement.
	if mode == modeModels && a.fields.n > 0 {
		if p.vals = r.pre; p.vals == nil {
			p.vals = r.modelValues(p.cols, p.now, a)
			if err := r.validateModelRows(p.cols, p.vals, a); err != nil {
				return p, err
			}
		}
	}
	return p, r.validateConflictSet(a)
}

// writeColumnsAndRows writes the column list and the VALUES rows of a plan.
func (r insertRef) writeColumnsAndRows(b *Builder, d *dialect.Dialect, p insertPlan, a insAccess) {
	b.WriteString(" (")
	for i, ci := range p.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(r.m.Columns[ci].Name)
	}
	b.writeByte(')')
	if r.overriding(d, p.cols) {
		b.WriteString(" OVERRIDING SYSTEM VALUE")
	}
	b.WriteString(" VALUES ")
	if p.mode == modeModels {
		r.writeModelRows(b, p, a)
	} else {
		writeAssignRows(b, p)
	}
}

// writeModelRows writes one tuple per model.
func (r insertRef) writeModelRows(b *Builder, p insertPlan, a insAccess) {
	for row := range r.n {
		if row > 0 {
			b.WriteString(", ")
		}
		b.writeByte('(')
		for i, ci := range p.cols {
			if i > 0 {
				b.WriteString(", ")
			}
			if r.dbDefaultRow(row, ci) {
				b.WriteString("DEFAULT") // a dbdefault column of a row that sets others
			} else if p.vals != nil {
				b.argFor(r.m.Columns[ci].Flags, p.vals[row][i])
			} else {
				b.argFor(r.m.Columns[ci].Flags, r.modelValue(row, ci, p.now, a))
			}
		}
		b.writeByte(')')
	}
}

// writeAssignRows writes one tuple per Values or ValuesRows row; a column a row
// leaves out is DEFAULT.
func writeAssignRows(b *Builder, p insertPlan) {
	for row, m := range p.rowAssigns {
		if row > 0 {
			b.WriteString(", ")
		}
		b.writeByte('(')
		for i, ci := range p.cols {
			if i > 0 {
				b.WriteString(", ")
			}
			if as, ok := m[ci]; ok {
				appendAssignedValue(b, as)
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
// rows only fails with ErrUnsupported; Exec and Rows never reach that case,
// because plan has split such rows into runs (defaultRuns).
func (r insertRef) valueRows(d *dialect.Dialect, now time.Time, a insAccess) ([]int, []map[int]Assignment, error) {
	src := r.i.rows
	if !r.i.hasRows {
		src = [][]Assignment{r.i.assigns}
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
	for ri, row := range src {
		m := map[int]Assignment{}
		for _, as := range row {
			ci := r.column(as.col)
			if ci < 0 {
				return nil, nil, r.verr(as.col, "Values: column %q is not mapped by %s", as.col, r.m.Table)
			}
			if !r.writable(ci) {
				return nil, nil, r.verr(as.col, "Values: column %q is identity or read-only and is never written", as.col)
			}
			if _, dup := m[ci]; dup {
				return nil, nil, r.verr(as.col, "Values: column %q assigned twice", as.col)
			}
			if as.kind == setExcluded {
				return nil, nil, r.verr(as.col, "Values: SetExcluded is only valid in DoUpdate")
			}
			m[ci] = as
			add(ci)
		}
		// Go-side defaults fill the columns this row leaves out; an explicit
		// assignment, including SetDefault and SetNull, wins.
		for _, ci := range a.fields.defCols {
			if _, assigned := m[ci]; assigned || !r.writable(ci) {
				continue
			}
			m[ci] = Assignment{col: r.m.Columns[ci].Name, kind: setValue, val: a.defOf(ci, -1), flags: r.m.Columns[ci].Flags}
			add(ci)
		}
		seen := make(map[int]struct{}, len(m))
		for ci := range m {
			seen[ci] = struct{}{}
		}
		tc, ta, err := r.timestampAssigns(seen, now, a)
		if err != nil {
			return nil, nil, err
		}
		for k, ci := range tc {
			m[ci] = ta[k]
			add(ci)
		}
		rows[ri] = m
		err = a.fields.validate("insert", r.m.Table, r.m.Columns, func(ci int) (any, bool) {
			if as, ok := m[ci]; ok {
				return assignedValue(as)
			}
			return nil, r.writable(ci)
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
			if as, ok := m[ci]; !ok || as.kind == setDefault {
				defaults++
			}
		}
		switch {
		case defaults == len(rows):
			continue // omitting the column applies the default
		case defaults > 0:
			return nil, nil, &Error{Kind: ErrUnsupported, Op: "insert", Table: r.m.Table, Column: r.m.Columns[ci].Name,
				Err: fmt.Errorf("a per-row DEFAULT in a multi-row VALUES list is not supported by %s in one statement (SQL renders one); Exec and Rows split such rows into several statements", d.Name)}
		}
		cols = append(cols, ci)
	}
	return cols, rows, nil
}

// modelCols returns the columns a model insert writes, in metadata order.
func (r insertRef) modelCols(a insAccess) ([]int, error) {
	if r.i.fixedCols != nil {
		return r.i.fixedCols, nil
	}
	omit := map[int]struct{}{}
	for _, n := range r.i.omit {
		ci := r.column(n)
		if ci < 0 {
			return nil, r.verr(n, "Omit: column %q is not mapped by %s", n, r.m.Table)
		}
		omit[ci] = struct{}{}
	}
	var cols []int
	for i := range r.m.Columns {
		_, skip := omit[i]
		hasDefault := a.fields.hasDef(i)
		switch {
		case skip && hasDefault && r.writable(i):
			cols = append(cols, i) // omitted, so the Go-side default is written
		case !skip && (r.writable(i) || r.insertsIdentity(i)):
			drop, err := r.dbDefaultDecision(i)
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

// checkTimestamp reports an error when the created-at or updated-at field of
// column ci has a Go type automatic timestamps do not support. It is nil for a
// missing column.
func (r insertRef) checkTimestamp(ci int, a insAccess) error {
	if ci < 0 || ci >= len(r.m.Columns) {
		return nil
	}
	sample, ok := a.zero(ci)
	if !ok {
		return nil
	}
	return checkTimestampSample(r.m.Columns[ci].Name, sample)
}

// autoTimestamp returns the value for the timestamp column ci read at now; ok
// is false when the model has no such column or its field type is unsupported.
func (r insertRef) autoTimestamp(ci int, now time.Time, a insAccess) (any, bool) {
	if ci < 0 || ci >= len(r.m.Columns) {
		return nil, false
	}
	sample, ok := a.zero(ci)
	if !ok {
		return nil, false
	}
	return timestampValue(now, sample)
}

// timestampAssigns returns the automatic created-at and updated-at writes of
// a Values insert for the columns the caller did not assign.
func (r insertRef) timestampAssigns(seen map[int]struct{}, now time.Time, a insAccess) ([]int, []Assignment, error) {
	var cols []int
	var assigns []Assignment
	for _, ci := range []int{r.m.CreatedAt, r.m.UpdatedAt} {
		if _, assigned := seen[ci]; assigned || ci < 0 || !r.writable(ci) {
			continue
		}
		if err := r.checkTimestamp(ci, a); err != nil {
			return nil, nil, r.verr(r.m.Columns[ci].Name, "%v", err)
		}
		if v, ok := r.autoTimestamp(ci, now, a); ok {
			cols = append(cols, ci)
			assigns = append(assigns, Assignment{col: r.m.Columns[ci].Name, kind: setValue, val: v, flags: r.m.Columns[ci].Flags})
		}
	}
	return cols, assigns, nil
}

// modelValue is the value a model insert binds for column ci: the field value,
// except that an unset created-at or updated-at field takes the statement time
// and an omitted column takes its Go-side default.
func (r insertRef) modelValue(row, ci int, now time.Time, a insAccess) any {
	if a.fields.hasDef(ci) && slices.Contains(r.i.omit, r.m.Columns[ci].Name) {
		return a.defOf(ci, row)
	}
	if r.dbDefaultRow(row, ci) {
		return nil // DEFAULT: validators see the same nil as for an omitted column
	}
	v := a.val(row, ci)
	if (ci == r.m.CreatedAt || ci == r.m.UpdatedAt) && timestampUnset(v) {
		if t, ok := r.autoTimestamp(ci, now, a); ok {
			return t
		}
	}
	return v
}

// renderWhere writes ` WHERE ` and the predicates ANDed.
func renderWhere(b *Builder, preds []Predicate) {
	b.WriteString(" WHERE ")
	And(preds...).AppendSQL(b)
}

// modelValues computes the bound value of every column in cols for every
// model: row r, column cols[i] is at [r][i].
func (r insertRef) modelValues(cols []int, now time.Time, a insAccess) [][]any {
	vals := make([][]any, r.n)
	for row := range r.n {
		vs := make([]any, len(cols))
		for i, ci := range cols {
			vs[i] = r.modelValue(row, ci, now, a)
		}
		vals[row] = vs
	}
	return vals
}

// validateModelRows runs the field validators over the computed values of a
// model insert: vals[r][i] is the value of column cols[i] in row r. A column
// the statement leaves out validates as nil when it is writable.
func (r insertRef) validateModelRows(cols []int, vals [][]any, a insAccess) error {
	if a.fields.n == 0 {
		return nil
	}
	for _, row := range vals {
		err := a.fields.validate("insert", r.m.Table, r.m.Columns, func(ci int) (any, bool) {
			if i := slices.Index(cols, ci); i >= 0 {
				return row[i], true
			}
			return nil, r.writable(ci)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// validateConflictSet runs the field validators over the values a DO UPDATE
// list assigns, since an upsert writes them to the existing row.
func (r insertRef) validateConflictSet(a insAccess) error {
	if r.i.conflict != conflictUpdate || a.fields.n == 0 {
		return nil
	}
	return a.fields.validate("insert", r.m.Table, r.m.Columns, func(ci int) (any, bool) {
		for _, as := range r.i.set {
			if as.col == r.m.Columns[ci].Name {
				return assignedValue(as)
			}
		}
		return nil, false
	})
}
