// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"slices"
	"strconv"
)

// joinedLoader is the sealed Loader for single-query join loading of a
// belongs-to relation. It only works through SelectQuery.Preload.
type joinedLoader[P, C any] struct{ rel Relation[P, C] }

// joinLoader is implemented by joined loaders; Preload routes them to the
// parent statement instead of a separate relation load.
type joinLoader[P any] interface {
	Loader[P]
	// joinCheck reports why the relation cannot be loaded by join.
	joinCheck() error
	// joinExpand adds the LEFT JOIN under alias and the aliased child columns.
	joinExpand(parent, alias string, cols []Expr, joins []join) ([]Expr, []join)
	// joinWidth is the number of child columns joinExpand adds.
	joinWidth() int
	// joinAssign sets the to-one field from one row's child columns.
	joinAssign(p *P, vals []any) error
}

// Joined returns a loader that fetches the parent's belongs-to child in the
// parent statement itself, as a LEFT JOIN with aliased columns (momo_j<n>), so
// the relation costs no extra round trip. Pass it to SelectQuery.Preload;
// Load, LoadAll, Scope and Then do not take it.
//
// Only belongs-to relations are joinable, and only when the relation's child
// key is the child's primary key or a single unique column, because then the
// join cannot multiply parent rows: Limit, Offset, OrderBy and Where of the
// parent query keep their meaning. has-one, has-many and many-to-many fail
// with ErrUnsupported (Op "select") when the query runs, as do GroupBy,
// Having and row locking on the same query (FOR UPDATE cannot lock the
// nullable side of an outer join), and Each and Iter.
//
// Semantics match Load for belongs-to: a NULL (or part-NULL composite) foreign
// key, or a key matching no child, leaves the field nil, never a zero struct;
// children are excluded when soft-deleted, by a condition in the ON clause so
// the parent row survives; each parent gets its own copy of a shared child.
// The select list names parent columns by their qualifier, so a raw
// unqualified column in the parent's Where may become ambiguous.
//
// A joined load runs no statement of its own, so it emits no RelationEvent;
// the parent statement is reported to Hook as usual. Loaders for other
// relations in the same Preload still load as separate statements.
func (r Relation[P, C]) Joined() Loader[P] { return joinedLoader[P, C]{r} }

func (j joinedLoader[P, C]) relationName() string { return j.rel.Name }

func (j joinedLoader[P, C]) prepare(Querier, []P) (pendingLoad, error) {
	return nil, &Error{Kind: ErrUnsupported, Op: "load", Table: j.rel.Child.Name,
		Err: errors.New(j.rel.Name + ": a joined loader is applied by SelectQuery.Preload, not Load or LoadAll")}
}

func (j joinedLoader[P, C]) unsupported(msg string) error {
	return &Error{Kind: ErrUnsupported, Op: "select", Table: j.rel.Child.Name, Err: errors.New(j.rel.Name + ": " + msg)}
}

func (j joinedLoader[P, C]) joinCheck() error {
	r := &j.rel
	if err := r.validate(); err != nil {
		return err
	}
	if r.Kind != BelongsTo {
		return j.unsupported("join loading applies to belongs-to relations; " + r.Kind.String() + " can match several children per parent")
	}
	if !r.childKeyUnique() {
		return j.unsupported("join loading needs the child key to be the child's primary key or a unique column")
	}
	return nil
}

// childKeyUnique reports whether ChildCols identify at most one child row: the
// primary key (as a set), or a single column flagged primary key or unique.
func (r *Relation[P, C]) childKeyUnique() bool {
	if len(r.ChildCols) == len(r.ChildPK) {
		same := true
		for _, c := range r.ChildCols {
			same = same && slices.Contains(r.ChildPK, c)
		}
		if same {
			return true
		}
	}
	if len(r.ChildCols) != 1 {
		return false
	}
	for _, col := range r.ChildMeta.Columns {
		if col.Name == r.ChildCols[0] {
			return col.Flags&(FlagPK|FlagUnique) != 0 && len(r.ChildMeta.PK) <= 1
		}
	}
	return false
}

// aliasedCol is a select-list column with an output label.
type aliasedCol struct {
	qcol
	label string
}

func (c aliasedCol) AppendSQL(b *Builder) {
	c.qcol.AppendSQL(b)
	b.WriteString(" AS ")
	b.Ident(c.label)
}

