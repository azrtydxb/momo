// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

// Subquery is a query usable as an expression: IN (...), EXISTS (...),
// scalar subqueries and FROM sources. *SelectQuery satisfies it.
type Subquery interface {
	Expr
	isSubquery()
}

type join struct {
	kind string
	src  TableSource
	on   []Predicate
}

type cte struct {
	name      string
	cols      []string
	q         Subquery
	recursive bool
}

// selectCore holds a SELECT statement's clauses. All slices are treated as
// immutable once stored: every builder method appends to a clipped copy, so
// derived queries never share writable backing arrays.
type selectCore struct {
	with     []cte
	distinct bool
	cols     []Expr // nil: the model's mapped columns
	from     TableSource
	joins    []join
	where    []Predicate
	groupBy  []Expr
	having   []Predicate
	order    []Order
	keyset   []any
	// joinLoads holds the joined loaders added by Preload (copy-on-write).
	joinLoads   []any
	limit       int64 // -1: none
	offset      int64
	lock        string
	indexedBy   string
	withDeleted bool
	onlyDeleted bool
	// keysOnly marks an internal key-set subquery (relation traversal) whose
	// select list is not the model's column list, so the arity check is skipped.
	keysOnly bool
	// preloads holds the Loader[T] values added by Preload (copy-on-write).
	preloads []any
	err      error
}

// SelectQuery builds and runs a SELECT whose rows scan into T.
//
// SelectQuery is an immutable value: every method returns a new query and
// leaves the receiver unchanged, so a base query can be extended
// concurrently from many goroutines. Executing a query does not modify it.
type SelectQuery[T any] struct {
	q    Querier
	meta *Meta[T]
	s    selectCore
}

// Select starts a query for model or projection T:
//
//	momo.Select[models.Document](db).Where(document.Published.Eq(true)).All(ctx)
//
// For a model, FROM and the select list default to the model's table and
// mapped columns. For a projection, call From and Columns.
func Select[T any, PT Modeler[T]](q Querier) SelectQuery[T] {
	m := metaOf[T, PT]()
	s := selectCore{limit: -1}
	if m.Table != "" {
		s.from = TableRef{Name: m.Table}
	}
	return SelectQuery[T]{q: q, meta: m, s: s}
}

func (sq SelectQuery[T]) isSubquery() {}

// AppendSQL renders the query in parentheses, for use as a subquery.
func (sq SelectQuery[T]) AppendSQL(b *Builder) {
	b.writeByte('(')
	sq.render(b)
	b.writeByte(')')
}

// Where adds predicates combined with AND.
func (sq SelectQuery[T]) Where(ps ...Predicate) SelectQuery[T] {
	sq.s.where = append(slices.Clip(sq.s.where), ps...)
	return sq
}

// Columns replaces the select list. For model results the expressions must
// match the model's mapped columns one-to-one; for projections they map to
// the projection's fields in order.
func (sq SelectQuery[T]) Columns(es ...Expr) SelectQuery[T] {
	sq.s.cols = slices.Clone(es)
	return sq
}

// Distinct adds DISTINCT.
func (sq SelectQuery[T]) Distinct() SelectQuery[T] { sq.s.distinct = true; return sq }

// From replaces the FROM source (a table, aliased table, CTE or subquery).
func (sq SelectQuery[T]) From(src TableSource) SelectQuery[T] { sq.s.from = src; return sq }

func (sq SelectQuery[T]) addJoin(kind string, src TableSource, on []Predicate) SelectQuery[T] {
	sq.s.joins = append(slices.Clip(sq.s.joins), join{kind, src, slices.Clone(on)})
	return sq
}

// Join adds an INNER JOIN.
func (sq SelectQuery[T]) Join(src TableSource, on ...Predicate) SelectQuery[T] {
	return sq.addJoin("INNER JOIN", src, on)
}

// LeftJoin adds a LEFT JOIN. Columns of src may be NULL in the result:
// project them into nullable fields (pointer or sql.Null types). Scanning a
// NULL into a non-nullable field fails with a driver error; it never
// silently yields a zero value.
func (sq SelectQuery[T]) LeftJoin(src TableSource, on ...Predicate) SelectQuery[T] {
	return sq.addJoin("LEFT JOIN", src, on)
}

