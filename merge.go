// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

const (
	// pgMergeMin is server_version_num of PostgreSQL 15, the first with MERGE.
	pgMergeMin = 150000
	// pgMergeReturningMin is server_version_num of PostgreSQL 17, the first
	// with MERGE ... RETURNING.
	pgMergeReturningMin = 170000

	serverVersionQuery = "SELECT current_setting('server_version_num')::int"
)

// mergeGate returns ErrUnsupported when serverVersion (server_version_num) is
// below 150000, or when returning is true and serverVersion is below 170000.
// It returns nil otherwise.
func mergeGate(serverVersion int, returning bool) error {
	switch {
	case serverVersion < pgMergeMin:
		return &Error{Kind: ErrUnsupported, Op: "merge",
			Err: fmt.Errorf("MERGE needs PostgreSQL 15 or later, the server reports %d", serverVersion)}
	case returning && serverVersion < pgMergeReturningMin:
		return &Error{Kind: ErrUnsupported, Op: "merge",
			Err: fmt.Errorf("MERGE ... RETURNING needs PostgreSQL 17 or later, the server reports %d", serverVersion)}
	}
	return nil
}

// serverVersionOf reads server_version_num for a MERGE. A plain DB asks the
// driver (ServerVersioner); a Tx or a pinned handle runs the query on its own
// connection, so it neither needs a second pooled connection nor leaves the
// session. A driver that cannot report its version fails the MERGE closed.
func serverVersionOf(ctx context.Context, q Querier) (int, error) {
	if db, ok := q.(*DB); ok && !db.pinned {
		sv, ok := db.drv.(ServerVersioner)
		if !ok {
			return 0, &Error{Kind: ErrUnsupported, Op: "merge",
				Err: errors.New("the driver does not report the server version (momo.ServerVersioner), so the MERGE version gate cannot run")}
		}
		v, err := sv.ServerVersion(ctx)
		return v, db.ex.wrapErr("merge", "", err)
	}
	rows, err := q.conn().query(ctx, "query", "", "", Compiled{SQL: serverVersionQuery})
	if err != nil {
		return 0, err
	}
	var v int
	if rows.Next() {
		err = rows.Scan(&v)
	} else {
		err = rows.Err()
		if err == nil {
			err = errors.New("momo: server_version_num returned no row")
		}
	}
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	return v, err
}

// MergeQuery builds a MERGE into one model table. It is a value: every method
// returns a copy.
type MergeQuery[T any] struct {
	q      Querier
	meta   *Meta[T]
	source Subquery
	on     []Predicate

	matched    []Assignment
	hasMatched bool
	insCols    []ColumnRef
	insVals    []Expr
	hasInsert  bool
	returning  returningSet
}

// Merge starts a MERGE into T from a source, which the statement names src.
// PostgreSQL 15 or later only (dialect.Merge); RETURNING on MERGE needs
// PostgreSQL 17 or later. The dialect is checked when the statement is built,
// before any server call, and the server version is read through
// ServerVersioner (or, on a Tx or an OnConn handle, on that connection) just
// before the statement runs. A driver that cannot report its version fails
// with ErrUnsupported. The version is read on every Exec or Rows call.
//
// Matched rows are updated through WhenMatchedUpdate, which keeps the
// updated-at and version columns up to date like Update does; the values of
// WhenNotMatchedInsert come from the source, so field validators do not run on
// them. Nothing is written without On, and at least one WHEN clause is
// required.
func Merge[T any, PT Modeler[T]](q Querier, source Subquery) MergeQuery[T] {
	return MergeQuery[T]{q: q, meta: metaOf[T, PT](), source: source}
}

// On sets the join condition between the target table and src. Several
// predicates are ANDed. Reference the source as src, for example with
// TypedSQL.
func (m MergeQuery[T]) On(ps ...Predicate) MergeQuery[T] {
	m.on = append(slices.Clip(m.on), ps...)
	return m
}

// WhenMatchedUpdate adds WHEN MATCHED THEN UPDATE SET with the assignments.
// The primary key, version, soft-delete, created-at and updated-at columns
// cannot be assigned directly, as in Update.
func (m MergeQuery[T]) WhenMatchedUpdate(assigns ...Assignment) MergeQuery[T] {
	m.matched = slices.Clone(assigns)
	m.hasMatched = true
	return m
}

