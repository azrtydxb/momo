// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"slices"
)

// The emulation of DISTINCT ON for dialects without it (SQLite, MySQL and
// MariaDB) renders
//
//	SELECT d.c0 AS name0, d.c1 AS name1 FROM (
//	  SELECT <expr0> AS momo_c0, <expr1> AS momo_c1,
//	         <order0> AS momo_o0, <order1> AS momo_o1,
//	         ROW_NUMBER() OVER (PARTITION BY <keys> ORDER BY <orders>) AS momo_rn
//	  FROM ... WHERE ... GROUP BY ... HAVING ...
//	) AS momo_d WHERE momo_d.momo_rn = 1
//	ORDER BY momo_d.momo_o0, momo_d.momo_o1 LIMIT ... OFFSET ...
//
// Every select-list item is aliased to momo_cN inside the derived table, so a
// join that selects two columns of the same name does not trip MySQL's
// duplicate-column check, and the outer query gives each item back its own
// name (the column name, or the As alias) so a raw read of the statement sees
// the names PostgreSQL would report. Every ORDER BY expression is projected as
// a hidden momo_oN column, so an ordering by a column that is not in the select
// list still works in the outer ORDER BY, which cannot see the inner tables.
// The helper columns never reach the outer select list, so a scan reads exactly
// the columns of the query. An item that is neither a column nor an As alias
// has no name to give back and keeps momo_cN: alias it with As to name it, and
// a subquery or CTE that reads such an item by its PostgreSQL name must alias
// it too. An As alias is checked as Aliased.AppendSQL checks it.
//
// Count and Exists wrap the statement. With the real select list the derived
// table could repeat a name (a join that selects p.id and c.id), which MySQL
// rejects, so a count renders the outer query as SELECT 1 AS momo_one over the
// same rows, without the ORDER BY.
//
// A nullable ORDER BY item with no NULLS term gets PostgreSQL's placement
// (ASC NULLS LAST, DESC NULLS FIRST) written out, so the tiebreak picks the
// same row on every dialect. This applies where momo knows the column is
// nullable (a model column or a NullColumn); for ColumnOf and other
// expressions it does not, so give those an explicit NullsFirst or NullsLast.
const (
	distinctDerived = "momo_d"
	distinctRowNum  = "momo_rn"
)

// derivedCol is a column of the derived table, qualified by its alias.
type derivedCol string

func (c derivedCol) AppendSQL(b *Builder) { b.QualifiedIdent(distinctDerived, string(c)) }
func (c derivedCol) ColumnName() string   { return string(c) }

// unalias returns the expression and the alias of a select-list item.
func (a Aliased[T]) unalias() (Expr, string) { return a.e, a.alias }

// derivedItem is one select-list item of the emulated query.
type derivedItem struct {
	expr    Expr
	name    string // the name the outer query gives the item back, or empty
	aliased bool   // the item is an As alias, which ORDER BY may name
}

// derivedItems lists the select-list items, or false after recording an
// error. It validates the list as writeSelectList does.
func (sq SelectQuery[T]) derivedItems(b *Builder) ([]derivedItem, bool) {
	s := &sq.s
	if s.cols == nil {
		if sq.meta.Table == "" {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("projection query needs Columns")})
			return nil, false
		}
		q := sq.qualifier()
		items := make([]derivedItem, len(sq.meta.Columns))
		for i, c := range sq.meta.Columns {
			items[i] = derivedItem{expr: qcol{q, c.Name}, name: c.Name}
		}
		return items, true
	}
	if len(s.cols) != len(sq.meta.Columns) && !s.keysOnly {
		b.SetErr(newError(ErrValidation, "build", sq.meta.Table, fmt.Errorf("select list has %d expressions but result type maps %d fields", len(s.cols), len(sq.meta.Columns))))
		return nil, false
	}
	items := make([]derivedItem, len(s.cols))
	for i, e := range s.cols {
		it := derivedItem{expr: e}
		if a, ok := e.(interface{ unalias() (Expr, string) }); ok {
			it.expr, it.name = a.unalias()
			it.aliased = true
			// Aliased.AppendSQL checks the alias; the emulation renders the
			// expression itself, so it checks here and refuses what PostgreSQL
			// refuses.
			if err := validName(it.name); err != nil {
				b.SetErr(&Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: fmt.Errorf("alias %q: %w", it.name, err)})
				return nil, false
			}
		} else if c, ok := e.(interface{ ColumnName() string }); ok {
			it.name = c.ColumnName()
		}
		items[i] = it
	}
	return items, true
}