// CrossJoin adds a CROSS JOIN.
func (sq SelectQuery[T]) CrossJoin(src TableSource) SelectQuery[T] {
	return sq.addJoin("CROSS JOIN", src, nil)
}

// GroupBy sets GROUP BY expressions.
func (sq SelectQuery[T]) GroupBy(es ...Expr) SelectQuery[T] {
	for _, e := range es {
		rejectAliased(&sq.s.err, "GROUP BY", e)
	}
	sq.s.groupBy = append(slices.Clip(sq.s.groupBy), es...)
	return sq
}

// Having adds HAVING predicates combined with AND.
func (sq SelectQuery[T]) Having(ps ...Predicate) SelectQuery[T] {
	sq.s.having = append(slices.Clip(sq.s.having), ps...)
	return sq
}

// OrderBy appends ORDER BY items. Pass Column.Asc()/Desc(), or a typed
// expression (e.g. a vector distance), which orders ascending.
func (sq SelectQuery[T]) OrderBy(items ...Orderable) SelectQuery[T] {
	o := slices.Clip(sq.s.order)
	for _, it := range items {
		o = append(o, it.order())
	}
	sq.s.order = o
	return sq
}

// Orderable is accepted by OrderBy: an Order or any Expression.
type Orderable interface{ order() Order }

func (o Order) order() Order           { return o }
func (c Column[T]) order() Order       { return Order{Expr: c} }
func (t typed[T]) order() Order        { return Order{Expr: t} }
func (t typedFn[T]) order() Order      { return Order{Expr: t} }
func (c NullColumn[T]) order() Order   { return Order{Expr: c} }
func (sq SelectQuery[T]) order() Order { return Order{Expr: sq} }

// After restricts the result to rows strictly after the cursor given by
// values, one per effective ORDER BY key in order. The effective keys are the
// OrderBy items followed by the primary key ascending when it is not already
// an item, which makes the order total. Ascending keys compare with >,
// descending keys with <. After needs OrderBy, a value count equal to the
// number of effective keys (ErrValidation otherwise), and non-nullable keys
// (ErrUnsupported otherwise).
func (sq SelectQuery[T]) After(values ...any) SelectQuery[T] {
	if len(values) == 0 {
		sq.s.err = &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: errors.New("After needs at least one value")}
		return sq
	}
	sq.s.keyset = append(slices.Clip(sq.s.keyset), values...)
	return sq
}

// keysetKey is one effective ORDER BY key of a keyset predicate.
type keysetKey struct {
	expr     Expr
	desc     bool
	nullable bool
}

func (sq SelectQuery[T]) keysetKeys() []keysetKey {
	s := &sq.s
	keys := make([]keysetKey, 0, len(s.order)+len(sq.meta.PK))
	for _, o := range s.order {
		k := keysetKey{expr: o.Expr, desc: o.Desc}
		if f, ok := o.Expr.(interface{ Flags() uint16 }); ok {
			k.nullable = f.Flags()&FlagNullable != 0
		}
		keys = append(keys, k)
	}
	q := sq.qualifier()
	for _, i := range sq.meta.PK {
		c := sq.meta.Columns[i]
		present := false
		for _, o := range s.order {
			r, ok := o.Expr.(ColumnRef)
			if !ok || r.ColumnName() != c.Name {
				continue
			}
			if t, ok := o.Expr.(interface{ Table() string }); ok && t.Table() != q {
				continue // the same column name on another table source
			}
			present = true
			break
		}
		if !present {
			keys = append(keys, keysetKey{expr: qcol{q, c.Name}, nullable: c.Flags&FlagNullable != 0})
		}
	}
	return keys
}

