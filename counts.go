// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/chunk"
	"github.com/azrtydxb/momo/internal/keyenc"
)

// LoadCounts counts the children of every parent in parents with one grouped
// statement per bind-limit chunk, instead of one Relation.Count call per
// parent. The result has one entry per parent, in parents order; a parent with
// no children, or with a NULL key, counts zero. Parents sharing a key share the
// count.
//
//	counts, err := momo.LoadCounts(ctx, db, docs, document.Comments,
//		momo.LoadWhere[models.Comment](comment.Approved.Eq(true)))
//
// It supports has-one, has-many and many-to-many relations, including
// composite keys and polymorphic (Discriminator) relations; belongs-to fails
// with ErrUnsupported, as Count does. Load options narrow what is counted:
// LoadWhere filters the children (it may reference the child table), and
// soft-deleted children are excluded unless LoadWithDeleted or LoadOnlyDeleted
// says otherwise, exactly like Load. LoadOrderBy has no effect, and
// LoadPerParentLimit fails with ErrValidation.
//
// Many-to-many counts join-table rows, as Count does: the join to the far
// table is added only when the child has a soft-delete column or a filter
// applies. Statements are SELECT key, COUNT(*) ... GROUP BY key.
func LoadCounts[P, C any](ctx context.Context, q Querier, parents []P, rel Relation[P, C], opts ...LoadOption[C]) ([]int64, error) {
	var cfg loadConfig
	for _, o := range opts {
		o(&cfg)
	}
	if err := rel.validate(); err != nil {
		return nil, err
	}
	if rel.Kind == BelongsTo {
		return nil, rel.notLinkable("count")
	}
	if cfg.perParentLimit != 0 {
		return nil, &Error{Kind: ErrValidation, Op: "load", Table: rel.Child.Name,
			Err: errors.New(rel.Name + ": LoadPerParentLimit does not apply to LoadCounts")}
	}
	out := make([]int64, len(parents))
	groups := map[string][]int{}
	var keys [][]any
	for i := range parents {
		vals, ok := rel.ParentKey(&parents[i])
		if !ok || hasNilPart(vals) {
			continue
		}
		k := keyenc.String(vals)
		if _, seen := groups[k]; !seen {
			keys = append(keys, vals)
		}
		groups[k] = append(groups[k], i)
	}
	qs, err := rel.countQueries(q.Dialect(), keys, &cfg)
	if err != nil {
		return nil, err
	}
	if len(qs) == 0 {
		return out, nil
	}
	ev := &RelationEvent{Parent: rel.Parent.Name, Name: rel.Name, Kind: rel.Kind, Parents: len(parents), Queries: len(qs)}
	ctx, end := beginRelation(ctx, q.conn().db, ev)
	err = rel.runCounts(ctx, q, qs, groups, out, ev)
	end(err)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// runCounts runs the count statements one at a time and adds each group's
// count to the parents that share the key.
func (r Relation[P, C]) runCounts(ctx context.Context, q Querier, qs []Compiled, groups map[string][]int, out []int64, ev *RelationEvent) error {
	rel := r.Parent.Name + "." + r.Name
	table := r.Child.Name
	if r.Through != nil {
		table = r.Through.Table.Name // the counted table, as Count labels it
	}
	nk := len(r.ParentCols)
	for _, c := range qs {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := q.conn().query(ctx, "select", table, rel, c)
		if err != nil {
			return err
		}
		err = func() error {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				key := make([]any, nk)
				dest := make([]any, nk+1)
				for i := range key {
					dest[i] = &key[i]
				}
				var n int64
				dest[nk] = &n
				if err := rows.Scan(dest...); err != nil {
					return q.conn().wrapErr("load", r.Child.Name, err)
				}
				for _, i := range groups[keyenc.String(key)] {
					out[i] += n
				}
				ev.Children += n
			}
			return q.conn().wrapErr("load", r.Child.Name, rows.Err())
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

// countQueries builds one grouped statement per bind-limit chunk of keys.
func (r Relation[P, C]) countQueries(d *dialect.Dialect, keys [][]any, cfg *loadConfig) ([]Compiled, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	scopeArgs, err := countPredicateArgs(d, cfg.where)
	if err != nil {
		return nil, err
	}
	if r.Discriminator != nil {
		scopeArgs++
	}
	ncols := len(r.ParentCols)
	if d.MaxParams > 0 && scopeArgs+ncols > d.MaxParams {
		return nil, &Error{Kind: ErrValidation, Op: "load", Table: r.Child.Name,
			Err: fmt.Errorf("%s: scope binds %d arguments, leaving no room for a key under the %d-parameter limit", r.Name, scopeArgs, d.MaxParams)}
	}
	budget := chunk.KeysPerStatement(d, ncols, 16+scopeArgs)
	var out []Compiled
	for start := 0; start < len(keys); start += budget {
		part := keys[start:min(start+budget, len(keys))]
		c, err := compile(d, func(b *Builder) { r.writeCount(b, part, cfg) })
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// writeCount renders the grouped count for one chunk:
//
//	SELECT t.key_cols..., COUNT(*) FROM t [INNER JOIN child ON ...]
//	WHERE (t.key_cols) IN (keys) [AND scope] GROUP BY t.key_cols
//
// t is the child table, or the join table for many-to-many.
func (r Relation[P, C]) writeCount(b *Builder, chunk [][]any, cfg *loadConfig) {
	tr, cols := r.Child, r.ChildCols
	joined := true
	if r.Through != nil {
		tr, cols = r.Through.Table, r.Through.ParentCols
		soft := r.ChildMeta != nil && r.ChildMeta.SoftDelete >= 0 && !cfg.withDeleted
		joined = soft || len(cfg.where) > 0
	}
	b.WriteString("SELECT ")
	for _, c := range cols {
		b.QualifiedIdent(tr.Qualifier(), c)
		b.WriteString(", ")
	}
	b.WriteString("COUNT(*) FROM ")
	tr.appendFrom(b)
	if r.Through != nil && joined {
		b.WriteString(" INNER JOIN ")
		r.Child.appendFrom(b)
		b.WriteString(" ON ")
		for i, jc := range r.Through.ChildCols {
			if i > 0 {
				b.WriteString(" AND ")
			}
			b.QualifiedIdent(tr.Qualifier(), jc)
			b.WriteString(" = ")
			b.QualifiedIdent(r.Child.Qualifier(), r.ChildCols[i])
		}
	}
	b.WriteString(" WHERE ")
	inTuple{table: tr.Qualifier(), cols: cols, keys: chunk, flags: r.ownerFlags()}.AppendSQL(b)
	if joined {
		r.writeScope(b, cfg)
	}
	b.WriteString(" GROUP BY ")
	for i, c := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.QualifiedIdent(tr.Qualifier(), c)
	}
}
