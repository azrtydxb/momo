// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// The bodies of SelectQuery[T] that do not depend on T live here, on
// non-generic receivers. Go compiles a generic method once per GC shape, and
// every distinct struct T has its own shape, so a method body on SelectQuery[T]
// is compiled again for every model and every Row type a package selects into.
// The generic methods are thin wrappers: they hand the clauses to these
// functions and keep only what needs T (scanning into T, hooks, preloads).

// tableInfo is the part of Meta[T] that the SELECT renderer reads. It does not
// mention T, so the renderer is compiled once.
type tableInfo struct {
	Table          string
	Columns        []ColumnMeta
	PK             []int
	SoftDelete     int
	SoftDeleteKind SoftDeleteKind
}

// infoOf returns the T-independent description of m; the zero tableInfo for a
// nil Meta.
func infoOf[T any](m *Meta[T]) tableInfo {
	if m == nil {
		return tableInfo{}
	}
	return tableInfo{Table: m.Table, Columns: m.Columns, PK: m.PK, SoftDelete: m.SoftDelete, SoftDeleteKind: m.SoftDeleteKind}
}

// tableOf is the table name of m, empty for a nil Meta.
func tableOf[T any](m *Meta[T]) string {
	if m == nil {
		return ""
	}
	return m.Table
}

// softDeletePred is the live-row (or, with deleted, the deleted-row) filter on
// the soft-delete column of the table.
func (m tableInfo) softDeletePred(qualifier string, deleted bool) Predicate {
	return softMarkerPred{qualifier, m.Columns[m.SoftDelete].Name, m.SoftDeleteKind, deleted}
}

// selectRef is a select statement stripped of T: its clauses and the table
// description, which is all the SQL rendering needs.
type selectRef struct {
	s    *selectCore
	meta tableInfo
}

// ref returns the non-generic view of sq; it points at sq's own clauses.
func (sq *SelectQuery[T]) ref() selectRef {
	return selectRef{s: &sq.s, meta: infoOf(sq.meta)}
}

// ---------------------------------------------------------------- builders

// The clause builders below are marked go:noinline. They are small enough for
// the inliner, and inlining them copies their bodies into the wrapper of every
// instantiation of SelectQuery[T], which is what this file exists to avoid.

//go:noinline
func (s *selectCore) addWhere(ps []Predicate) { s.where = append(slices.Clip(s.where), ps...) }

//go:noinline
func (s *selectCore) setColumns(es []Expr) { s.cols = slices.Clone(es) }

//go:noinline
func (s *selectCore) addJoin(kind string, src TableSource, on []Predicate) {
	s.joins = append(slices.Clip(s.joins), join{kind, src, slices.Clone(on)})
}

//go:noinline
func (s *selectCore) addGroupBy(es []Expr) {
	for _, e := range es {
		rejectAliased(&s.err, "GROUP BY", e)
	}
	s.groupBy = append(slices.Clip(s.groupBy), es...)
}

//go:noinline
func (s *selectCore) addHaving(ps []Predicate) { s.having = append(slices.Clip(s.having), ps...) }

//go:noinline
func (s *selectCore) addOrder(items []Orderable) {
	o := slices.Clip(s.order)
	for _, it := range items {
		o = append(o, it.order())
	}
	s.order = o
}

//go:noinline
func (s *selectCore) after(table string, values []any) {
	if len(values) == 0 {
		s.err = newError(ErrValidation, "build", table, errors.New("After needs at least one value"))
		return
	}
	s.keyset = append(slices.Clip(s.keyset), values...)
}

//go:noinline
func (s *selectCore) setLimit(table string, n int64) {
	if n < 0 {
		s.err = newError(ErrValidation, "build", table, fmt.Errorf("Limit must not be negative, got %d", n))
		return
	}
	s.limit = n
}

//go:noinline
func (s *selectCore) setOffset(table string, n int64) {
	if n < 0 {
		s.err = newError(ErrValidation, "build", table, fmt.Errorf("Offset must not be negative, got %d", n))
		return
	}
	s.offset = n
}

//go:noinline
func (s *selectCore) setLock(table, l string, opt []string) {
	s.lockWait = ""
	for _, o := range opt {
		switch o {
		case "NOWAIT", "SKIP LOCKED":
			if s.lockWait != "" && s.lockWait != " "+o {
				s.err = newError(ErrValidation, "build", table, errors.New("lock options NOWAIT and SKIP LOCKED are mutually exclusive"))
			}
			s.lockWait = " " + o
		default:
			s.err = newError(ErrValidation, "build", table, fmt.Errorf("unknown lock option %q (want NOWAIT or SKIP LOCKED)", o))
		}
	}
	s.lock = l
}

//go:noinline
func (s *selectCore) setLockOf(table string, tables []string) {
	if len(tables) == 0 {
		s.err = newError(ErrValidation, "build", table, errors.New("LockOf needs at least one table"))
		return
	}
	if slices.Contains(tables, "") {
		s.err = newError(ErrValidation, "build", table, errors.New("LockOf table name must not be empty"))
		return
	}
	s.lockOf = slices.Clone(tables)
}