// keysetPredicate builds the lexicographic expansion of the cursor, or nil
// after recording an error.
func (sq SelectQuery[T]) keysetPredicate(b *Builder) Predicate {
	s := &sq.s
	if len(s.order) == 0 {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: errors.New("After requires OrderBy")})
		return nil
	}
	keys := sq.keysetKeys()
	if len(s.keyset) != len(keys) {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: fmt.Errorf("After got %d values for %d ORDER BY keys (including the primary key tie-breaker)", len(s.keyset), len(keys))})
		return nil
	}
	for _, k := range keys {
		if k.nullable {
			b.SetErr(&Error{Kind: ErrUnsupported, Op: "build", Table: sq.meta.Table, Err: errors.New("After with a nullable ORDER BY key is not supported")})
			return nil
		}
	}
	disjuncts := make([]Predicate, len(keys))
	for i := range keys {
		parts := make([]Predicate, 0, i+1)
		for j := 0; j < i; j++ {
			parts = append(parts, binOp{keys[j].expr, " = ", argExpr{s.keyset[j]}})
		}
		op := " > "
		if keys[i].desc {
			op = " < "
		}
		parts = append(parts, binOp{keys[i].expr, op, argExpr{s.keyset[i]}})
		if len(parts) == 1 {
			disjuncts[i] = parts[0]
		} else {
			disjuncts[i] = junction{" AND ", parts}
		}
	}
	return junction{" OR ", disjuncts}
}

// Limit sets LIMIT.
func (sq SelectQuery[T]) Limit(n int64) SelectQuery[T] {
	if n < 0 {
		sq.s.err = &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: fmt.Errorf("Limit must not be negative, got %d", n)}
		return sq
	}
	sq.s.limit = n
	return sq
}

// Offset sets OFFSET. Prefer keyset pagination (After) for deep pages.
func (sq SelectQuery[T]) Offset(n int64) SelectQuery[T] {
	if n < 0 {
		sq.s.err = &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: fmt.Errorf("Offset must not be negative, got %d", n)}
		return sq
	}
	sq.s.offset = n
	return sq
}

// ForUpdate adds FOR UPDATE (PostgreSQL). Options: "NOWAIT", "SKIP LOCKED".
func (sq SelectQuery[T]) ForUpdate(opt ...string) SelectQuery[T] {
	return sq.withLock("FOR UPDATE", opt)
}

// ForShare adds FOR SHARE (PostgreSQL).
func (sq SelectQuery[T]) ForShare(opt ...string) SelectQuery[T] { return sq.withLock("FOR SHARE", opt) }

func (sq SelectQuery[T]) withLock(l string, opt []string) SelectQuery[T] {
	for _, o := range opt {
		switch o {
		case "NOWAIT", "SKIP LOCKED":
			l += " " + o
		default:
			sq.s.err = &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: fmt.Errorf("unknown lock option %q (want NOWAIT or SKIP LOCKED)", o)}
		}
	}
	sq.s.lock = l
	return sq
}

// IndexedBy renders SQLite's INDEXED BY hint after the FROM table. The index
// name is quoted, never spliced. PostgreSQL and MySQL have no such clause
// and return ErrUnsupported when the query is built. The FROM source must be
// a plain table (TableRef).
func (sq SelectQuery[T]) IndexedBy(index string) SelectQuery[T] {
	if index == "" {
		sq.s.err = &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: errors.New("IndexedBy needs an index name")}
		return sq
	}
	sq.s.indexedBy = index
	return sq
}

// WithTableName replaces the FROM table name at runtime, for example to run
// one query shape over several identically shaped tables. The name is
// quoted; it is never spliced. The replacement is aliased to the model's
// table name, so the model's column expressions and the soft-delete filter
// keep resolving. It needs a plain table as the FROM source.
func (sq SelectQuery[T]) WithTableName(name string) SelectQuery[T] {
	t, ok := sq.s.from.(TableRef)
	switch {
	case name == "":
		sq.s.err = &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: errors.New("WithTableName needs a table name")}
	case !ok:
		sq.s.err = &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: errors.New("WithTableName needs a plain table as the FROM source")}
	default:
		if t.Alias == "" {
			t.Alias = t.Name
		}
		t.Name = name
		sq.s.from = t
	}
	return sq
}

// WithDeleted includes soft-deleted rows for models with a soft-delete
// column (they are excluded by default).
func (sq SelectQuery[T]) WithDeleted() SelectQuery[T] {
	sq.s.withDeleted, sq.s.onlyDeleted = true, false
	return sq
}

// With adds a common table expression. cols optionally names its columns.
func (sq SelectQuery[T]) With(name string, q Subquery, cols ...string) SelectQuery[T] {
	sq.s.with = append(slices.Clip(sq.s.with), cte{name: name, cols: slices.Clone(cols), q: q})
	return sq
}

