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
// insert may write it. Like everything here it runs on insertRef, the
// non-generic view of the query.
func (r insertRef) dbDefaultCol(ci int) bool {
	return r.m.Columns[ci].Flags&FlagDBDefault != 0 && r.writable(ci) &&
		ci != r.m.CreatedAt && ci != r.m.UpdatedAt
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

// withDBD returns r with its dbdefault plan computed. A column listed in Omit
// keeps its Go-side default and never takes the database default.
func (r insertRef) withDBD(a insAccess) insertRef {
	if r.dbd != nil {
		return r
	}
	var cols []int
	for ci := range r.m.Columns {
		if r.dbDefaultCol(ci) && !slices.Contains(r.i.omit, r.m.Columns[ci].Name) {
			cols = append(cols, ci)
		}
	}
	if len(cols) == 0 {
		r.dbd = noDBD // no allocation for the common model without dbdefault columns
		return r
	}
	p := &dbdPlan{count: make([]int, len(r.m.Columns))}
	p.rows = make([][]bool, r.n)
	for row := range r.n {
		flags := make([]bool, len(r.m.Columns))
		for _, ci := range cols {
			if dbDefaultZero(a.val(row, ci)) {
				flags[ci] = true
				p.count[ci]++
			}
		}
		p.rows[row] = flags
	}
	r.dbd = p
	return r
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

// dbDefaultRow reports whether the insert leaves column ci of model row to the
// database default. The plan is computed by withDBD before anything asks.
func (r insertRef) dbDefaultRow(row, ci int) bool {
	p := r.dbd
	return p != nil && row < len(p.rows) && p.rows[row][ci]
}

// dbDefaultCount counts the models whose column ci takes the database default.
func (r insertRef) dbDefaultCount(ci int) int {
	if r.dbd == nil {
		return 0
	}
	if c := r.dbd.count; ci < len(c) {
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
//
//go:noinline
func (r insertRef) dbDefaultDecision(ci int) (drop bool, err error) {
	n := r.dbDefaultCount(ci)
	if n == 0 {
		return false, nil
	}
	name := r.m.Columns[ci].Name
	if r.q != nil {
		if d := r.q.Dialect(); !d.Returning && !d.InsertReturning {
			return false, &Error{Kind: ErrUnsupported, Op: "insert", Table: r.m.Table, Column: name,
				Err: fmt.Errorf("dbdefault column %q is zero and %s cannot read the stored value back (no RETURNING): set a value, or leave the column out with Omit and read it yourself", name, d.Name)}
		}
	}
	if r.i.noRowDefault {
		return false, &Error{Kind: ErrUnsupported, Op: "copy", Table: r.m.Table, Column: name,
			Err: fmt.Errorf("dbdefault column %q is zero and Copy returns no rows to read the stored value back: set a value, or use Models", name)}
	}
	if n == r.n {
		return true, nil
	}
	why := ""
	switch {
	case r.i.unnest:
		why = "Unnest cannot put DEFAULT in a row"
	case r.q != nil && !r.q.Dialect().DefaultInValues:
		why = string(r.q.Dialect().Name) + " cannot put DEFAULT in a multi-row VALUES list (SQL renders one statement; Exec and Rows split such rows into several)"
	}
	if why != "" {
		return false, &Error{Kind: ErrUnsupported, Op: "insert", Table: r.m.Table, Column: name,
			Err: fmt.Errorf("rows mix zero and set values of dbdefault column %q and %s: insert the rows in separate statements, or give every row a value", name, why)}
	}
	return false, nil
}

// dbDefaultReadBack returns the dbdefault columns Exec reads back after the
// statement: those at least one model leaves to the database. It is empty when
// the caller asked for RETURNING itself (Rows reads the columns it names).
func (r insertRef) dbDefaultReadBack() []int {
	if r.i.returning.active() || r.nBack != r.n {
		return nil
	}
	var out []int
	for ci := range r.m.Columns {
		if r.dbDefaultCount(ci) > 0 {
			out = append(out, ci)
		}
	}
	return out
}

// later runs apply once the statement (all chunks) has committed when the
// query collects write-backs in pend, and at once otherwise.
func later(pend *[]func(), apply func()) {
	if pend != nil {
		*pend = append(*pend, apply)
		return
	}
	apply()
}

// readBackPlan is the RETURNING list an insert with dbdefault columns reads
// back: idCol is the generated key (MariaDB), all the columns copied into the
// models, keyCols the columns that match a short result to the models.
type readBackPlan struct {
	idCol     int
	all       []int
	keyCols   []int
	returning returningSet
}

// readBackPlan resolves what execReadBack asks RETURNING for.
func (r insertRef) readBackPlan(cols []int, a insAccess) readBackPlan {
	idCol := -1
	if r.wantsGeneratedKey(a) {
		idCol = r.m.Identity
	}
	all := slices.Clone(cols)
	if idCol >= 0 {
		all = append(all, idCol)
	}
	keyCols := r.matchKey(cols, a) // lets a short bulk result be matched by key
	ret := slices.Clone(all)
	for _, ci := range keyCols {
		if !slices.Contains(ret, ci) {
			ret = append(ret, ci)
		}
	}
	refs := make([]ColumnRef, len(ret))
	for i, ci := range ret {
		refs[i] = NewColumn[string](r.m.Table, r.m.Columns[ci].Name, 0)
	}
	return readBackPlan{idCol: idCol, all: all, keyCols: keyCols, returning: returningSet{cols: refs}}
}

// execReadBack runs a model insert that leaves dbdefault columns to the
// database, asking RETURNING for them (and for the generated key on MariaDB),
// and writes the stored values into the caller's models once the statement has
// committed. Rows are matched by position when every model came back, by key
// when a bulk insert came back short and the key is known, and otherwise the
// insert fails, see execSingle and countMismatch.
func (q InsertQuery[T]) execReadBack(ctx context.Context, cols []int) (Result, error) {
	ms, back, meta, pend := q.models, q.back, q.meta, q.pend
	var got []T
	a := insAccess{
		value:   func(row, ci int) any { return meta.Value(&ms[row], ci) },
		zero:    func(ci int) (any, bool) { return zeroValue(meta, ci) },
		hasBack: func(row int) bool { return back[row] != nil },
		got:     func(g, ci int) any { return meta.Value(&got[g], ci) },
		queueCopy: func(row, g, ci int) {
			dst, src := back[row], &got[g]
			later(pend, func() { copyField(meta, dst, src, ci) })
		},
	}
	r := q.ref()
	rp := r.readBackPlan(cols, a)
	q2 := q
	q2.i.returning = rp.returning
	c, err := q2.SQL()
	if err != nil {
		return Result{}, err
	}
	rows, err := runReturning(ctx, q.q, "insert", q.meta, q2.i.returning, c)
	if err != nil {
		return Result{}, err
	}
	rows.noHooks = true // the scanned models hold only the returned columns
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
	return r.finishReadBack(rp, cols, len(got), a)
}

// finishReadBack matches the nGot rows an insert returned to its models and
// queues the stored values for the caller's models.
func (r insertRef) finishReadBack(rp readBackPlan, cols []int, nGot int, a insAccess) (Result, error) {
	res := Result{RowsAffected: int64(nGot)}
	if rp.idCol >= 0 && nGot == 1 {
		switch v := reflect.ValueOf(a.got(0, rp.idCol)); v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			res.InsertID = v.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			res.InsertID = int64(v.Uint()) // #nosec G115 -- an auto-increment key is positive
		}
	}
	total := r.n
	write := func(row, g int) {
		if !a.hasBack(row) {
			return
		}
		for _, ci := range rp.all {
			if ci != rp.idCol && !r.dbDefaultRow(row, ci) {
				continue
			}
			a.queueCopy(row, g, ci)
		}
	}
	switch {
	case nGot == total:
		for g := range nGot {
			write(g, g)
		}
	case total == 1:
		// skipped by the conflict action: the model stays as it was
	case rp.keyCols == nil:
		return Result{}, r.countMismatch(nGot)
	default:
		byKey := make(map[string]int, total)
		for i := range total {
			k := idKeyOf(func(ci int) any { return a.val(i, ci) }, rp.keyCols)
			if _, dup := byKey[k]; !dup {
				byKey[k] = i
			}
		}
		matched := make([]bool, total)
		for g := range nGot {
			i, ok := byKey[idKeyOf(func(ci int) any { return a.got(g, ci) }, rp.keyCols)]
			if !ok {
				return Result{}, r.verr("", "a RETURNING row of %s matched no model on %s: the key columns must read back exactly as written (citext, char(n) and numeric columns may not); %s", r.m.Table, strings.Join(keyNamesOf(r.m.Columns, rp.keyCols), ", "), r.rollbackNote())
			}
			matched[i] = true
			write(i, g)
		}
		if err := r.unread(matched, cols); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}

// isPinned reports whether the statement runs on a pinned OnConn handle, which
// cannot open a transaction and so cannot roll an insert back.
func (r insertRef) isPinned() bool {
	db, ok := r.q.(*DB)
	return ok && db.pinned
}

// rollbackNote says what became of the write when a read-back fails.
func (r insertRef) rollbackNote() string {
	if r.isPinned() {
		return "the returned rows were stored (a pinned OnConn handle cannot roll back) and no model was updated"
	}
	return "the write is rolled back (to a savepoint inside a Tx) and no model was updated"
}

// countMismatch is the error for a bulk insert that returned fewer rows than
// models when the rows cannot be matched to the models by key.
func (r insertRef) countMismatch(got int) error {
	return &Error{Kind: ErrUnsupported, Op: "insert", Table: r.m.Table,
		Err: fmt.Errorf("the statement returned %d rows for %d models, so the stored dbdefault values cannot be matched to the models: give those columns a value, or read them with Returning and Rows; %s", got, r.n, r.rollbackNote())}
}

// unread fails when ON CONFLICT DO NOTHING left a model without its stored
// dbdefault values: matched[i] says whether model i came back. Other conflict
// actions and statements without one leave unmatched models untouched.
func (r insertRef) unread(matched []bool, cols []int) error {
	if r.i.conflict != conflictNothing || r.n < 2 {
		return nil // a single skipped Model just reports 0 rows
	}
	for i, m := range matched {
		if m {
			continue
		}
		for _, ci := range cols {
			if r.dbDefaultRow(i, ci) {
				return &Error{Kind: ErrUnsupported, Op: "insert", Table: r.m.Table, Column: r.m.Columns[ci].Name,
					Err: fmt.Errorf("ON CONFLICT DO NOTHING skipped model %d, so its dbdefault column %q stays unread: give the column a value, or read it with Returning and Rows; %s", i, r.m.Columns[ci].Name, r.rollbackNote())}
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
func (r insertRef) matchKey(cols []int, a insAccess) []int {
	if r.n < 2 {
		return nil
	}
	key := r.m.PK
	if rq, err := r.resolveUpsert(a); err == nil && len(rq.i.target) > 0 {
		key = key[:0:0]
		for _, n := range rq.i.target {
			ci := r.column(n)
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
		if slices.Contains(cols, ci) || !r.writable(ci) || slices.Contains(r.i.omit, r.m.Columns[ci].Name) {
			return nil
		}
	}
	return key
}

// copyField stores the field of src for column ci into dst.
func copyField[T any](meta *Meta[T], dst, src *T, ci int) {
	copyPointee(meta.Ptr(dst, ci), meta.Ptr(src, ci))
}

// copyPointee stores what the field pointer sp points to into the field
// pointer dp, when both point to the same type.
func copyPointee(dp, sp any) {
	d, s := reflect.ValueOf(dp), reflect.ValueOf(sp)
	if d.Kind() != reflect.Pointer || d.IsNil() || s.Kind() != reflect.Pointer || s.IsNil() || d.Type() != s.Type() {
		return
	}
	d.Elem().Set(s.Elem())
}

// nilPointer reports whether a field value is a nil pointer. Update.Model uses
// it to leave a nil dbdefault pointer out of SET.
func nilPointer(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
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
