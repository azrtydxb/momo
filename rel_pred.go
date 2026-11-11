// SPDX-License-Identifier: Apache-2.0

package momo

import "strconv"

// Direction orders by relation count. DirAsc and DirDesc avoid the names
// Asc(e Expr) and Desc(e Expr) already used in expr.go.
type Direction uint8

const (
	// DirAsc orders by ascending count.
	DirAsc Direction = iota
	// DirDesc orders by descending count.
	DirDesc
)

// relAlias is the child alias used inside EXISTS and COUNT subqueries. Inside
// a scope predicate it always means the child of the subquery the predicate
// belongs to: a nested subquery renders under relLevelAlias(relAlias, depth),
// and the builder resolves this name to the innermost level.
const relAlias = "momo_r"

// relJoinAlias is the join-table alias of many-to-many correlated subqueries.
const relJoinAlias = "momo_j"

// relLevelAlias returns base for the outermost subquery and base plus the
// level number (momo_r2, momo_r3, ...) for each level nested inside it.
func relLevelAlias(base string, depth int) string {
	if depth == 0 {
		return base
	}
	return base + strconv.Itoa(depth+1)
}

// relCorr is the non-generic part of a relation used by correlated predicates.
type relCorr struct {
	parentQual string
	// explicitParent is set by HasFrom: the caller named the parent alias, so
	// an enclosing subquery's alias never replaces it.
	explicitParent bool
	child          TableRef
	parentCols     []string
	childCols      []string
	through        *Through
	disc           *Discriminator
	soft           Predicate // child soft-delete filter, nil when the child has none
}

// correlated renders EXISTS / NOT EXISTS / COUNT over the child table,
// correlated with the parent through the relation's key columns.
type correlated struct {
	r      *relCorr
	scope  []Predicate
	count  bool
	negate bool
}

func (x correlated) AppendSQL(b *Builder) {
	if x.count {
		b.WriteString("(SELECT COUNT(*) FROM ")
	} else {
		if x.negate {
			b.WriteString("NOT ")
		}
		b.WriteString("EXISTS (SELECT 1 FROM ")
	}
	r := x.r
	depth := len(b.rels)
	alias := relLevelAlias(relAlias, depth)
	joinAlias := relLevelAlias(relJoinAlias, depth)
	// A relation whose parent is the child of the enclosing subquery
	// correlates on that level's alias. An explicit alias (HasFrom) is kept.
	parentQual := r.parentQual
	if !r.explicitParent && depth > 0 && parentQual == b.rels[depth-1].table {
		parentQual = b.rels[depth-1].alias
	}
	child := TableRef{Name: r.child.Name, Schema: r.child.Schema, Alias: alias}
	names := []string{alias}
	if r.through != nil {
		names = append(names, joinAlias)
	}
	b.withScope(names, func() {
		b.rels = append(b.rels, relFrame{alias: alias, table: r.child.Qualifier()})
		defer func() { b.rels = b.rels[:len(b.rels)-1] }()
		if r.through != nil {
			r.through.Table.appendFrom(b)
			b.WriteString(" AS ")
			b.Ident(joinAlias)
			b.WriteString(" INNER JOIN ")
			child.appendFrom(b)
			b.WriteString(" ON ")
			for i, c := range r.through.ChildCols {
				if i > 0 {
					b.WriteString(" AND ")
				}
				b.qualifiedExact(alias, r.childCols[i])
				b.WriteString(" = ")
				b.qualifiedExact(joinAlias, c)
			}
			b.WriteString(" WHERE ")
			for i, c := range r.through.ParentCols {
				if i > 0 {
					b.WriteString(" AND ")
				}
				b.qualifiedExact(joinAlias, c)
				b.WriteString(" = ")
				b.qualifiedExact(parentQual, r.parentCols[i])
			}
		} else {
			child.appendFrom(b)
			b.WriteString(" WHERE ")
			for i, c := range r.childCols {
				if i > 0 {
					b.WriteString(" AND ")
				}
				b.qualifiedExact(alias, c)
				b.WriteString(" = ")
				b.qualifiedExact(parentQual, r.parentCols[i])
			}
		}
		if r.disc != nil {
			b.WriteString(" AND ")
			binOp{qcol{alias, r.disc.Column}, " = ", argExpr{v: r.disc.Value}}.AppendSQL(b)
		}
		if r.soft != nil {
			b.WriteString(" AND ")
			r.soft.AppendSQL(b)
		}
		for _, p := range x.scope {
			b.WriteString(" AND (")
			p.AppendSQL(b)
			b.WriteString(")")
		}
	})
	b.WriteString(")")
}