// WithRecursive adds a recursive CTE; q is typically a UNION ALL built with
// SQL or Union.
func (sq SelectQuery[T]) WithRecursive(name string, q Subquery, cols ...string) SelectQuery[T] {
	sq.s.with = append(slices.Clip(sq.s.with), cte{name: name, cols: slices.Clone(cols), q: q, recursive: true})
	return sq
}

// SQL compiles the query without executing it.
func (sq SelectQuery[T]) SQL() (Compiled, error) {
	c, _, err := sq.compileJoined()
	return c, err
}

// compileJoined compiles sq with its joined relations expanded and returns
// the scanner for the resulting rows, expanding the joins once.
func (sq SelectQuery[T]) compileJoined() (Compiled, scanner[T], error) {
	sc := scanner[T]{meta: sq.meta}
	if sq.q == nil {
		return Compiled{}, sc, errors.New("momo: query has no Querier")
	}
	run, ls, err := sq.expandJoins()
	if err != nil {
		return Compiled{}, sc, err
	}
	if len(ls) > 0 {
		sc.join = newJoinScan(ls)
	}
	c, err := compile(sq.q.Dialect(), run.render)
	return c, sc, err
}

func (sq SelectQuery[T]) render(b *Builder) {
	s := &sq.s
	if s.err != nil {
		b.SetErr(s.err)
		return
	}
	names := make([]string, 0, 1+len(s.joins)+len(s.with))
	for _, c := range s.with {
		names = append(names, c.name)
	}
	for _, src := range append([]TableSource{s.from}, joinSources(s.joins)...) {
		if q, ok := src.(interface{ Qualifier() string }); ok {
			names = append(names, q.Qualifier())
		}
	}
	// FromSubquery and CTE bodies render inside this scope, so they can see
	// the sibling FROM names. That is more lenient than SQL, which allows it
	// only with LATERAL, but it is safe: it never hides a wrong table.
	b.withScope(names, func() { sq.renderBody(b) })
}