// WhenNotMatchedInsert adds WHEN NOT MATCHED THEN INSERT with one value
// expression per column. An identity column in the list is written with
// OVERRIDING SYSTEM VALUE.
func (m MergeQuery[T]) WhenNotMatchedInsert(cols []ColumnRef, values ...Expr) MergeQuery[T] {
	m.insCols = slices.Clone(cols)
	m.insVals = slices.Clone(values)
	m.hasInsert = true
	return m
}

// Returning appends a RETURNING list; read the rows with Rows. It needs
// PostgreSQL 17 or later.
func (m MergeQuery[T]) Returning(cols ...ColumnRef) MergeQuery[T] {
	m.returning = m.returning.with(cols...)
	return m
}

func (m MergeQuery[T]) verr(col, format string, args ...any) error {
	return &Error{Kind: ErrValidation, Op: "merge", Table: m.meta.Table, Column: col, Err: fmt.Errorf(format, args...)}
}

// SQL renders the statement without executing it. It checks the dialect but
// not the server version, which needs a connection.
func (m MergeQuery[T]) SQL() (Compiled, error) {
	if m.q == nil {
		return Compiled{}, errors.New("momo: query has no Querier")
	}
	return compile(m.q.Dialect(), m.render)
}

// mergeInsert is the validated insert side of a MERGE.
type mergeInsert struct {
	cols     []int
	vals     []Expr
	tsCols   []int
	tsAssign []Assignment
	override bool
}

func (m MergeQuery[T]) planInsert(ins InsertQuery[T]) (mergeInsert, error) {
	var p mergeInsert
	if len(m.insCols) == 0 || len(m.insCols) != len(m.insVals) {
		return p, m.verr("", "WhenNotMatchedInsert needs one value per column and at least one column")
	}
	seen := map[int]struct{}{}
	for i, c := range m.insCols {
		name := c.ColumnName()
		ci := ins.column(name)
		if ci < 0 {
			return p, m.verr(name, "WhenNotMatchedInsert: column %q is not mapped by %s", name, m.meta.Table)
		}
		f := m.meta.Columns[ci].Flags
		if f&FlagReadOnly != 0 {
			return p, m.verr(name, "WhenNotMatchedInsert: column %q is read-only and is never written", name)
		}
		if _, dup := seen[ci]; dup {
			return p, m.verr(name, "WhenNotMatchedInsert: column %q listed twice", name)
		}
		if m.insVals[i] == nil {
			return p, m.verr(name, "WhenNotMatchedInsert: nil value for column %q", name)
		}
		seen[ci] = struct{}{}
		p.cols = append(p.cols, ci)
		p.vals = append(p.vals, m.insVals[i])
		p.override = p.override || f&FlagIdentity != 0
	}
	tc, ta, err := ins.timestampAssigns(seen, m.q.conn().now())
	if err != nil {
		return p, err
	}
	p.tsCols, p.tsAssign = tc, ta
	return p, nil
}

func (m MergeQuery[T]) planMatched(upd UpdateQuery[T]) ([]Assignment, error) {
	if len(m.matched) == 0 {
		return nil, m.verr("", "WhenMatchedUpdate needs at least one assignment")
	}
	var sets []Assignment
	for _, a := range m.matched {
		if role := upd.reservedRole(a.col); role != "" {
			return nil, m.verr(a.col, "WhenMatchedUpdate: column %q is the %s column and cannot be set directly", a.col, role)
		}
		ci := slices.IndexFunc(m.meta.Columns, func(c ColumnMeta) bool { return c.Name == a.col })
		if ci < 0 {
			return nil, m.verr(a.col, "WhenMatchedUpdate: column %q is not mapped by %s", a.col, m.meta.Table)
		}
		if !columnAssignable(writeUpdate, m.meta.Columns[ci].Flags) {
			return nil, m.verr(a.col, "WhenMatchedUpdate: column %q cannot be written by UPDATE", a.col)
		}
		if a.kind == setExcluded {
			return nil, m.verr(a.col, "WhenMatchedUpdate: SetExcluded is only valid in an upsert")
		}
		if slices.ContainsFunc(sets, func(s Assignment) bool { return s.col == a.col }) {
			return nil, m.verr(a.col, "WhenMatchedUpdate: column %q assigned twice", a.col)
		}
		sets = append(sets, a)
	}
	sets, err := upd.applyFieldHooks(sets)
	if err != nil {
		return nil, err
	}
	if ci := m.meta.UpdatedAt; ci >= 0 && columnAssignable(writeUpdate, m.meta.Columns[ci].Flags) {
		if err := checkTimestampField(m.meta, ci); err != nil {
			return nil, m.verr(m.meta.Columns[ci].Name, "%v", err)
		}
		if v, ok := autoTimestamp(m.meta, ci, m.q.conn().now()); ok {
			sets = append(sets, Assignment{col: m.meta.Columns[ci].Name, kind: setValue, val: v, flags: m.meta.Columns[ci].Flags})
		}
	}
	return sets, nil
}

