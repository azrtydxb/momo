// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"

	"github.com/azrtydxb/momo/dialect"
)

// pending is one relation's load in progress: its child statements, the parent
// key groups, and the child rows collected so far.
type pending[P, C any] struct {
	r       *Relation[P, C]
	q       Querier
	parents []P
	cfg     *loadConfig
	nested  []Loader[C]
	groups  map[string][]int // parent key -> parent indexes
	keys    [][]any          // distinct parent keys, first-seen order
	queries []Compiled
	pool    []C            // child rows, in query order
	owners  [][]int        // owners[j]: parent indexes that receive pool[j]
	ev      *RelationEvent // event reported to relation hooks
}

// prepareWith validates the descriptor, clears existing associations and
// compiles the child statements. It is the single entry point used by all
// loaders.
func (r *Relation[P, C]) prepareWith(q Querier, parents []P, cfg *loadConfig, nested []Loader[C]) (pendingLoad, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	if cfg.perParentLimit != 0 {
		return nil, &Error{Kind: ErrUnsupported, Op: "load", Table: r.Child.Name,
			Err: errors.New(r.Name + ": LoadPerParentLimit is not supported yet")}
	}
	pd := &pending[P, C]{r: r, q: q, parents: parents, cfg: cfg, nested: nested, groups: map[string][]int{}}
	for i := range parents {
		r.clear(&parents[i])
		vals, ok := r.ParentKey(&parents[i])
		if !ok || hasNilPart(vals) {
			continue // NULL in any component means no key
		}
		k := keyString(vals)
		if _, seen := pd.groups[k]; !seen {
			pd.keys = append(pd.keys, vals)
		}
		pd.groups[k] = append(pd.groups[k], i)
	}
	qs, err := r.childQueries(q, pd.keys, cfg)
	if err != nil {
		return nil, err
	}
	pd.queries = qs
	return pd, nil
}

// clear empties the association before loading, so a parent that matches
// nothing (or has a NULL key) ends up empty rather than stale.
func (r *Relation[P, C]) clear(p *P) {
	if r.Kind == BelongsTo || r.Kind == HasOne {
		r.SetOne(p, nil)
		return
	}
	r.SetMany(p, nil)
}

// run executes the child statements one at a time, consuming each result set
// before the next statement starts (a single pooled connection must never be
// held by two open result sets). Nested loads run in finish.
func (pd *pending[P, C]) run(ctx context.Context, q Querier) error {
	ctx, end := pd.begin(ctx, q)
	err := pd.runQueries(ctx, q)
	if err == nil {
		err = pd.finish(ctx)
	}
	end(err)
	return err
}

// begin reports the start of this relation load to relation hooks.
func (pd *pending[P, C]) begin(ctx context.Context, q Querier) (context.Context, func(error)) {
	return beginRelation(ctx, q.conn().db, pd.event())
}

// event returns this load's RelationEvent, created on first use.
func (pd *pending[P, C]) event() *RelationEvent {
	if pd.ev == nil {
		pd.ev = &RelationEvent{Parent: pd.r.Parent.Name, Name: pd.r.Name, Kind: pd.r.Kind, Parents: len(pd.parents), Queries: len(pd.queries)}
	}
	return pd.ev
}

func (pd *pending[P, C]) runQueries(ctx context.Context, q Querier) error {
	rel := pd.r.Parent.Name + "." + pd.r.Name
	for _, c := range pd.queries {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := q.conn().query(ctx, "select", pd.r.Child.Name, rel, c)
		if err != nil {
			return err
		}
		if err := pd.consume(rows); err != nil {
			return err
		}
	}
	return nil
}

// consume reads one result set into the pool and closes it. Rows whose key is
// NULL or matches no requested parent are dropped.
func (pd *pending[P, C]) consume(rows Rows) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c C
		if err := scanChild(pd.r, rows, &c); err != nil {
			return pd.wrap(err)
		}
		key, ok := pd.r.ChildKey(&c)
		if !ok {
			continue
		}
		owners := pd.groups[keyString(key)]
		if len(owners) == 0 {
			continue
		}
		pd.pool = append(pd.pool, c)
		pd.owners = append(pd.owners, owners)
	}
	return pd.wrap(rows.Err())
}

// wrap classifies a scan or iteration error like a query error, tagged Op "load".
func (pd *pending[P, C]) wrap(err error) error {
	return pd.q.conn().wrapErr("load", pd.r.Child.Name, err)
}

// finish runs nested loads once over the whole pool, then assigns children to
// their parents. To-one children are copied per parent; to-many children are
// collected per parent in pool order, so association order is stable.
func (pd *pending[P, C]) finish(ctx context.Context) error {
	if pd.ev != nil {
		pd.ev.Children = int64(len(pd.pool))
	}
	if len(pd.nested) > 0 && len(pd.pool) > 0 {
		if err := LoadAll(ctx, pd.q, pd.pool, pd.nested...); err != nil {
			return err
		}
	}
	toOne := pd.r.Kind == BelongsTo || pd.r.Kind == HasOne
	counts := make([]int, len(pd.parents))
	lists := make([][]C, len(pd.parents))
	for j := range pd.pool {
		for _, i := range pd.owners[j] {
			counts[i]++
			if toOne {
				if counts[i] > 1 && pd.r.Kind == HasOne {
					return &Error{Kind: ErrTooManyRows, Op: "load", Table: pd.r.Child.Name,
						Err: errors.New(pd.r.Name + ": has-one matched more than one child")}
				}
				cp := pd.pool[j]
				pd.r.SetOne(&pd.parents[i], &cp)
				continue
			}
			lists[i] = append(lists[i], pd.pool[j])
		}
	}
	if !toOne {
		for i, l := range lists {
			if len(l) > 0 {
				pd.r.SetMany(&pd.parents[i], l)
			}
		}
	}
	return nil
}

