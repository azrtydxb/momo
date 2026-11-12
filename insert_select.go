// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

// ExprSelect is a SELECT of explicit expressions that needs no result type:
// a FROM-less list of values or parameters with an optional WHERE, or a
// plain SELECT over one source. It is a Subquery, so it is the row source of
// InsertQuery.FromSelect and also works as an EXISTS operand:
//
//	momo.SelectExprs(momo.Value(1)).From(meters).Where(meter.Legacy.Eq(true))
//
// ExprSelect is an immutable value: every method returns a new one.
type ExprSelect struct {
	cols  []Expr
	from  TableSource
	where []Predicate

	joins   []join
	groupBy []Expr
	having  []Predicate
	order   []Order
	err     error
}

// SelectExprs starts a SELECT of the given expressions with no FROM clause.
// Add a source with From and conditions with Where. Pass every literal as a
// momo.Value so it is bound as a parameter and nothing is concatenated into
// the SQL text.
func SelectExprs(cols ...Expr) ExprSelect {
	return ExprSelect{cols: slices.Clone(cols)}
}

// From sets the FROM source. Without it the SELECT has no FROM clause, which
// PostgreSQL and SQLite accept and MySQL accepts only as FROM DUAL (added
// automatically when a WHERE is present). The soft-delete filter is added
// only when src is a TableRef whose SoftDelete marker is set: generated Table
// refs carry it, a hand-built TableRef{Name: ...} does not and then counts
// soft-deleted rows, so set SoftDelete and SoftDeleteKind on it.
func (s ExprSelect) From(src TableSource) ExprSelect { s.from = src; return s }

// Where adds predicates combined with AND. On a FROM-less select the WHERE
// guards the single row, so the select yields zero rows when it is false.
func (s ExprSelect) Where(ps ...Predicate) ExprSelect {
	s.where = append(slices.Clip(s.where), ps...)
	return s
}

// Join adds an INNER JOIN, like SelectQuery.Join.
func (s ExprSelect) Join(src TableSource, on ...Predicate) ExprSelect {
	return s.addJoin("INNER JOIN", src, on)
}

// LeftJoin adds a LEFT JOIN, like SelectQuery.LeftJoin.
func (s ExprSelect) LeftJoin(src TableSource, on ...Predicate) ExprSelect {
	return s.addJoin("LEFT JOIN", src, on)
}

// CrossJoin adds a CROSS JOIN, like SelectQuery.CrossJoin.
func (s ExprSelect) CrossJoin(src TableSource) ExprSelect { return s.addJoin("CROSS JOIN", src, nil) }

func (s ExprSelect) addJoin(kind string, src TableSource, on []Predicate) ExprSelect {
	s.joins = append(slices.Clip(s.joins), join{kind, src, slices.Clone(on)})
	return s
}

// GroupBy adds GROUP BY expressions. An As(...) alias is rejected with
// ErrValidation, as in SelectQuery.GroupBy. This is what lets a grouped
// INSERT ... SELECT source have more columns than a typed Select can map.
func (s ExprSelect) GroupBy(es ...Expr) ExprSelect {
	for _, e := range es {
		rejectAliased(&s.err, "GROUP BY", e)
	}
	s.groupBy = append(slices.Clip(s.groupBy), es...)
	return s
}

// Having adds HAVING predicates combined with AND.
func (s ExprSelect) Having(ps ...Predicate) ExprSelect {
	s.having = append(slices.Clip(s.having), ps...)
	return s
}

// OrderBy appends ORDER BY items: an Order such as Column.Asc() or any
// Expression, which orders ascending.
func (s ExprSelect) OrderBy(items ...Orderable) ExprSelect {
	o := slices.Clip(s.order)
	for _, it := range items {
		o = append(o, it.order())
	}
	s.order = o
	return s
}

func (ExprSelect) isSubquery() {}

// selectArity reports the number of columns the select yields.
func (s ExprSelect) selectArity() (int, bool) { return len(s.cols), true }

// AppendSQL renders the select in parentheses, for use as a subquery.
func (s ExprSelect) AppendSQL(b *Builder) {
	b.writeByte('(')
	s.render(b)
	b.writeByte(')')
}

// render writes the bare SELECT; INSERT ... SELECT and set operations use it.
func (s ExprSelect) render(b *Builder) { s.renderBare(b, false) }

// renderBare is render; onConflictFollows makes a select with a FROM source
// and no WHERE end in WHERE true so SQLite does not read ON CONFLICT as a join.
func (s ExprSelect) renderBare(b *Builder, onConflictFollows bool) {
	if s.err != nil {
		b.SetErr(s.err)
		return
	}
	var names []string
	for _, src := range append([]TableSource{s.from}, joinSources(s.joins)...) {
		if q, ok := src.(interface{ Qualifier() string }); ok {
			names = append(names, q.Qualifier())
		}
	}
	b.withScope(names, func() {
		if len(s.cols) == 0 {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("SelectExprs: no expressions")})
			return
		}
		if s.from == nil && (len(s.joins) > 0 || len(s.groupBy) > 0 || len(s.having) > 0) {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("SelectExprs: Join, GroupBy and Having need a From source")})
			return
		}
		b.WriteString("SELECT ")
		appendList(b, s.cols)
		// FROM, the soft-delete marker and WHERE are rendered by the same
		// selectCore methods as a SelectQuery, so they exist once.
		core := selectCore{from: s.from, joins: s.joins, where: s.where, groupBy: s.groupBy, having: s.having}
		where := core.where
		if core.from != nil {
			core.writeFrom(b, "")
			if p, ok := core.sourceSoftDelete(); ok {
				where = append(slices.Clip(where), p)
			}
		} else if len(where) > 0 && b.d.Name == dialect.MySQL {
			b.WriteString(" FROM DUAL")
		}
		core.writeJoins(b)
		core.writeWhere(b, where, onConflictFollows)
		core.writeGroupHaving(b)
		for i, o := range s.order {
			if i == 0 {
				b.WriteString(" ORDER BY ")
			} else {
				b.WriteString(", ")
			}
			o.appendSQL(b)
		}
	})
}