func (m MergeQuery[T]) render(b *Builder) {
	d := b.Dialect()
	switch {
	case !d.Merge:
		b.unsupported("MERGE")
		return
	case m.meta.Table == "":
		b.SetErr(m.verr("", "%T has no table", *new(T)))
		return
	case m.source == nil:
		b.SetErr(m.verr("", "Merge needs a source"))
		return
	case len(m.on) == 0:
		b.SetErr(m.verr("", "Merge needs On: a MERGE without a join condition is refused"))
		return
	case !m.hasMatched && !m.hasInsert:
		b.SetErr(m.verr("", "Merge needs WhenMatchedUpdate or WhenNotMatchedInsert"))
		return
	}
	upd := UpdateQuery[T]{q: m.q, meta: m.meta, assigns: m.matched}
	ins := InsertQuery[T]{q: m.q, meta: m.meta}
	var sets []Assignment
	var ip mergeInsert
	var err error
	if m.hasMatched {
		if sets, err = m.planMatched(upd); err != nil {
			b.SetErr(err)
			return
		}
	}
	if m.hasInsert {
		if ip, err = m.planInsert(ins); err != nil {
			b.SetErr(err)
			return
		}
	}
	target := TableRef{Name: m.meta.Table, Schema: m.meta.Schema}
	b.withScope([]string{target.Qualifier(), "src"}, func() {
		b.WriteString("MERGE INTO ")
		target.appendFrom(b)
		b.WriteString(" USING ")
		m.source.AppendSQL(b)
		b.WriteString(" AS ")
		b.Ident("src")
		b.WriteString(" ON ")
		appendAnd(b, m.on)
		if m.hasMatched {
			b.WriteString(" WHEN MATCHED THEN UPDATE SET ")
			for i, a := range sets {
				if i > 0 {
					b.WriteString(", ")
				}
				upd.appendAssignment(b, a)
			}
			if vi := m.meta.Version; vi >= 0 {
				b.WriteString(", ")
				b.Ident(m.meta.Columns[vi].Name)
				b.WriteString(" = ")
				b.QualifiedIdent(target.Qualifier(), m.meta.Columns[vi].Name)
				b.WriteString(" + 1")
			}
		}
		if m.hasInsert {
			m.renderInsert(b, ins, ip)
		}
	})
	renderReturning(b, m.meta, m.returning, func(col string) error {
		return m.verr(col, "Returning: column %q is not mapped by %s", col, m.meta.Table)
	})
}

func (m MergeQuery[T]) renderInsert(b *Builder, ins InsertQuery[T], p mergeInsert) {
	b.WriteString(" WHEN NOT MATCHED THEN INSERT (")
	for i, ci := range append(slices.Clone(p.cols), p.tsCols...) {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(m.meta.Columns[ci].Name)
	}
	b.writeByte(')')
	if p.override {
		b.WriteString(" OVERRIDING SYSTEM VALUE")
	}
	b.WriteString(" VALUES (")
	for i, ci := range p.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.exprFor(m.meta.Columns[ci].Flags, p.vals[i])
	}
	for _, a := range p.tsAssign {
		b.WriteString(", ")
		ins.appendValue(b, a)
	}
	b.writeByte(')')
}

// prepare compiles the statement, then applies the server-version gate.
func (m MergeQuery[T]) prepare(ctx context.Context) (Compiled, error) {
	c, err := m.SQL()
	if err != nil {
		return Compiled{}, err
	}
	v, err := serverVersionOf(ctx, m.q)
	if err != nil {
		return Compiled{}, err
	}
	if err := mergeGate(v, m.returning.active()); err != nil {
		var me *Error
		if errors.As(err, &me) {
			me.Table = m.meta.Table
		}
		return Compiled{}, err
	}
	return c, nil
}