func (sq SelectQuery[T]) renderBody(b *Builder) {
	s := &sq.s
	if len(s.with) > 0 {
		b.WriteString("WITH ")
		for _, c := range s.with {
			if c.recursive {
				if !b.d.RecursiveCTE {
					b.unsupported("WITH RECURSIVE")
				}
				b.WriteString("RECURSIVE ")
				break
			}
		}
		for i, c := range s.with {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Ident(c.name)
			if len(c.cols) > 0 {
				b.WriteString(" (")
				for j, col := range c.cols {
					if j > 0 {
						b.WriteString(", ")
					}
					b.Ident(col)
				}
				b.writeByte(')')
			}
			b.WriteString(" AS ")
			c.q.AppendSQL(b)
		}
		b.writeByte(' ')
	}
	b.WriteString("SELECT ")
	if s.distinct {
		b.WriteString("DISTINCT ")
	}
	if s.cols == nil {
		if sq.meta.Table == "" {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("projection query needs Columns")})
			return
		}
		q := sq.qualifier()
		for i, c := range sq.meta.Columns {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(q, c.Name)
		}
	} else {
		if len(s.cols) != len(sq.meta.Columns) && !s.keysOnly {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: fmt.Errorf("select list has %d expressions but result type maps %d fields", len(s.cols), len(sq.meta.Columns))})
			return
		}
		for i, e := range s.cols {
			if i > 0 {
				b.WriteString(", ")
			}
			e.AppendSQL(b)
		}
	}
	if s.from != nil {
		b.WriteString(" FROM ")
		s.from.appendFrom(b)
		if s.indexedBy != "" {
			if _, ok := s.from.(TableRef); !ok {
				b.SetErr(&Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: errors.New("IndexedBy needs a plain table as the FROM source")})
				return
			}
			if b.d.Name != dialect.SQLite {
				b.unsupported("INDEXED BY")
				return
			}
			b.WriteString(" INDEXED BY ")
			b.Ident(s.indexedBy)
		}
	}
	for _, j := range s.joins {
		b.writeByte(' ')
		b.WriteString(j.kind)
		b.writeByte(' ')
		j.src.appendFrom(b)
		if len(j.on) > 0 {
			b.WriteString(" ON ")
			appendAnd(b, j.on)
		}
	}
	where := s.where
	if s.keyset != nil {
		if p := sq.keysetPredicate(b); p != nil {
			where = append([]Predicate{p}, where...)
		}
	}
	if sq.meta.SoftDelete >= 0 && sq.meta.Table != "" && !s.withDeleted {
		where = append(slices.Clip(where), softDeletePred(sq.meta, sq.qualifier(), s.onlyDeleted))
	}
	if sq.meta.Table == "" && !s.withDeleted {
		// A projection has no soft-delete column of its own: the filter
		// comes from the table it reads FROM. Joined tables are left alone,
		// as for a model.
		if t, ok := s.from.(TableRef); ok && t.SoftDelete != "" {
			where = append(slices.Clip(where), softMarkerPred{t.Qualifier(), t.SoftDelete, t.SoftDeleteKind, s.onlyDeleted})
		}
	}
	if len(where) > 0 {
		b.WriteString(" WHERE ")
		appendAnd(b, where)
	}
	if len(s.groupBy) > 0 {
		b.WriteString(" GROUP BY ")
		appendList(b, s.groupBy)
	}
	if len(s.having) > 0 {
		b.WriteString(" HAVING ")
		appendAnd(b, s.having)
	}
	if s.keyset != nil && len(s.order) > 0 {
		// A keyset query orders by the effective keys, directions explicit.
		b.WriteString(" ORDER BY ")
		for i, k := range sq.keysetKeys() {
			if i > 0 {
				b.WriteString(", ")
			}
			if i < len(s.order) {
				s.order[i].appendSQL(b)
			} else {
				Order{Expr: k.expr, Desc: k.desc}.appendSQL(b)
			}
		}
	} else if len(s.order) > 0 {
		b.WriteString(" ORDER BY ")
		for i, o := range s.order {
			if i > 0 {
				b.WriteString(", ")
			}
			o.appendSQL(b)
		}
	}
	if s.limit >= 0 {
		b.WriteString(" LIMIT ")
		b.Arg(s.limit)
	}
	if s.offset > 0 {
		if s.limit < 0 && b.d.Name == dialect.SQLite {
			b.WriteString(" LIMIT -1")
		}
		b.WriteString(" OFFSET ")
		b.Arg(s.offset)
	}
	if s.lock != "" {
		if !b.d.Locking {
			b.unsupported(s.lock)
			return
		}
		b.writeByte(' ')
		b.WriteString(s.lock)
	}
}

func joinSources(js []join) []TableSource {
	out := make([]TableSource, len(js))
	for i, j := range js {
		out[i] = j.src
	}
	return out
}

// qualifier is the name the model's own columns are qualified with.
// A schema-qualified, unaliased TableRef source renders bare column names;
// every other source qualifies with its alias or table name.
func (sq SelectQuery[T]) qualifier() string {
	if t, ok := sq.s.from.(interface{ Qualifier() string }); ok {
		return t.Qualifier()
	}
	return sq.meta.Table
}

type qcol struct{ table, name string }

func (c qcol) AppendSQL(b *Builder) { b.QualifiedIdent(c.table, c.name) }

func appendAnd(b *Builder, ps []Predicate) {
	for i, p := range ps {
		if i > 0 {
			b.WriteString(" AND ")
		}
		if _, isJ := p.(junction); isJ || len(ps) == 1 {
			p.AppendSQL(b)
			continue
		}
		b.writeByte('(')
		p.AppendSQL(b)
		b.writeByte(')')
	}
}

func appendList(b *Builder, es []Expr) {
	for i, e := range es {
		if i > 0 {
			b.WriteString(", ")
		}
		e.AppendSQL(b)
	}
}

// ------------------------------------------------------------ execution

// rows runs the query and returns its rows with the scanner for them.
func (sq SelectQuery[T]) rows(ctx context.Context) (Rows, scanner[T], error) {
	c, sc, err := sq.compileJoined()
	if err != nil {
		return nil, sc, err
	}
	r, err := sq.q.conn().query(ctx, "select", sq.meta.Table, "", c)
	return r, sc, err
}

