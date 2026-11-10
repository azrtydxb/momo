// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"fmt"
	"reflect"
	"slices"
)

// dbDefaultCol reports whether column ci carries the dbdefault tag and a model
// insert may write it.
func (q InsertQuery[T]) dbDefaultCol(ci int) bool {
	return q.meta.Columns[ci].Flags&FlagDBDefault != 0 && q.writable(ci) &&
		ci != q.meta.CreatedAt && ci != q.meta.UpdatedAt
}

// dbdPlan records, once per statement, which cells of a model insert take the
// database default: rows[r][ci] is true when model r leaves column ci to it,
// count[ci] is the number of such rows. It is nil-free: a statement without a
// dbdefault column has empty rows and a zero count everywhere.
type dbdPlan struct {
	rows  [][]bool
	count []int
}

// noDBD is the shared plan of a statement without dbdefault cells.
var noDBD = &dbdPlan{}

// withDBD returns q with its dbdefault plan computed. A column listed in Omit
// keeps its Go-side default and never takes the database default.
func (q InsertQuery[T]) withDBD() InsertQuery[T] {
	if q.dbd != nil {
		return q
	}
	var cols []int
	for ci := range q.meta.Columns {
		if q.dbDefaultCol(ci) && !slices.Contains(q.i.omit, q.meta.Columns[ci].Name) {
			cols = append(cols, ci)
		}
	}
	if len(cols) == 0 {
		q.dbd = noDBD // no allocation for the common model without dbdefault columns
		return q
	}
	p := &dbdPlan{count: make([]int, len(q.meta.Columns))}
	p.rows = make([][]bool, len(q.models))
	for r := range q.models {
		row := make([]bool, len(q.meta.Columns))
		for _, ci := range cols {
			if dbDefaultZero(q.meta.Value(&q.models[r], ci)) {
				row[ci] = true
				p.count[ci]++
			}
		}
		p.rows[r] = row
	}
	q.dbd = p
	return q
}

// slice returns the plan of models lo..hi, for a chunk of a bulk insert.
func (p *dbdPlan) slice(lo, hi int) *dbdPlan {
	if p == nil || p.rows == nil {
		return p
	}
	out := &dbdPlan{count: make([]int, len(p.count))}
	out.rows = p.rows[lo:hi]
	for _, row := range out.rows {
		for ci, set := range row {
			if set {
				out.count[ci]++
			}
		}
	}
	return out
}

// dbDefaultRow reports whether the insert leaves column ci of model r to the
// database default.
func (q InsertQuery[T]) dbDefaultRow(r, ci int) bool {
	p := q.withDBD().dbd
	return r < len(p.rows) && p.rows[r][ci]
}

// dbDefaultCount counts the models whose column ci takes the database default.
func (q InsertQuery[T]) dbDefaultCount(ci int) int {
	if c := q.withDBD().dbd.count; ci < len(c) {
		return c[ci]
	}
	return 0
}

// dbDefaultDecision reports whether the INSERT column list drops column ci
// because every row takes its database default. It fails with ErrUnsupported
// when the dialect cannot read the stored value back (no RETURNING), when Copy
// would leave the column to the database without a read-back, and when some
// rows set the column and others do not on a statement that cannot put DEFAULT
// in the VALUES list of those rows (SQLite, Unnest, Copy).
func (q InsertQuery[T]) dbDefaultDecision(ci int) (drop bool, err error) {
	n := q.dbDefaultCount(ci)
	if n == 0 {
		return false, nil
	}
	name := q.meta.Columns[ci].Name
	if q.q != nil {
		if d := q.q.Dialect(); !d.Returning && !d.InsertReturning {
			return false, &Error{Kind: ErrUnsupported, Op: "insert", Table: q.meta.Table, Column: name,
				Err: fmt.Errorf("dbdefault column %q is zero and %s cannot read the stored value back (no RETURNING): set a value, or leave the column out with Omit and read it yourself", name, d.Name)}
		}
	}
	if q.i.noRowDefault {
		return false, &Error{Kind: ErrUnsupported, Op: "copy", Table: q.meta.Table, Column: name,
			Err: fmt.Errorf("dbdefault column %q is zero and Copy returns no rows to read the stored value back: set a value, or use Models", name)}
	}
	if n == len(q.models) {
		return true, nil
	}
	why := ""
	switch {
	case q.i.unnest:
		why = "Unnest cannot put DEFAULT in a row"
	case q.q != nil && !q.q.Dialect().DefaultInValues:
		why = string(q.q.Dialect().Name) + " cannot put DEFAULT in a multi-row VALUES list"
	}
	if why != "" {
		return false, &Error{Kind: ErrUnsupported, Op: "insert", Table: q.meta.Table, Column: name,
			Err: fmt.Errorf("rows mix zero and set values of dbdefault column %q and %s: insert the rows in separate statements, or give every row a value", name, why)}
	}
	return false, nil
}

