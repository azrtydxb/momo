// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"slices"

	"github.com/azrtydxb/momo/dialect"
)

func (sq SelectQuery[T]) render(b *Builder) { sq.renderBare(b, false) }

// renderBare is render; onConflictFollows ends a select that has a FROM and
// no WHERE in WHERE true so SQLite does not read ON CONFLICT as a join.
func (sq SelectQuery[T]) renderBare(b *Builder, onConflictFollows bool) {
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
	b.withScope(names, func() { sq.renderBody(b, onConflictFollows) })
}

// renderBody writes the clauses in SQL order. A clause that fails records its
// error on the builder and stops the statement.
func (sq SelectQuery[T]) renderBody(b *Builder, onConflictFollows bool) {
	s := &sq.s
	s.writeWith(b)
	b.WriteString("SELECT ")
	if len(s.distinctOn) > 0 {
		sq.renderDistinctOn(b)
	} else if s.distinct {
		b.WriteString("DISTINCT ")
	}
	if !sq.writeSelectList(b) {
		return
	}
	if !s.writeFrom(b, sq.meta.Table) {
		return
	}
	s.writeJoins(b)
	s.writeWhere(b, sq.effectiveWhere(b), onConflictFollows)
	s.writeGroupHaving(b)
	sq.writeOrderBy(b)
	s.writeLimitOffset(b)
	sq.writeLock(b)
}

// writeWith writes the WITH clause, if any.
func (s *selectCore) writeWith(b *Builder) {
	if len(s.with) == 0 {
		return
	}
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

// writeSelectList writes the select list: the model's columns, or the explicit
// expressions. It reports false when the list is invalid.
func (sq SelectQuery[T]) writeSelectList(b *Builder) bool {
	s := &sq.s
	if s.cols == nil {
		if sq.meta.Table == "" {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("projection query needs Columns")})
			return false
		}
		q := sq.qualifier()
		for i, c := range sq.meta.Columns {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(q, c.Name)
		}
		return true
	}
	if len(s.cols) != len(sq.meta.Columns) && !s.keysOnly {
		b.SetErr(newError(ErrValidation, "build", sq.meta.Table, fmt.Errorf("select list has %d expressions but result type maps %d fields", len(s.cols), len(sq.meta.Columns))))
		return false
	}
	for i, e := range s.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		e.AppendSQL(b)
	}
	return true
}

// writeFrom writes FROM and the source, plus INDEXED BY when set. It does
// nothing without a source. table names the model in an error. It reports
// false when the clause is invalid.
func (s *selectCore) writeFrom(b *Builder, table string) bool {
	if s.from == nil {
		return true
	}
	b.WriteString(" FROM ")
	s.from.appendFrom(b)
	if s.indexedBy == "" {
		return true
	}
	if _, ok := s.from.(TableRef); !ok {
		b.SetErr(newError(ErrValidation, "build", table, errors.New("IndexedBy needs a plain table as the FROM source")))
		return false
	}
	if b.d.Name != dialect.SQLite {
		b.unsupported("INDEXED BY")
		return false
	}
	b.WriteString(" INDEXED BY ")
	b.Ident(s.indexedBy)
	return true
}

// writeJoins writes the JOIN clauses.
func (s *selectCore) writeJoins(b *Builder) {
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
}

// sourceSoftDelete returns the soft-delete filter of the FROM source when it is
// a TableRef whose SoftDelete marker is set and deleted rows are not asked
// for. A model query has its own soft-delete column instead; a projection or an
// ExprSelect takes the filter from the table it reads FROM. Joined tables are
// left alone.
func (s *selectCore) sourceSoftDelete() (Predicate, bool) {
	if s.withDeleted {
		return nil, false
	}
	if t, ok := s.from.(TableRef); ok && t.SoftDelete != "" {
		return softMarkerPred{t.Qualifier(), t.SoftDelete, t.SoftDeleteKind, s.onlyDeleted}, true
	}
	return nil, false
}

// effectiveWhere returns the predicates of the WHERE clause: the keyset
// predicate, the user's predicates and the soft-delete filter.
func (sq SelectQuery[T]) effectiveWhere(b *Builder) []Predicate {
	s := &sq.s
	where := s.where
	if s.keyset != nil {
		if p := sq.keysetPredicate(b); p != nil {
			where = append([]Predicate{p}, where...)
		}
	}
	if sq.meta.SoftDelete >= 0 && sq.meta.Table != "" && !s.withDeleted {
		where = append(slices.Clip(where), softDeletePred(sq.meta, sq.qualifier(), s.onlyDeleted))
	}
	if sq.meta.Table == "" {
		if p, ok := s.sourceSoftDelete(); ok {
			where = append(slices.Clip(where), p)
		}
	}
	return where
}