//go:noinline
func (s *selectCore) setIndexedBy(table, index string) {
	if index == "" {
		s.err = newError(ErrValidation, "build", table, errors.New("IndexedBy needs an index name"))
		return
	}
	s.indexedBy = index
}

//go:noinline
func (s *selectCore) setTableName(table, name string) {
	t, ok := s.from.(TableRef)
	switch {
	case name == "":
		s.err = newError(ErrValidation, "build", table, errors.New("WithTableName needs a table name"))
	case !ok:
		s.err = newError(ErrValidation, "build", table, errors.New("WithTableName needs a plain table as the FROM source"))
	default:
		if t.Alias == "" {
			t.Alias = t.Name
		}
		t.Name = name
		s.from = t
	}
}

//go:noinline
func (s *selectCore) addWith(name string, q Subquery, cols []string, recursive bool) {
	s.with = append(slices.Clip(s.with), cte{name: name, cols: slices.Clone(cols), q: q, recursive: recursive})
}

//go:noinline
func (s *selectCore) setMaxRows(table string, n int) {
	if n <= 0 {
		s.err = &Error{Kind: ErrValidation, Op: "build", Table: table, Err: fmt.Errorf("MaxRows must be positive, got %d", n)}
		return
	}
	s.maxRows = n
}

//go:noinline
func (s *selectCore) setDistinctOn(table string, es []Expr) {
	if len(es) == 0 {
		s.err = &Error{Kind: ErrValidation, Op: "build", Table: table, Err: errors.New("DistinctOn needs at least one expression")}
		return
	}
	for _, e := range es {
		rejectAliased(&s.err, "DISTINCT ON", e)
	}
	s.distinctOn = slices.Clone(es)
}

//go:noinline
func (s *selectCore) addPreloads(plain, joined []any) {
	s.preloads = append(slices.Clip(s.preloads), plain...)
	s.joinLoads = append(slices.Clip(s.joinLoads), joined...)
}

// qualifier is the name the model's own columns are qualified with.
// A schema-qualified, unaliased TableRef source renders bare column names;
// every other source qualifies with its alias or table name.
func (s *selectCore) qualifier(table string) string {
	if t, ok := s.from.(interface{ Qualifier() string }); ok {
		return t.Qualifier()
	}
	return table
}

// intercept adds the predicates of the handle's interceptors to the WHERE.
func (s *selectCore) intercept(ctx context.Context, q Querier, table string) error {
	extra, err := runInterceptors(ctx, q, "select", table)
	if err != nil {
		return err
	}
	if len(extra) > 0 {
		s.where = append(slices.Clone(s.where), extra...)
	}
	return nil
}

// errStreamPreload is the error Each and Iter return when preloads are set.
//
//go:noinline
func (s *selectCore) errStreamPreload(table string) error {
	if len(s.preloads) == 0 && len(s.joinLoads) == 0 {
		return nil
	}
	return &Error{Kind: ErrUnsupported, Op: "select", Table: table,
		Err: errors.New("Preload is not supported with Each or Iter; use All, One or First")}
}

//go:noinline
func maxRowsErr(table string, n int) error {
	return &Error{Kind: ErrLimitExceeded, Op: "select", Table: table, Err: fmt.Errorf("result has more than %d rows (MaxRows)", n)}
}

// -------------------------------------------------------------- execution

// compileSelect compiles the statement with its joined relations expanded and
// returns the loaders of the joined blocks, in block order. The interceptors of
// the handle run first, with ctx. s is the caller's copy: it is changed here.
func compileSelect(ctx context.Context, q Querier, m tableInfo, s selectCore) (Compiled, []joinLoaderCore, error) {
	if q == nil {
		return Compiled{}, nil, noQuerier("select", m.Table)
	}
	if err := s.intercept(ctx, q, m.Table); err != nil {
		return Compiled{}, nil, err
	}
	ls, err := expandJoins(&s, m)
	if err != nil {
		return Compiled{}, nil, err
	}
	run := selectRef{s: &s, meta: m}
	c, err := compile(q.Dialect(), run.render)
	return c, ls, err
}

// selectRows compiles the statement and runs it.
func selectRows(ctx context.Context, q Querier, m tableInfo, s selectCore) (Rows, []joinLoaderCore, error) {
	c, ls, err := compileSelect(ctx, q, m, s)
	if err != nil {
		return nil, nil, err
	}
	r, err := q.conn().query(ctx, "select", m.Table, "", c)
	return r, ls, err
}