func (correlated) resultType() bool { return false }

// Has returns EXISTS over the child table, correlated with the parent. The
// predicate is a subquery, never a JOIN, so it cannot duplicate parent rows.
//
// Scope predicates must reference the child through the alias "momo_r": build
// them with As on the generated column, for example
// comment.Approved.As("momo_r").Eq(true). Has, HasNo and CountOrder nest: a
// nested subquery has its own alias (momo_r2, momo_r3, ...), "momo_r" in its
// scope names its own child, and a relation whose parent is the enclosing
// child correlates on the enclosing alias. Soft-deleted children do not count.
// For many-to-many relations the join table gets no soft-delete filter: only
// the child table's marker is applied.
func (r Relation[P, C]) Has(scope ...Predicate) Predicate {
	return correlated{r: r.corr(r.Parent.Qualifier()), scope: scope}
}

// HasFrom is Has correlated on an explicit parent alias. Self-referencing
// relations need it: the parent table is aliased in the outer query and the
// alias is passed here.
func (r Relation[P, C]) HasFrom(parentAlias string, scope ...Predicate) Predicate {
	c := r.corr(parentAlias)
	c.explicitParent = true
	return correlated{r: c, scope: scope}
}

// HasNo returns NOT EXISTS over the child table. Scope predicates follow the
// rules of Has.
func (r Relation[P, C]) HasNo(scope ...Predicate) Predicate {
	return correlated{r: r.corr(r.Parent.Qualifier()), scope: scope, negate: true}
}

// CountOrder orders parents by the number of related children, using a
// correlated COUNT(*) subquery.
func (r Relation[P, C]) CountOrder(dir Direction) Orderable {
	return Order{Expr: correlated{r: r.corr(r.Parent.Qualifier()), count: true}, Desc: dir == DirDesc}
}

func (r Relation[P, C]) corr(parentQual string) *relCorr {
	c := &relCorr{parentQual: parentQual, child: r.Child, parentCols: r.ParentCols, childCols: r.ChildCols, through: r.Through, disc: r.Discriminator}
	if m := r.ChildMeta; m != nil && m.SoftDelete >= 0 {
		c.soft = softDeletePred(m, relAlias, false)
	}
	return c
}

// From returns the children whose key is in the parents' key set. The parents
// query is embedded as a subquery, so no parent row is fetched. Many-to-many
// relations are not supported by From; the returned query fails with
// ErrValidation.
func (r Relation[P, C]) From(parents SelectQuery[P]) SelectQuery[C] {
	if r.Through != nil {
		bad := selectFrom(parents.q, r.ChildMeta, r.Child)
		bad.s.err = r.invalid("From does not support many-to-many relations")
		return bad
	}
	keyCols := make([]Expr, len(r.ParentCols))
	for i, c := range r.ParentCols {
		keyCols[i] = qcol{parents.qualifier(), c}
	}
	keys := parents.Columns(keyCols...)
	keys.s.keysOnly = true
	var where Predicate = tupleInSubquery{table: r.Child.Qualifier(), cols: r.ChildCols, sub: keys}
	if r.Discriminator != nil {
		where = And(binOp{qcol{r.Child.Qualifier(), r.Discriminator.Column}, " = ", argExpr{v: r.Discriminator.Value}}, where)
	}
	return selectFrom(parents.q, r.ChildMeta, r.Child).Where(where)
}

// tupleInSubquery renders (cols) IN (subquery).
type tupleInSubquery struct {
	table string
	cols  []string
	sub   Subquery
}

func (x tupleInSubquery) AppendSQL(b *Builder) {
	if len(x.cols) == 1 {
		b.QualifiedIdent(x.table, x.cols[0])
	} else {
		b.WriteString("(")
		for i, c := range x.cols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(x.table, c)
		}
		b.WriteString(")")
	}
	b.WriteString(" IN ")
	x.sub.AppendSQL(b)
}

func (tupleInSubquery) resultType() bool { return false }