func (j joinedLoader[P, C]) joinExpand(parent, alias string, cols []Expr, joins []join) ([]Expr, []join) {
	r := &j.rel
	on := make([]Predicate, 0, len(r.ParentCols)+1)
	for i, pc := range r.ParentCols {
		on = append(on, binOp{qcol{alias, r.ChildCols[i]}, " = ", qcol{parent, pc}})
	}
	// The soft-delete filter belongs in ON: in WHERE it would drop the parent.
	if m := r.ChildMeta; m.SoftDelete >= 0 {
		on = append(on, softDeletePred(m, alias, false))
	}
	for i, c := range r.ChildMeta.Columns {
		cols = append(cols, aliasedCol{qcol{alias, c.Name}, alias + "_" + strconv.Itoa(i)})
	}
	src := TableRef{Name: r.Child.Name, Schema: r.Child.Schema, Alias: alias}
	return cols, append(joins, join{kind: "LEFT JOIN", src: src, on: on})
}

func (j joinedLoader[P, C]) joinWidth() int { return len(j.rel.ChildMeta.Columns) }

// joinAssign sets the parent's field from one row's child columns. A LEFT JOIN
// without a match yields NULL in every child column, in particular in the
// matched key columns, which are never NULL on a real child.
func (j joinedLoader[P, C]) joinAssign(p *P, vals []any) error {
	r := &j.rel
	matched := false
	for i, col := range r.ChildMeta.Columns {
		if vals[i] != nil && slices.Contains(r.ChildCols, col.Name) {
			matched = true
		}
	}
	if !matched {
		r.SetOne(p, nil)
		return nil
	}
	var c C
	for i, v := range vals {
		if err := ScanValue(r.ChildMeta.Ptr(&c, i), v); err != nil {
			return err
		}
	}
	r.SetOne(p, &c)
	return nil
}

// expandJoins returns the query to run when joined loaders are present: the
// parent columns, then one aliased block per joined loader, plus the LEFT
// JOINs. It also returns the loaders, in block order. Without joined loaders
// it returns sq unchanged. It is applied at execution and in SQL, not at
// Preload, so clauses added after Preload (Columns, joins, Where) compose.
func (sq SelectQuery[T]) expandJoins() (SelectQuery[T], []joinLoader[T], error) {
	if len(sq.s.joinLoads) == 0 {
		return sq, nil, nil
	}
	ls := make([]joinLoader[T], len(sq.s.joinLoads))
	for i, l := range sq.s.joinLoads {
		ls[i] = l.(joinLoader[T])
		if err := ls[i].joinCheck(); err != nil {
			return sq, nil, err
		}
	}
	unsupported := func(what string) error {
		return &Error{Kind: ErrUnsupported, Op: "select", Table: sq.meta.Table, Err: errors.New("Joined relations cannot be combined with " + what)}
	}
	switch {
	case len(sq.s.groupBy) > 0 || len(sq.s.having) > 0:
		return sq, nil, unsupported("GroupBy or Having")
	case sq.s.lock != "":
		return sq, nil, unsupported("row locking")
	}
	parent := sq.qualifier()
	cols := slices.Clone(sq.s.cols)
	switch {
	case cols == nil:
		for _, c := range sq.meta.Columns {
			cols = append(cols, qcol{parent, c.Name})
		}
	case len(cols) != len(sq.meta.Columns):
		return sq, nil, &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table,
			Err: errors.New("select list must have one expression per mapped field to preload a joined relation")}
	}
	joins := slices.Clip(sq.s.joins)
	for i, l := range ls {
		cols, joins = l.joinExpand(parent, "momo_j"+strconv.Itoa(i), cols, joins)
	}
	sq.s.cols, sq.s.joins, sq.s.joinLoads = cols, joins, nil
	sq.s.keysOnly = true // the select list is wider than the model's column list
	return sq, ls, nil
}

// joinScan scans the child blocks of a joined query into scratch values and
// assigns them to the parent after each row.
type joinScan[T any] struct {
	loaders []joinLoader[T]
	vals    []any
}

func newJoinScan[T any](ls []joinLoader[T]) *joinScan[T] {
	n := 0
	for _, l := range ls {
		n += l.joinWidth()
	}
	return &joinScan[T]{loaders: ls, vals: make([]any, n)}
}

// bind points dst at the scratch values.
func (js *joinScan[T]) bind(dst []any) {
	for i := range js.vals {
		js.vals[i] = nil
		dst[i] = &js.vals[i]
	}
}

// assign sets every joined relation on m from the scanned row.
func (js *joinScan[T]) assign(m *T) error {
	off := 0
	for _, l := range js.loaders {
		w := l.joinWidth()
		if err := l.joinAssign(m, js.vals[off:off+w]); err != nil {
			return err
		}
		off += w
	}
	return nil
}