// writeWhere writes WHERE and the predicates ANDed. With no predicates it
// writes WHERE true when onConflictFollows and there is a FROM source.
func (s *selectCore) writeWhere(b *Builder, where []Predicate, onConflictFollows bool) {
	if len(where) > 0 {
		b.WriteString(" WHERE ")
		appendAnd(b, where)
	} else if s.from != nil && onConflictFollows {
		b.WriteString(" WHERE true")
	}
}

// writeGroupHaving writes GROUP BY and HAVING.
func (s *selectCore) writeGroupHaving(b *Builder) {
	if len(s.groupBy) > 0 {
		b.WriteString(" GROUP BY ")
		appendList(b, s.groupBy)
	}
	if len(s.having) > 0 {
		b.WriteString(" HAVING ")
		appendAnd(b, s.having)
	}
}

// writeOrderBy writes ORDER BY. A keyset query orders by the effective keys,
// directions explicit.
func (sq SelectQuery[T]) writeOrderBy(b *Builder) {
	s := &sq.s
	if len(s.order) == 0 {
		return
	}
	b.WriteString(" ORDER BY ")
	if s.keyset != nil {
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
		return
	}
	for i, o := range s.order {
		if i > 0 {
			b.WriteString(", ")
		}
		o.appendSQL(b)
	}
}

// writeLimitOffset writes LIMIT and OFFSET (SQLite needs LIMIT -1 before a
// bare OFFSET).
func (s *selectCore) writeLimitOffset(b *Builder) {
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
}

// writeLock writes the row-lock clause, if any: the lock mode, OF tables, then
// NOWAIT or SKIP LOCKED. A lock cannot be combined with DISTINCT, DISTINCT ON, GROUP BY or
// HAVING, and LockOf without a lock is an error.
func (sq SelectQuery[T]) writeLock(b *Builder) {
	s := &sq.s
	if s.lock == "" {
		if len(s.lockOf) > 0 {
			b.SetErr(newError(ErrValidation, "build", sq.meta.Table, errors.New("LockOf needs ForUpdate or ForShare")))
		}
		return
	}
	if !b.d.Locking {
		b.unsupported(s.lock)
		return
	}
	if s.distinct || len(s.distinctOn) > 0 || len(s.groupBy) > 0 || len(s.having) > 0 {
		b.SetErr(newError(ErrValidation, "build", sq.meta.Table, fmt.Errorf("%s cannot be combined with DISTINCT [ON], GROUP BY or HAVING", s.lock)))
		return
	}
	if !sq.checkLockOf(b) {
		return
	}
	b.writeByte(' ')
	b.WriteString(s.lock)
	for i, t := range s.lockOf {
		if i == 0 {
			b.WriteString(" OF ")
		} else {
			b.WriteString(", ")
		}
		b.Ident(t)
	}
	b.WriteString(s.lockWait)
}

// checkLockOf reports whether every LockOf name is the qualifier of the FROM
// table or of a joined table. A subquery, CTE or unaliased source has no
// lockable name, and naming one is a validation error here rather than a
// server error. Aggregates in the select list and set operations around this
// select are not detected; the server rejects those.
func (sq SelectQuery[T]) checkLockOf(b *Builder) bool {
	s := sq.s
	if len(s.lockOf) == 0 {
		return true
	}
	ctes := make(map[string]struct{}, len(s.with))
	for _, c := range s.with {
		ctes[c.name] = struct{}{}
	}
	known := map[string]struct{}{}
	for _, src := range append([]TableSource{s.from}, joinSources(s.joins)...) {
		if _, sub := src.(subquerySource); sub {
			continue
		}
		if _, no := src.(interface{ notLockable() }); no {
			continue // LATERAL, unnest and generate_series sources
		}
		if tr, ok := src.(TableRef); ok && tr.Schema == "" {
			if _, isCTE := ctes[tr.Name]; isCTE {
				continue // a WITH query cannot be locked
			}
		}
		if q, ok := src.(interface{ Qualifier() string }); ok {
			known[q.Qualifier()] = struct{}{}
		}
	}
	for _, t := range s.lockOf {
		if _, ok := known[t]; !ok {
			b.SetErr(newError(ErrValidation, "build", sq.meta.Table, fmt.Errorf("LockOf names %q, which is not a table or alias in the FROM or JOIN sources (a CTE or subquery cannot be locked)", t)))
			return false
		}
	}
	return true
}

// selectArity reports the number of columns the select yields, when known.
func (sq SelectQuery[T]) selectArity() (int, bool) {
	if sq.s.cols != nil {
		return len(sq.s.cols), true
	}
	if sq.meta.Table == "" {
		return 0, false
	}
	return len(sq.meta.Columns), true
}

// arityOf reports how many columns a subquery yields, when momo can tell.
func arityOf(q Subquery) (int, bool) {
	a, ok := q.(interface{ selectArity() (int, bool) })
	if !ok {
		return 0, false
	}
	return a.selectArity()
}