// Exec runs the MERGE and reports the rows it inserted or updated.
func (m MergeQuery[T]) Exec(ctx context.Context) (Result, error) {
	c, err := m.prepare(ctx)
	if err != nil {
		return Result{}, err
	}
	n, err := m.q.conn().exec(ctx, "merge", m.meta.Table, c)
	if err != nil {
		return Result{}, err
	}
	return Result{RowsAffected: n}, nil
}

// Rows runs the MERGE and returns its RETURNING rows. It fails with
// ErrValidation when no Returning list was set and with ErrUnsupported on a
// server older than PostgreSQL 17.
func (m MergeQuery[T]) Rows(ctx context.Context) (*ModelRows[T], error) {
	if m.q == nil {
		return nil, errors.New("momo: query has no Querier")
	}
	if !m.returning.active() {
		return nil, m.verr("", "Rows needs a Returning list")
	}
	c, err := m.prepare(ctx)
	if err != nil {
		return nil, err
	}
	return runReturning(ctx, m.q, "merge", m.meta, m.returning, c)
}

// bulkSpec is the VALUES list of BulkUpdate.
type bulkSpec struct {
	key  ColumnRef
	cols []ColumnRef
	rows [][]any
}

// BulkUpdate updates rows of T from a VALUES list keyed by key. Only listed
// keys change. Each row holds the key value first, then one value per column
// of cols, in order:
//
//	BulkUpdate[Document](db, DocID, []ColumnRef{DocTitle}, [][]any{{int64(2), "new"}})
//
// It renders UPDATE t SET c = v.c FROM (VALUES ...) AS v WHERE t.key = v.key
// with every value bound, so a row whose key is not in the table changes
// nothing. The VALUES list takes its column types from the table. A key
// listed twice applies one of its rows. The statement also maintains the
// updated-at and version columns, runs the OnUpdate defaults and the field
// validators over each row, and honours Where for extra conditions; Set and
// Model cannot be combined with it. PostgreSQL only (dialect.Merge gates
// both): other dialects fail with ErrUnsupported. A statement over the
// bind-parameter limit fails with ErrLimitExceeded.
func BulkUpdate[T any, PT Modeler[T]](q Querier, key ColumnRef, cols []ColumnRef, rows [][]any) UpdateQuery[T] {
	u := Update[T, PT](q)
	u.bulk = &bulkSpec{key: key, cols: slices.Clone(cols), rows: slices.Clone(rows)}
	return u
}

// bulkPlan is a validated BulkUpdate: the key and column indexes and the
// extra assignments every row gets.
type bulkPlan struct {
	key   int
	cols  []int
	extra []Assignment
}