// childQueries builds one statement per bind-limit chunk for the BelongsTo,
// HasOne and HasMany shapes. Keys are never reordered, so chunks and rows
// follow parent first-seen order.
func (r *Relation[P, C]) childQueries(q Querier, keys [][]any, cfg *loadConfig) ([]Compiled, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	d := q.Dialect()
	scopeArgs, err := countPredicateArgs(d, cfg.where)
	if err != nil {
		return nil, err
	}
	if r.Discriminator != nil {
		scopeArgs++
	}
	if d.MaxParams > 0 && scopeArgs+len(r.ChildCols) > d.MaxParams {
		return nil, &Error{Kind: ErrValidation, Op: "load", Table: r.Child.Name,
			Err: fmt.Errorf("%s: scope binds %d arguments, leaving no room for a key under the %d-parameter limit", r.Name, scopeArgs, d.MaxParams)}
	}
	reserved := 16 + scopeArgs
	budget := keysPerStatement(d, len(r.ChildCols), reserved)
	var out []Compiled
	for start := 0; start < len(keys); start += budget {
		end := min(start+budget, len(keys))
		chunk := keys[start:end]
		c, err := compile(d, func(b *Builder) {
			b.WriteString("SELECT ")
			r.writeChildColumns(b)
			b.WriteString(" FROM ")
			r.Child.appendFrom(b)
			b.WriteString(" WHERE ")
			inTuple{table: r.Child.Qualifier(), cols: r.ChildCols, keys: chunk}.AppendSQL(b)
			r.writeScope(b, cfg)
			b.WriteString(" ORDER BY ")
			r.writeOrder(b, cfg)
		})
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *Relation[P, C]) writeChildColumns(b *Builder) {
	for i, col := range r.ChildMeta.Columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.QualifiedIdent(r.Child.Qualifier(), col.Name)
	}
}

// writeScope appends the discriminator and user predicates with AND.
func (r *Relation[P, C]) writeScope(b *Builder, cfg *loadConfig) {
	var ps []Predicate
	if r.Discriminator != nil {
		ps = append(ps, binOp{qcol{r.Child.Qualifier(), r.Discriminator.Column}, " = ", argExpr{r.Discriminator.Value}})
	}
	if m := r.ChildMeta; m.SoftDelete >= 0 && !cfg.withDeleted {
		ps = append(ps, softDeletePred(m, r.Child.Qualifier(), cfg.onlyDeleted))
	}
	ps = append(ps, cfg.where...)
	for _, p := range ps {
		b.WriteString(" AND ")
		p.AppendSQL(b)
	}
}

// writeOrder writes user order items, then the relation key columns, then the child primary key.
func (r *Relation[P, C]) writeOrder(b *Builder, cfg *loadConfig) {
	first := true
	sep := func() {
		if !first {
			b.WriteString(", ")
		}
		first = false
	}
	for _, o := range cfg.order {
		sep()
		o.order().appendSQL(b)
	}
	for _, c := range r.ChildCols {
		sep()
		b.QualifiedIdent(r.Child.Qualifier(), c)
	}
	for _, c := range r.ChildPK {
		sep()
		b.QualifiedIdent(r.Child.Qualifier(), c)
	}
}

// countPredicateArgs returns how many bind parameters the predicates render.
func countPredicateArgs(d *dialect.Dialect, ps []Predicate) (int, error) {
	if len(ps) == 0 {
		return 0, nil
	}
	c, err := compile(d, func(b *Builder) {
		for _, p := range ps {
			p.AppendSQL(b)
		}
	})
	if err != nil {
		return 0, err
	}
	return len(c.Args), nil
}

// scanChild scans the child's mapped columns in ChildMeta order.
func scanChild[P, C any](r *Relation[P, C], rows Rows, c *C) error {
	dest := make([]any, len(r.ChildMeta.Columns))
	for i := range dest {
		dest[i] = r.ChildMeta.Ptr(c, i)
	}
	return rows.Scan(dest...)
}

// hasNilPart reports whether any key component is NULL. A composite key with a
// NULL component never matches a row, so it is treated as no key. NULL follows
// keyString: an untyped nil, a nil pointer, or a driver.Valuer whose Value is
// nil (sql.NullInt64{} and friends).
func hasNilPart(vals []any) bool {
	for _, v := range vals {
		if isNullKeyPart(v) {
			return true
		}
	}
	return false
}

// isNullKeyPart reports whether v is a NULL key component in keyString's sense.
func isNullKeyPart(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case driver.Valuer:
		if rv := reflect.ValueOf(x); rv.Kind() == reflect.Pointer && rv.IsNil() {
			return true
		}
		vv, err := x.Value()
		return err == nil && vv == nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		return rv.IsNil() || isNullKeyPart(rv.Elem().Interface())
	}
	return false
}