// FromSelect makes the rows of a SELECT the source of the INSERT, rendering
// INSERT INTO t (cols) SELECT ... in one atomic statement. cols are the typed
// target columns of T, in the order of the select list; the select must
// yield exactly that many columns, otherwise the build fails with
// ErrValidation (a RawSubquery or set operation has no known width, so the
// database checks it).
//
// The select is the row source, for example a FROM-less select guarded by
// NOT EXISTS so the row is inserted only when none exists yet:
//
//	momo.Insert[Meter](db).
//		FromSelect(
//			momo.SelectExprs(momo.Value(p), momo.Value(id), momo.Value("distinct")).
//				Where(momo.NotExists(momo.SelectExprs(momo.Value(1)).From(meterTable).Where(meter.Product.Eq(p)))),
//			meter.Product, meter.ID, meter.Kind).
//		OnConflict(meter.Product, meter.ID).DoNothing().
//		Exec(ctx)
//
// FromSelect composes with OnConflict (DoNothing, DoUpdate and their Where),
// with Returning, ReturningAll and Rows, and with Upsert[T], whose default
// DO UPDATE list then covers the listed columns only. The bind parameters of
// the select come first, then those of the ON CONFLICT part; each dialect
// numbers them in order. Exec reports the rows inserted (0 when the select
// yields no row or every row conflicts).
//
// Only the listed columns are written. Nothing is injected: the automatic
// created-at and updated-at timestamps, Go-side defaults and field
// validators do not apply, so list those columns and select their values
// yourself. A column that is an identity, read-only, or otherwise not
// writable by INSERT is refused with ErrValidation, as are an unmapped or
// repeated column, an empty list, and combining FromSelect with Model,
// Models, Values, ValuesRows or Omit. Calling FromSelect again replaces the
// source. FromSelect runs no model hooks.
func (q InsertQuery[T]) FromSelect(sel Subquery, cols ...ColumnRef) InsertQuery[T] {
	if sel == nil {
		return q.fail(q.verr("", "FromSelect: nil select"))
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.ColumnName()
	}
	q.i.sel, q.i.selCols = sel, names
	return q
}

// renderFromSelect renders INSERT INTO t (cols) SELECT ... plus the conflict
// and RETURNING clauses.
func (q InsertQuery[T]) renderFromSelect(b *Builder) {
	switch {
	case len(q.models) > 0 || q.i.hasValues || q.i.hasRows:
		b.SetErr(q.verr("", "FromSelect cannot be combined with Model, Models, Values or ValuesRows"))
		return
	case len(q.i.omit) > 0:
		b.SetErr(q.verr("", "Omit cannot be combined with FromSelect: list only the columns to write"))
		return
	case len(q.i.selCols) == 0:
		b.SetErr(q.verr("", "FromSelect: no target columns"))
		return
	}
	for i, n := range q.i.selCols {
		ci := q.column(n)
		switch {
		case ci < 0:
			b.SetErr(q.verr(n, "FromSelect: column %q is not mapped by %s", n, q.meta.Table))
			return
		case !q.writable(ci):
			b.SetErr(q.verr(n, "FromSelect: column %q is identity, serial or read-only and is never written", n))
			return
		case slices.Contains(q.i.selCols[:i], n):
			b.SetErr(q.verr(n, "FromSelect: column %q listed twice", n))
			return
		}
	}
	if n, ok := arityOf(q.i.sel); ok && n != len(q.i.selCols) {
		b.SetErr(q.verr("", "FromSelect: select yields %d columns but %d target columns are listed", n, len(q.i.selCols)))
		return
	}
	fp, err := newFieldPlan(q.q, q.meta)
	if err != nil {
		b.SetErr(err)
		return
	}
	if err := q.validateConflictSet(fp); err != nil {
		b.SetErr(err)
		return
	}
	q.writeInsertHead(b)
	b.WriteString(" (")
	for i, n := range q.i.selCols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(n)
	}
	b.WriteString(") ")
	if e, ok := q.i.sel.(interface{ renderBare(*Builder, bool) }); ok {
		// SQLite parses ON after a FROM select without WHERE as a join.
		e.renderBare(b, b.d.Name == dialect.SQLite && q.i.conflict != conflictNone)
	} else if r, ok := q.i.sel.(interface{ render(*Builder) }); ok {
		r.render(b) // bare: SQLite rejects a parenthesized SELECT here
	} else {
		q.i.sel.AppendSQL(b)
	}
	q.renderConflict(b)
	renderReturning(b, q.meta, q.i.returning, func(col string) error {
		return q.verr(col, "Returning: column %q is not mapped by %s", col, q.meta.Table)
	})
}
