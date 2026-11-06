// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"reflect"
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
	// joinBinder returns the per-scan state that scans one row's child columns.
	joinBinder() joinBinder[P]
}

// joinBinder scans the child block of one joined relation. bind is called
// before each row; assign after Scan, to set the to-one field.
type joinBinder[P any] interface {
	bind(dst []any)
	assign(p *P) error
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

// joinBinding scans a joined child block into a scratch child through the
// model's own destinations (Meta.Ptr), so every column scans exactly as a
// non-joined Load of the child would. A LEFT JOIN without a match yields NULL in
// every child column, which a non-nullable destination would reject; so each
// destination that cannot hold NULL is scanned through a pointer to a fresh
// pointer (**T), which both pgx and database/sql fill with nil for NULL and
// otherwise convert exactly as for *T. Once the key columns show a match, a
// NULL in such a column gets the same treatment as in normal scanning (Scan(nil)
// for a sql.Scanner, nil for slices, an error otherwise).
type joinBinding[P, C any] struct {
	rel  *Relation[P, C]
	c    *C
	dst  []reflect.Value // destination pointer (*E) per column
	tmp  []reflect.Value // **E holder per column; invalid when scanned directly
	keys []bool          // column is one of the relation's child key columns
}

func (j joinedLoader[P, C]) joinBinder() joinBinder[P] {
	n := len(j.rel.ChildMeta.Columns)
	b := &joinBinding[P, C]{rel: &j.rel, dst: make([]reflect.Value, n), tmp: make([]reflect.Value, n), keys: make([]bool, n)}
	for i, col := range j.rel.ChildMeta.Columns {
		b.keys[i] = slices.Contains(j.rel.ChildCols, col.Name)
	}
	return b
}

func (b *joinBinding[P, C]) bind(dst []any) {
	b.c = new(C)
	for i := range b.dst {
		p := b.rel.ChildMeta.Ptr(b.c, i)
		v := reflect.ValueOf(p)
		b.dst[i] = v
		if v.Elem().Kind() == reflect.Pointer {
			b.tmp[i] = reflect.Value{} // nullable: NULL leaves the pointer nil
			dst[i] = p
			continue
		}
		b.tmp[i] = reflect.New(reflect.PointerTo(v.Elem().Type()))
		dst[i] = b.tmp[i].Interface()
	}
}

// null reports whether column i of the scanned row was NULL.
func (b *joinBinding[P, C]) null(i int) bool {
	if t := b.tmp[i]; t.IsValid() {
		return t.Elem().IsNil()
	}
	return b.dst[i].Elem().IsNil()
}

// assign sets the parent's field from the scanned row: nil when the key columns
// are NULL (no child matched), else the scratch child.
func (b *joinBinding[P, C]) assign(p *P) error {
	matched := false
	for i, k := range b.keys {
		if k && !b.null(i) {
			matched = true
		}
	}
	if !matched {
		b.rel.SetOne(p, nil)
		return nil
	}
	for i, t := range b.tmp {
		if !t.IsValid() {
			continue
		}
		if t.Elem().IsNil() {
			if err := scanNull(b.dst[i].Interface()); err != nil {
				return err
			}
			continue
		}
		b.dst[i].Elem().Set(t.Elem().Elem())
	}
	b.rel.SetOne(p, b.c)
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

// joinScan scans the child blocks of a joined query and assigns them to the
// parent after each row.
type joinScan[T any] struct {
	binders []joinBinder[T]
	widths  []int
	width   int
}

func newJoinScan[T any](ls []joinLoader[T]) *joinScan[T] {
	js := &joinScan[T]{}
	for _, l := range ls {
		js.binders = append(js.binders, l.joinBinder())
		js.widths = append(js.widths, l.joinWidth())
		js.width += l.joinWidth()
	}
	return js
}

// bind points dst at the child destinations of a fresh row.
func (js *joinScan[T]) bind(dst []any) {
	off := 0
	for i, b := range js.binders {
		b.bind(dst[off : off+js.widths[i]])
		off += js.widths[i]
	}
}

// assign sets every joined relation on m from the scanned row.
func (js *joinScan[T]) assign(m *T) error {
	for _, b := range js.binders {
		if err := b.assign(m); err != nil {
			return err
		}
	}
	return nil
}