// scanner reuses one destination slice across rows.
type scanner[T any] struct {
	meta *Meta[T]
	dest []any
	join *joinScan[T] // joined relations scanned after the model's columns
}

func (s *scanner[T]) scan(r Rows, m *T) error {
	n := len(s.meta.Columns)
	if s.dest == nil {
		extra := 0
		if s.join != nil {
			extra = s.join.width
		}
		s.dest = make([]any, n+extra)
	}
	for i := 0; i < n; i++ {
		s.dest[i] = s.meta.Ptr(m, i)
	}
	if s.join == nil {
		return r.Scan(s.dest...)
	}
	s.join.bind(s.dest[n:])
	if err := r.Scan(s.dest...); err != nil {
		return err
	}
	return s.join.assign(m)
}

// All runs the query and collects every row. An empty result is an empty
// (nil) slice and a nil error, never ErrNotFound.
func (sq SelectQuery[T]) All(ctx context.Context) ([]T, error) {
	r, sc, err := sq.rows(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var out []T
	if sq.s.limit > 0 && sq.s.limit <= 1024 {
		out = make([]T, 0, sq.s.limit)
	}
	var zero T
	for r.Next() {
		out = append(out, zero)
		if err := sc.scan(r, &out[len(out)-1]); err != nil {
			return nil, err
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	if err := r.Close(); err != nil {
		return nil, err
	}
	if err := sq.runPreloads(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// One returns the single matching row: ErrNotFound when none matches and
// ErrTooManyRows when more than one does. It adds no LIMIT.
func (sq SelectQuery[T]) One(ctx context.Context) (T, error) {
	var zero, m T
	r, sc, err := sq.rows(ctx)
	if err != nil {
		return zero, err
	}
	defer r.Close()
	if !r.Next() {
		if err := r.Err(); err != nil {
			return zero, err
		}
		return zero, &Error{Kind: ErrNotFound, Op: "select", Table: sq.meta.Table}
	}
	if err := sc.scan(r, &m); err != nil {
		return zero, err
	}
	if r.Next() {
		return zero, &Error{Kind: ErrTooManyRows, Op: "select", Table: sq.meta.Table}
	}
	if err := r.Err(); err != nil {
		return zero, err
	}
	if err := r.Close(); err != nil {
		return zero, err
	}
	if len(sq.s.preloads) > 0 {
		rows := []T{m}
		if err := sq.runPreloads(ctx, rows); err != nil {
			return zero, err
		}
		m = rows[0]
	}
	return m, nil
}

// First applies LIMIT 1 and returns the first row, or ErrNotFound.
func (sq SelectQuery[T]) First(ctx context.Context) (T, error) { return sq.Limit(1).One(ctx) }

// Each streams rows to fn without collecting them. The *T passed to fn is
// reused for the next row; copy it to retain it. Returning an error from fn
// stops iteration, closes the cursor and returns that error.
func (sq SelectQuery[T]) Each(ctx context.Context, fn func(*T) error) error {
	if err := sq.errStreamPreload(); err != nil {
		return err
	}
	r, sc, err := sq.rows(ctx)
	if err != nil {
		return err
	}
	defer r.Close()
	var m, zero T
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("momo: select cancelled: %w", err)
		}
		if !r.Next() {
			break
		}
		m = zero
		if err := sc.scan(r, &m); err != nil {
			return err
		}
		if err := fn(&m); err != nil {
			return err
		}
	}
	if err := r.Err(); err != nil {
		return err
	}
	return r.Close()
}

// Iter streams rows as a range-over-func sequence. Breaking out of the loop
// closes the cursor. An error is yielded once, as the final element.
func (sq SelectQuery[T]) Iter(ctx context.Context) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		if err := sq.errStreamPreload(); err != nil {
			yield(zero, err)
			return
		}
		r, sc, err := sq.rows(ctx)
		if err != nil {
			yield(zero, err)
			return
		}
		defer r.Close()
		for {
			if err := ctx.Err(); err != nil {
				yield(zero, fmt.Errorf("momo: select cancelled: %w", err))
				return
			}
			if !r.Next() {
				break
			}
			var m T
			if err := sc.scan(r, &m); err != nil {
				yield(zero, err)
				return
			}
			if !yield(m, nil) {
				return
			}
		}
		if err := r.Err(); err != nil {
			yield(zero, err)
			return
		}
		if err := r.Close(); err != nil {
			yield(zero, err)
		}
	}
}

// Count runs SELECT COUNT(*) over this query (as a subquery, so DISTINCT,
// GROUP BY, LIMIT and OFFSET keep their meaning).
func (sq SelectQuery[T]) Count(ctx context.Context) (int64, error) {
	if sq.q == nil {
		return 0, errors.New("momo: query has no Querier")
	}
	inner := sq
	// The count covers the whole filtered set: neither the ordering nor the
	// page cursor applies.
	inner.s.order = nil
	inner.s.keyset = nil
	c, err := compile(sq.q.Dialect(), func(b *Builder) {
		b.WriteString("SELECT COUNT(*) FROM ")
		inner.AppendSQL(b)
		b.WriteString(" AS momo_count")
	})
	if err != nil {
		return 0, err
	}
	r, err := sq.q.conn().query(ctx, "select", sq.meta.Table, "", c)
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

// Exists reports whether the query yields any row.
func (sq SelectQuery[T]) Exists(ctx context.Context) (bool, error) {
	n, err := sq.Limit(1).Count(ctx)
	return n > 0, err
}

// --------------------------------------------------------------- sources

type subquerySource struct {
	q     Subquery
	alias string
}

func (s subquerySource) appendFrom(b *Builder) {
	s.q.AppendSQL(b)
	b.WriteString(" AS ")
	b.Ident(s.alias)
}
func (s subquerySource) Qualifier() string { return s.alias }

// FromSubquery uses a subquery as a FROM/JOIN source under alias. Reference
// its columns with ColumnOf.
func FromSubquery(q Subquery, alias string) TableSource { return subquerySource{q, alias} }

// CTE references a common table expression (or any table not described by a
// model) by name. Reference its columns with ColumnOf.
func CTE(name string) TableRef { return TableRef{Name: name} }

// ColumnOf declares a typed column of an ad-hoc source (CTE, subquery
// alias). Momo cannot check that the column exists or has type T.
func ColumnOf[T any](source string, name string) Column[T] {
	return Column[T]{table: source, name: name}
}

type rawSubquery struct {
	sql  string
	args []any
}

func (rawSubquery) isSubquery() {}
func (r rawSubquery) AppendSQL(b *Builder) {
	b.writeByte('(')
	r.render(b)
	b.writeByte(')')
}

// render writes the fragment bare; a set operation renders members this way.
func (r rawSubquery) render(b *Builder) { b.appendRaw(r.sql, r.args) }

// RawSubquery is an explicit escape hatch: a raw SELECT usable as a Subquery
// (IN, EXISTS, CTE body, set operation member, FromSubquery source).
//
// Safety contract: as for SQL, sql is inserted verbatim and must be a
// trusted constant; '?' binds the next argument and "??" is a literal '?'.
// Momo cannot check which tables the fragment references. Only '?'
// placeholders bind; never write native '$1' style placeholders, which would
// collide with momo's own numbering. For a scalar subquery as a typed
// comparison operand use TypedSQL[T]("?", sub): the subquery renders its own
// parentheses.
func RawSubquery(sql string, args ...any) Subquery {
	return rawSubquery{sql, slices.Clone(args)}
}

// Union combines queries with UNION (all=false) or UNION ALL (all=true).
// The result is a Subquery, usable in WithRecursive, FromSubquery and IN.
func Union(all bool, qs ...Subquery) Subquery { return setOp{all, slices.Clone(qs)} }

type setOp struct {
	all bool
	qs  []Subquery
}

func (setOp) isSubquery() {}
func (s setOp) AppendSQL(b *Builder) {
	b.writeByte('(')
	for i, q := range s.qs {
		if i > 0 {
			if s.all {
				b.WriteString(" UNION ALL ")
			} else {
				b.WriteString(" UNION ")
			}
		}
		// Compound members are rendered without their own parentheses:
		// SQLite rejects parenthesized SELECTs in compound queries.
		if r, ok := q.(interface{ render(*Builder) }); ok {
			r.render(b)
		} else {
			q.AppendSQL(b)
		}
	}
	b.writeByte(')')
}
