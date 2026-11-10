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

// dbDefaultRow reports whether the insert leaves column ci of model m to the
// database default: the column is tagged dbdefault, not listed in Omit (an
// omitted column keeps its Go-side default), and the field holds its zero
// value.
func (q InsertQuery[T]) dbDefaultRow(m *T, ci int) bool {
	return q.dbDefaultCol(ci) && !slices.Contains(q.i.omit, q.meta.Columns[ci].Name) &&
		dbDefaultZero(q.meta.Value(m, ci))
}

// dbDefaultCount counts the models whose column ci takes the database default.
func (q InsertQuery[T]) dbDefaultCount(ci int) int {
	if !q.dbDefaultCol(ci) {
		return 0
	}
	n := 0
	for r := range q.models {
		if q.dbDefaultRow(&q.models[r], ci) {
			n++
		}
	}
	return n
}

// dbDefaultDecision reports whether the INSERT column list drops column ci
// because every row takes its database default. It fails with ErrUnsupported
// when the dialect cannot read the stored value back (no RETURNING), and when
// some rows set the column and others do not on a statement that cannot put
// DEFAULT in the VALUES list of those rows (SQLite, Unnest, Copy).
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
	if n == len(q.models) {
		return true, nil
	}
	why := ""
	switch {
	case q.i.unnest || q.i.noRowDefault:
		why = "this statement cannot put DEFAULT in a multi-row VALUES list"
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

// execReadBack runs a model insert that leaves dbdefault columns to the
// database, asking RETURNING for them (and for the generated key on MariaDB),
// and writes the stored values into the caller's models once the statement has
// committed. Rows are matched by position; when a conflict action skipped some
// rows (DO NOTHING) the count differs and no model is touched.
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
	if len(got) != len(q.models) {
		return res, nil
	}
	for r := range got {
		if q.back[r] == nil {
			continue
		}
		for _, ci := range all {
			if ci != idCol && !q.dbDefaultRow(&q.models[r], ci) {
				continue
			}
			dst, src, col := q.back[r], &got[r], ci
			apply := func() { copyField(q.meta, dst, src, col) }
			if q.pend != nil {
				*q.pend = append(*q.pend, apply) // after the statement commits
			} else {
				apply()
			}
		}
	}
	return res, nil
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