func (u UpdateQuery[T]) planBulk() (bulkPlan, error) {
	var p bulkPlan
	s := u.bulk
	switch {
	case u.meta.Table == "":
		return p, u.fail(ErrValidation, errors.New("model has no table"))
	case len(u.assigns) > 0 || u.modelSet || u.softDelete:
		return p, u.fail(ErrValidation, errors.New("BulkUpdate cannot be combined with Set or Model"))
	case s.key == nil || len(s.cols) == 0:
		return p, u.fail(ErrValidation, errors.New("BulkUpdate needs a key and at least one column"))
	case len(s.rows) == 0:
		return p, u.fail(ErrValidation, errors.New("BulkUpdate needs at least one row"))
	}
	index := func(name string) int {
		return slices.IndexFunc(u.meta.Columns, func(c ColumnMeta) bool { return c.Name == name })
	}
	if p.key = index(s.key.ColumnName()); p.key < 0 {
		return p, u.fail(ErrValidation, fmt.Errorf("key column %q is not mapped by %s", s.key.ColumnName(), u.meta.Table))
	}
	for _, c := range s.cols {
		ci := index(c.ColumnName())
		switch {
		case ci < 0:
			return p, u.fail(ErrValidation, fmt.Errorf("column %q is not mapped by %s", c.ColumnName(), u.meta.Table))
		case ci == p.key || slices.Contains(p.cols, ci):
			return p, u.fail(ErrValidation, fmt.Errorf("column %q listed twice", c.ColumnName()))
		case u.reservedRole(c.ColumnName()) != "":
			return p, u.fail(ErrValidation, fmt.Errorf("column %q is the %s column and cannot be set directly", c.ColumnName(), u.reservedRole(c.ColumnName())))
		case !columnAssignable(writeUpdate, u.meta.Columns[ci].Flags):
			return p, u.fail(ErrValidation, fmt.Errorf("column %q cannot be written by UPDATE", c.ColumnName()))
		}
		p.cols = append(p.cols, ci)
	}
	for r, row := range s.rows {
		if len(row) != 1+len(p.cols) {
			return p, u.fail(ErrValidation, fmt.Errorf("row %d has %d values, want the key and %d columns", r, len(row), len(p.cols)))
		}
	}
	fp, err := newFieldPlan(u.q, u.meta)
	if err != nil {
		return p, err
	}
	if len(fp.hooks) > 0 {
		if p.extra, err = u.bulkExtras(fp, p.cols); err != nil {
			return p, err
		}
		for _, row := range s.rows {
			err := fp.validate("update", u.meta, func(ci int) (any, bool) {
				if j := slices.Index(p.cols, ci); j >= 0 {
					return row[1+j], true
				}
				for _, a := range p.extra {
					if a.col == u.meta.Columns[ci].Name {
						return assignedValue(a)
					}
				}
				return nil, false
			})
			if err != nil {
				return p, err
			}
		}
	}
	if ci := u.meta.UpdatedAt; ci >= 0 && columnAssignable(writeUpdate, u.meta.Columns[ci].Flags) {
		if err := checkTimestampField(u.meta, ci); err != nil {
			return p, u.fail(ErrValidation, err)
		}
		if v, ok := autoTimestamp(u.meta, ci, u.q.conn().now()); ok {
			p.extra = append(p.extra, Assignment{col: u.meta.Columns[ci].Name, kind: setValue, val: v, flags: u.meta.Columns[ci].Flags})
		}
	}
	return p, nil
}

// bulkExtras returns the OnUpdate defaults for the columns the rows do not list.
func (u UpdateQuery[T]) bulkExtras(fp fieldPlan[T], listed []int) ([]Assignment, error) {
	var out []Assignment
	var zero T
	for _, ci := range slices.Sorted(maps.Keys(fp.upd)) {
		if slices.Contains(listed, ci) {
			continue // an explicit value wins
		}
		c := u.meta.Columns[ci]
		if role := u.reservedRole(c.Name); role != "" {
			return nil, fieldError("update", u.meta.Table, c.Name, fmt.Errorf("OnUpdate: column %q is the %s column and cannot be set by a field hook", c.Name, role))
		}
		if !columnAssignable(writeUpdate, c.Flags) {
			return nil, fieldError("update", u.meta.Table, c.Name, fmt.Errorf("OnUpdate: column %q cannot be written by UPDATE", c.Name))
		}
		out = append(out, Assignment{col: c.Name, kind: setValue, val: fp.upd[ci](&zero), flags: c.Flags})
	}
	return out, nil
}