// effectiveOrder is the ORDER BY the statement sorts by: the OrderBy items,
// and for a keyset query the primary-key tie-breakers After appends.
func (sq SelectQuery[T]) effectiveOrder() []Order {
	s := &sq.s
	if s.keyset == nil {
		return s.order
	}
	keys := sq.keysetKeys()
	out := make([]Order, len(keys))
	for i, k := range keys {
		if i < len(s.order) {
			out[i] = s.order[i]
		} else {
			out[i] = Order{Expr: k.expr, Desc: k.desc}
		}
	}
	return out
}

// renderDistinctOnWindow writes the whole statement (WITH clause included) for
// a DISTINCT ON query on a dialect without the clause.
func (sq SelectQuery[T]) renderDistinctOnWindow(b *Builder) {
	s := &sq.s
	if !b.d.WindowFunctions {
		b.unsupported("DISTINCT ON (it is emulated with ROW_NUMBER, which needs window functions)")
		return
	}
	if s.lock != "" {
		b.SetErr(newError(ErrUnsupported, "build", sq.meta.Table, fmt.Errorf("DISTINCT ON with %s is not supported by %s: the emulation reads the rows in a derived table, so the lock would not cover the rows it returns", s.lock, b.d.Name)))
		return
	}
	keys, ok := sq.validateDistinctOn(b)
	if !ok {
		return
	}
	items, ok := sq.derivedItems(b)
	if !ok {
		return
	}
	// An ORDER BY on a select-list alias orders by the aliased expression:
	// MySQL does not resolve a select alias inside a window's ORDER BY.
	order := slices.Clone(sq.effectiveOrder())
	for i, o := range order {
		ref, isAlias := o.Expr.(aliasRef)
		if !isAlias {
			continue
		}
		found := false
		for _, it := range items {
			if it.aliased && it.name == string(ref) {
				order[i].Expr, found = it.expr, true
				break
			}
		}
		if !found {
			b.SetErr(newError(ErrValidation, "build", sq.meta.Table, fmt.Errorf("ORDER BY alias %q is not in the select list", string(ref))))
			return
		}
	}
	// PostgreSQL puts NULLs last in an ascending order and first in a
	// descending one; SQLite and MySQL put them the other way round. Where momo
	// knows the expression is nullable, render the PostgreSQL placement
	// explicitly so the tiebreak picks the same row. A column declared without
	// nullability (ColumnOf, an expression) is not known to be nullable: give
	// such an item an explicit NullsFirst or NullsLast.
	for i, o := range order {
		if o.Nulls == NullsDefault && FlagsOf(o.Expr)&FlagNullable != 0 {
			if o.Desc {
				order[i].Nulls = NullsFirst
			} else {
				order[i].Nulls = NullsLast
			}
		}
	}

	writeWithClause(b, s.with)
	b.withScope([]string{distinctDerived}, func() {
		b.WriteString("SELECT ")
		if s.distinctCount {
			b.WriteString("1 AS ")
			b.Ident("momo_one")
			return
		}
		for i, it := range items {
			if i > 0 {
				b.WriteString(", ")
			}
			derivedCol(distinctName("momo_c", i)).AppendSQL(b)
			if it.name != "" {
				b.WriteString(" AS ")
				b.Ident(it.name)
			}
		}
	})
	b.WriteString(" FROM (SELECT ")
	for i, it := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		it.expr.AppendSQL(b)
		b.WriteString(" AS ")
		b.Ident(distinctName("momo_c", i))
	}
	for i, o := range order {
		b.WriteString(", ")
		o.Expr.AppendSQL(b)
		b.WriteString(" AS ")
		b.Ident(distinctName("momo_o", i))
	}
	b.WriteString(", ROW_NUMBER() OVER (PARTITION BY ")
	appendList(b, keys)
	b.WriteString(" ORDER BY ")
	for i, o := range order {
		if i > 0 {
			b.WriteString(", ")
		}
		o.appendSQL(b)
	}
	b.WriteString(") AS ")
	b.Ident(distinctRowNum)
	if !s.writeFrom(b, sq.meta.Table) {
		return
	}
	s.writeJoins(b)
	s.writeWhere(b, sq.effectiveWhere(b), false)
	s.writeGroupHaving(b)
	b.WriteString(") AS ")
	b.Ident(distinctDerived)
	b.withScope([]string{distinctDerived}, func() {
		b.WriteString(" WHERE ")
		b.QualifiedIdent(distinctDerived, distinctRowNum)
		b.WriteString(" = 1")
		if s.distinctCount {
			return // a count needs no order
		}
		b.WriteString(" ORDER BY ")
		for i, o := range order {
			if i > 0 {
				b.WriteString(", ")
			}
			Order{Expr: derivedCol(distinctName("momo_o", i)), Desc: o.Desc, Nulls: o.Nulls}.appendSQL(b)
		}
	})
	s.writeLimitOffset(b)
	sq.writeLock(b) // no lock here; it reports a LockOf without one
}

func distinctName(prefix string, i int) string { return fmt.Sprintf("%s%d", prefix, i) }
