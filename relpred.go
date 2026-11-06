// SPDX-License-Identifier: Apache-2.0

package momo

// Direction orders by relation count. DirAsc and DirDesc avoid the names
// Asc(e Expr) and Desc(e Expr) already used in expr.go.
type Direction uint8

const (
	// DirAsc orders by ascending count.
	DirAsc Direction = iota
	// DirDesc orders by descending count.
	DirDesc
)

// relAlias is the child alias used inside EXISTS and COUNT subqueries.
const relAlias = "momo_r"

// relJoinAlias is the join-table alias of many-to-many correlated subqueries.
const relJoinAlias = "momo_j"

// relCorr is the non-generic part of a relation used by correlated predicates.
type relCorr struct {
	parentQual string
	child      TableRef
	parentCols []string
	childCols  []string
	through    *Through
	disc       *Discriminator
	soft       Predicate // child soft-delete filter, nil when the child has none
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
	child := TableRef{Name: r.child.Name, Schema: r.child.Schema, Alias: relAlias}
	names := []string{relAlias}
	if r.through != nil {
		names = append(names, relJoinAlias)
	}
	b.withScope(names, func() {
		if r.through != nil {
			r.through.Table.appendFrom(b)
			b.WriteString(" AS ")
			b.Ident(relJoinAlias)
			b.WriteString(" INNER JOIN ")
			child.appendFrom(b)
			b.WriteString(" ON ")
			for i, c := range r.through.ChildCols {
				if i > 0 {
					b.WriteString(" AND ")
				}
				b.QualifiedIdent(relAlias, r.childCols[i])
				b.WriteString(" = ")
				b.QualifiedIdent(relJoinAlias, c)
			}
			b.WriteString(" WHERE ")
			for i, c := range r.through.ParentCols {
				if i > 0 {
					b.WriteString(" AND ")
				}
				b.QualifiedIdent(relJoinAlias, c)
				b.WriteString(" = ")
				b.QualifiedIdent(r.parentQual, r.parentCols[i])
			}
		} else {
			child.appendFrom(b)
			b.WriteString(" WHERE ")
			for i, c := range r.childCols {
				if i > 0 {
					b.WriteString(" AND ")
				}
				b.QualifiedIdent(relAlias, c)
				b.WriteString(" = ")
				b.QualifiedIdent(r.parentQual, r.parentCols[i])
			}
		}
		if r.disc != nil {
			b.WriteString(" AND ")
			binOp{qcol{relAlias, r.disc.Column}, " = ", argExpr{r.disc.Value}}.AppendSQL(b)
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
// comment.Approved.As("momo_r").Eq(true). Soft-deleted children do not count.
// For many-to-many relations the join table gets no soft-delete filter: only
// the child table's marker is applied.
func (r Relation[P, C]) Has(scope ...Predicate) Predicate {
	return correlated{r: r.corr(r.Parent.Qualifier()), scope: scope}
}

// HasFrom is Has correlated on an explicit parent alias. Self-referencing
// relations need it: the parent table is aliased in the outer query and the
// alias is passed here.
func (r Relation[P, C]) HasFrom(parentAlias string, scope ...Predicate) Predicate {
	return correlated{r: r.corr(parentAlias), scope: scope}
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
		where = And(binOp{qcol{r.Child.Qualifier(), r.Discriminator.Column}, " = ", argExpr{r.Discriminator.Value}}, where)
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