func (u UpdateQuery[T]) bulkSQL() (Compiled, error) {
	p, err := u.planBulk()
	if err != nil {
		return Compiled{}, err
	}
	d := u.q.Dialect()
	target := TableRef{Name: u.meta.Table, Schema: u.meta.Schema}
	c, err := compile(d, func(b *Builder) {
		if !b.Dialect().Merge {
			b.unsupported("bulk update from VALUES")
			return
		}
		src := "momo_v"
		names := make([]string, 0, 1+len(p.cols))
		names = append(names, u.meta.Columns[p.key].Name)
		for _, ci := range p.cols {
			names = append(names, u.meta.Columns[ci].Name)
		}
		b.withScope([]string{target.Qualifier(), src}, func() {
			b.WriteString("UPDATE ")
			target.appendFrom(b)
			b.WriteString(" SET ")
			for i, ci := range p.cols {
				if i > 0 {
					b.WriteString(", ")
				}
				b.Ident(u.meta.Columns[ci].Name)
				b.WriteString(" = ")
				b.QualifiedIdent(src, u.meta.Columns[ci].Name)
			}
			for _, a := range p.extra {
				b.WriteString(", ")
				u.appendAssignment(b, a)
			}
			if vi := u.meta.Version; vi >= 0 {
				b.WriteString(", ")
				b.Ident(u.meta.Columns[vi].Name)
				b.WriteString(" = ")
				b.QualifiedIdent(target.Qualifier(), u.meta.Columns[vi].Name)
				b.WriteString(" + 1")
			}
			// The empty SELECT gives the VALUES list the column types of the
			// table: an unknown-typed bind parameter takes the type of the
			// other UNION branch.
			b.WriteString(" FROM (SELECT ")
			for i, n := range names {
				if i > 0 {
					b.WriteString(", ")
				}
				b.Ident(n)
			}
			b.WriteString(" FROM ")
			target.appendFrom(b)
			b.WriteString(" WHERE false UNION ALL VALUES ")
			for r, row := range u.bulk.rows {
				if r > 0 {
					b.WriteString(", ")
				}
				b.writeByte('(')
				b.argFor(u.meta.Columns[p.key].Flags, row[0])
				for j, ci := range p.cols {
					b.WriteString(", ")
					b.argFor(u.meta.Columns[ci].Flags, row[1+j])
				}
				b.writeByte(')')
			}
			b.WriteString(") AS ")
			b.Ident(src)
			b.WriteString(" WHERE ")
			b.QualifiedIdent(target.Qualifier(), u.meta.Columns[p.key].Name)
			b.WriteString(" = ")
			b.QualifiedIdent(src, u.meta.Columns[p.key].Name)
			if len(u.where) > 0 {
				b.WriteString(" AND ")
				appendAnd(b, u.where)
			}
		})
		renderReturning(b, u.meta, u.returning, func(col string) error {
			return u.fail(ErrValidation, fmt.Errorf("Returning: column %q is not mapped by %s", col, u.meta.Table))
		})
	})
	if err != nil {
		return Compiled{}, err
	}
	if d.MaxParams > 0 && len(c.Args) > d.MaxParams {
		return Compiled{}, &Error{Kind: ErrLimitExceeded, Op: "update", Table: u.meta.Table,
			Err: fmt.Errorf("%d bind parameters exceed the %s limit of %d", len(c.Args), d.Name, d.MaxParams)}
	}
	return c, nil
}

// Truncate removes all rows of T's table, soft-deleted rows included, with no
// filter. PostgreSQL and MySQL run TRUNCATE TABLE (PostgreSQL keeps the
// identity sequence). SQLite runs DELETE FROM and deletes the table's
// sqlite_sequence row when that table exists, so a SQLite autoincrement key
// restarts. The two SQLite statements are atomic: on a DB they run in one
// transaction, on a Tx in a savepoint.
func Truncate[T any, PT Modeler[T]](ctx context.Context, q Querier) error {
	meta := metaOf[T, PT]()
	if meta.Table == "" {
		return &Error{Kind: ErrValidation, Op: "truncate", Err: errors.New("model has no table")}
	}
	d := q.Dialect()
	target := TableRef{Name: meta.Table, Schema: meta.Schema}
	verb := "TRUNCATE TABLE "
	if d.Name == dialect.SQLite {
		verb = "DELETE FROM "
	}
	c, err := compile(d, func(b *Builder) {
		b.WriteString(verb)
		target.appendFrom(b)
	})
	if err != nil {
		return err
	}
	if d.Name != dialect.SQLite {
		_, err = q.conn().exec(ctx, "truncate", meta.Table, c)
		return err
	}
	run := func(sq Querier) error {
		if _, err := sq.conn().exec(ctx, "truncate", meta.Table, c); err != nil {
			return err
		}
		return resetSQLiteSequence(ctx, sq, meta.Table)
	}
	if db, ok := q.(*DB); ok && db.pinned {
		return run(q) // a pinned connection cannot begin a transaction
	}
	return runAtomic(ctx, q, "truncate", meta.Table, run)
}

// resetSQLiteSequence deletes the table's sqlite_sequence row; the sequence
// table exists only once a table with AUTOINCREMENT was created.
func resetSQLiteSequence(ctx context.Context, q Querier, table string) error {
	rows, err := q.conn().query(ctx, "query", "", "", Compiled{SQL: "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_sequence'"})
	if err != nil {
		return err
	}
	has := rows.Next()
	err = rows.Err()
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	if err != nil || !has {
		return err
	}
	c, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("DELETE FROM sqlite_sequence WHERE name = ")
		b.Arg(table)
	})
	if err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "truncate", table, c)
	return err
}