// dbDefaultReadBack returns the dbdefault columns Exec reads back after the
// statement: those at least one model leaves to the database. It is empty when
// the caller asked for RETURNING itself (Rows reads the columns it names).
func (q InsertQuery[T]) dbDefaultReadBack() []int {
	if q.i.returning.active() || len(q.back) != len(q.models) {
		return nil
	}
	var out []int
	for ci := range q.meta.Columns {
		if q.dbDefaultCount(ci) > 0 {
			out = append(out, ci)
		}
	}
	return out
}

// later runs apply once the statement (all chunks) has committed when the
// query collects write-backs, and at once otherwise.
func (q InsertQuery[T]) later(apply func()) {
	if q.pend != nil {
		*q.pend = append(*q.pend, apply)
		return
	}
	apply()
}

// execReadBack runs a model insert that leaves dbdefault columns to the
// database, asking RETURNING for them (and for the generated key on MariaDB),
// and writes the stored values into the caller's models once the statement has
// committed. Rows are matched by position; when a conflict action skipped some
// rows (DO NOTHING) the count differs and the insert fails, see execSingle.
func (q InsertQuery[T]) execReadBack(ctx context.Context, cols []int) (Result, error) {
	idCol := -1
	if q.wantsGeneratedKey() {
		idCol = q.meta.Identity
	}
	all := slices.Clone(cols)
	if idCol >= 0 {
		all = append(all, idCol)
	}
	refs := make([]ColumnRef, len(all))
	for i, ci := range all {
		refs[i] = NewColumn[string](q.meta.Table, q.meta.Columns[ci].Name, 0)
	}
	q2 := q
	q2.i.returning = returningSet{cols: refs}
	c, err := q2.SQL()
	if err != nil {
		return Result{}, err
	}
	rows, err := runReturning(ctx, q.q, "insert", q.meta, q2.i.returning, c)
	if err != nil {
		return Result{}, err
	}
	rows.noHooks = true // the scanned models hold only the returned columns
	var got []T
	for rows.Next() {
		var g T
		if err := rows.ScanModel(&g); err != nil {
			_ = rows.Close()
			return Result{}, err
		}
		got = append(got, g)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Result{}, err
	}
	if err := rows.Close(); err != nil {
		return Result{}, err
	}
	res := Result{RowsAffected: int64(len(got))}
	if idCol >= 0 && len(got) == 1 {
		switch v := reflect.ValueOf(q.meta.Value(&got[0], idCol)); v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			res.InsertID = v.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			res.InsertID = int64(v.Uint()) // #nosec G115 -- an auto-increment key is positive
		}
	}
	if len(got) < len(q.models) {
		if err := q.shortfall(len(got)); err != nil {
			return Result{}, err
		}
		if len(q.models) == 1 {
			return res, nil // skipped by the conflict action: the model stays as it was
		}
	}
	for r := range got {
		if q.back[r] == nil {
			continue
		}
		for _, ci := range all {
			if ci != idCol && !q.dbDefaultRow(r, ci) {
				continue
			}
			dst, src, col := q.back[r], &got[r], ci
			q.later(func() { copyField(q.meta, dst, src, col) })
		}
	}
	return res, nil
}

// shortfall decides what fewer RETURNING rows than models mean for a read-back.
// A single model that returns no row was skipped, and a statement without a
// conflict action can lose rows only to a trigger or rule: both are tolerated
// and the rows that did return are written back by position. With a conflict
// action and several models the returned rows cannot be matched to the models,
// so the insert fails (and is rolled back, to the savepoint inside a *Tx)
// instead of leaving models stale. A pinned OnConn handle cannot roll back, so
// it is tolerated there too and the write-back is partial.
func (q InsertQuery[T]) shortfall(got int) error {
	total := len(q.models)
	if got >= total || total == 1 || !(q.i.conflict != conflictNone || q.i.upsert) {
		return nil
	}
	if db, ok := q.q.(*DB); ok && db.pinned {
		return nil
	}
	return &Error{Kind: ErrUnsupported, Op: "insert", Table: q.meta.Table,
		Err: fmt.Errorf("the statement returned %d rows for %d models, so the stored dbdefault values cannot be matched to the models: give those columns a value, or read them with Returning and Rows; the write is rolled back", got, total)}
}

// copyField stores the field of src for column ci into dst.
func copyField[T any](meta *Meta[T], dst, src *T, ci int) {
	d, s := reflect.ValueOf(meta.Ptr(dst, ci)), reflect.ValueOf(meta.Ptr(src, ci))
	if d.Kind() != reflect.Pointer || d.IsNil() || s.Kind() != reflect.Pointer || s.IsNil() || d.Type() != s.Type() {
		return
	}
	d.Elem().Set(s.Elem())
}

// dbDefaultZero reports whether a field value counts as unset: the zero value
// of its type, a nil pointer, or a value whose IsZero method says so
// (time.Time). A pointer to a zero value is set.
func dbDefaultZero(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		return rv.IsNil()
	}
	if z, ok := v.(interface{ IsZero() bool }); ok {
		return z.IsZero()
	}
	return rv.IsZero()
}
