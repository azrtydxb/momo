// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
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
// committed. Rows are matched by position when every model came back, by key
// when a bulk insert came back short and the key is known, and otherwise the
// insert fails, see execSingle and countMismatch.
func (q InsertQuery[T]) execReadBack(ctx context.Context, cols []int) (Result, error) {
	idCol := -1
	if q.wantsGeneratedKey() {
		idCol = q.meta.Identity
	}
	all := slices.Clone(cols)
	if idCol >= 0 {
		all = append(all, idCol)
	}
	keyCols := q.matchKey(cols) // lets a short bulk result be matched by key
	ret := slices.Clone(all)
	for _, ci := range keyCols {
		if !slices.Contains(ret, ci) {
			ret = append(ret, ci)
		}
	}
	refs := make([]ColumnRef, len(ret))
	for i, ci := range ret {
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
	total := len(q.models)
	write := func(r int, src *T) {
		if q.back[r] == nil {
			return
		}
		for _, ci := range all {
			if ci != idCol && !q.dbDefaultRow(r, ci) {
				continue
			}
			dst, col := q.back[r], ci
			q.later(func() { copyField(q.meta, dst, src, col) })
		}
	}
	switch {
	case len(got) == total:
		for r := range got {
			write(r, &got[r])
		}
	case total == 1:
		// skipped by the conflict action: the model stays as it was
	case keyCols == nil:
		return Result{}, q.countMismatch(len(got))
	default:
		byKey := make(map[string]int, total)
		for i := range q.models {
			if _, dup := byKey[idKey(q.meta, &q.models[i], keyCols)]; !dup {
				byKey[idKey(q.meta, &q.models[i], keyCols)] = i
			}
		}
		matched := make([]bool, total)
		for g := range got {
			i, ok := byKey[idKey(q.meta, &got[g], keyCols)]
			if !ok {
				return Result{}, q.verr("", "a RETURNING row of %s matched no model on %s: the key columns must read back exactly as written (citext, char(n) and numeric columns may not); %s", q.meta.Table, strings.Join(keyNames(q.meta, keyCols), ", "), q.rollbackNote())
			}
			matched[i] = true
			write(i, &got[g])
		}
		if err := q.unread(matched, cols); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}

// isPinned reports whether the statement runs on a pinned OnConn handle, which
// cannot open a transaction and so cannot roll an insert back.
func (q InsertQuery[T]) isPinned() bool {
	db, ok := q.q.(*DB)
	return ok && db.pinned
}

// rollbackNote says what became of the write when a read-back fails.
func (q InsertQuery[T]) rollbackNote() string {
	if q.isPinned() {
		return "the returned rows were stored (a pinned OnConn handle cannot roll back) and no model was updated"
	}
	return "the write is rolled back (to a savepoint inside a Tx) and no model was updated"
}

// countMismatch is the error for a bulk insert that returned fewer rows than
// models when the rows cannot be matched to the models by key.
func (q InsertQuery[T]) countMismatch(got int) error {
	return &Error{Kind: ErrUnsupported, Op: "insert", Table: q.meta.Table,
		Err: fmt.Errorf("the statement returned %d rows for %d models, so the stored dbdefault values cannot be matched to the models: give those columns a value, or read them with Returning and Rows; %s", got, len(q.models), q.rollbackNote())}
}

// unread fails when ON CONFLICT DO NOTHING left a model without its stored
// dbdefault values: matched[i] says whether model i came back. Other conflict
// actions and statements without one leave unmatched models untouched.
func (q InsertQuery[T]) unread(matched []bool, cols []int) error {
	if q.i.conflict != conflictNothing || len(q.models) < 2 {
		return nil // a single skipped Model just reports 0 rows
	}
	for i, m := range matched {
		if m {
			continue
		}
		for _, ci := range cols {
			if q.dbDefaultRow(i, ci) {
				return &Error{Kind: ErrUnsupported, Op: "insert", Table: q.meta.Table, Column: q.meta.Columns[ci].Name,
					Err: fmt.Errorf("ON CONFLICT DO NOTHING skipped model %d, so its dbdefault column %q stays unread: give the column a value, or read it with Returning and Rows; %s", i, q.meta.Columns[ci].Name, q.rollbackNote())}
			}
		}
	}
	return nil
}

// matchKey returns the columns that identify a model in the RETURNING rows of
// a bulk insert: the conflict target, else the primary key. It is nil when the
// rows cannot be matched by key: a single model, no key, or a key column the
// statement does not write from the model (omitted, identity, read-only) or
// that takes its database default in some row.
func (q InsertQuery[T]) matchKey(cols []int) []int {
	if len(q.models) < 2 {
		return nil
	}
	key := q.meta.PK
	if rq, err := q.resolveUpsert(); err == nil && len(rq.i.target) > 0 {
		key = key[:0:0]
		for _, n := range rq.i.target {
			ci := q.column(n)
			if ci < 0 {
				return nil
			}
			key = append(key, ci)
		}
	}
	if len(key) == 0 {
		return nil
	}
	for _, ci := range key {
		if slices.Contains(cols, ci) || !q.writable(ci) || slices.Contains(q.i.omit, q.meta.Columns[ci].Name) {
			return nil
		}
	}
	return key
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