// countSelect runs SELECT COUNT(*) over the statement (as a subquery, so
// DISTINCT, GROUP BY, LIMIT and OFFSET keep their meaning).
func countSelect(ctx context.Context, q Querier, m tableInfo, s selectCore) (int64, error) {
	if q == nil {
		return 0, noQuerier("select", m.Table)
	}
	if err := s.intercept(ctx, q, m.Table); err != nil {
		return 0, err
	}
	// The count covers the whole filtered set: neither the ordering nor the
	// page cursor applies.
	if len(s.distinctOn) == 0 { // DISTINCT ON keeps its ORDER BY: it picks the rows
		s.order = nil
	}
	s.distinctCount = len(s.distinctOn) > 0
	s.keyset = nil
	inner := selectRef{s: &s, meta: m}
	c, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("SELECT COUNT(*) FROM ")
		inner.AppendSQL(b)
		b.WriteString(" AS momo_count")
	})
	if err != nil {
		return 0, err
	}
	r, err := q.conn().query(ctx, "select", m.Table, "", c)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	var n int64
	if r.Next() {
		if err := r.Scan(&n); err != nil {
			return 0, err
		}
	}
	if err := r.Err(); err != nil {
		return 0, err
	}
	return n, r.Close()
}

// explainSelect runs EXPLAIN for the statement and returns the plan text.
func explainSelect(ctx context.Context, q Querier, m tableInfo, s selectCore) (string, error) {
	name := q.Dialect().Name
	var prefix string
	switch name {
	case dialect.Postgres:
		prefix = "EXPLAIN "
	case dialect.SQLite:
		prefix = "EXPLAIN QUERY PLAN "
	default:
		return "", fmt.Errorf("momo: Explain on %s: %w", name, ErrUnsupported)
	}
	c, _, err := compileSelect(ctx, q, m, s)
	if err != nil {
		return "", err
	}
	c.SQL = prefix + c.SQL
	r, err := q.conn().query(ctx, "explain", m.Table, "", c)
	if err != nil {
		return "", err
	}
	defer r.Close()
	var lines []string
	for r.Next() {
		var line string
		if name == dialect.SQLite {
			// EXPLAIN QUERY PLAN returns (id, parent, notused, detail).
			var id, parent, unused any
			err = r.Scan(&id, &parent, &unused, &line)
		} else {
			err = r.Scan(&line)
		}
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	if err := r.Err(); err != nil {
		return "", err
	}
	if err := r.Close(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

// finishRows reports the error that ended the iteration, then closes r.
//
//go:noinline
func finishRows(r Rows) error {
	if err := r.Err(); err != nil {
		return err
	}
	return r.Close()
}

// nextRow advances r. It reports false with the error that ended the
// iteration, if any, once the rows are exhausted, and fails with ErrCanceled
// when ctx is done.
//
//go:noinline
func nextRow(ctx context.Context, r Rows, table string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, newError(ErrCanceled, "select", table, err)
	}
	if r.Next() {
		return true, nil
	}
	return false, finishRows(r)
}

// firstRow advances r to its first row. It fails with ErrNotFound when there is
// none.
//
//go:noinline
func firstRow(r Rows, table string) error {
	if r.Next() {
		return nil
	}
	if err := r.Err(); err != nil {
		return err
	}
	return &Error{Kind: ErrNotFound, Op: "select", Table: table}
}

// lastRow checks that r holds no row after the first, then ends it. It fails
// with ErrTooManyRows when there is another.
//
//go:noinline
func lastRow(r Rows, table string) error {
	if r.Next() {
		return &Error{Kind: ErrTooManyRows, Op: "select", Table: table}
	}
	return finishRows(r)
}

// expandJoins rewrites s, the caller's own copy, when joined loaders are
// present: the parent columns, then one aliased block per joined loader, plus
// the LEFT JOINs. It returns the loaders, in block order, and leaves s alone
// when there are none or on error. It is applied at execution and in SQL, not
// at Preload, so clauses added after Preload (Columns, joins, Where) compose.
func expandJoins(s *selectCore, m tableInfo) ([]joinLoaderCore, error) {
	if len(s.joinLoads) == 0 {
		return nil, nil
	}
	ls := make([]joinLoaderCore, len(s.joinLoads))
	for i, l := range s.joinLoads {
		ls[i] = l.(joinLoaderCore)
		if err := ls[i].joinCheck(); err != nil {
			return nil, err
		}
	}
	unsupported := func(what string) error {
		return newError(ErrUnsupported, "select", m.Table, errors.New("Joined relations cannot be combined with "+what))
	}
	switch {
	case len(s.groupBy) > 0 || len(s.having) > 0:
		return nil, unsupported("GroupBy or Having")
	case s.lock != "":
		return nil, unsupported("row locking")
	}
	parent := s.qualifier(m.Table)
	cols := slices.Clone(s.cols)
	switch {
	case cols == nil:
		for _, c := range m.Columns {
			cols = append(cols, qcol{parent, c.Name})
		}
	case len(cols) != len(m.Columns):
		return nil, &Error{Kind: ErrValidation, Op: "build", Table: m.Table,
			Err: errors.New("select list must have one expression per mapped field to preload a joined relation")}
	}
	joins := slices.Clip(s.joins)
	for i, l := range ls {
		cols, joins = l.joinExpand(parent, "momo_j"+strconv.Itoa(i), cols, joins)
	}
	s.cols, s.joins, s.joinLoads = cols, joins, nil
	s.keysOnly = true // the select list is wider than the model's column list
	return ls, nil
}
