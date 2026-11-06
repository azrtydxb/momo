// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
)

// Graph writes are explicit and opt-in. CreateGraph writes a parent and the
// children of the relations the caller lists, FindOrCreate inserts a row unless
// its key exists, and UpsertByPK inserts or updates by primary key. Nothing
// here saves a relation the caller did not name.
//
// Every row goes through the ordinary insert path (InsertQuery), so field
// hooks, automatic timestamps, Go-side defaults and validation run exactly as
// for a plain Insert, and generated columns are read back with RETURNING.

// Association is one relation listed for CreateGraph. It is produced only by
// Relation.Include, so unlisted relations are never written.
type Association[P any] interface {
	before(ctx context.Context, tx *Tx, p *P) error
	after(ctx context.Context, tx *Tx, p *P) error
}

type includedRel[P, C any] struct{ r Relation[P, C] }

// Include lists the relation for CreateGraph: the children held in the parent's
// field are inserted in the same transaction. Only the listed relation is
// written; the children's own relations are not (a graph is one level deep), so
// a self-referencing relation or a cycle in memory cannot recurse.
func (r Relation[P, C]) Include() Association[P] { return includedRel[P, C]{r} }

// graphErr builds a validation error for a graph write.
func graphErr(table, msg string) error {
	return &Error{Kind: ErrValidation, Op: "create graph", Table: table, Err: errors.New(msg)}
}

// before runs for BelongsTo: the child is inserted first and its key written into
// the parent, which is inserted after it.
func (x includedRel[P, C]) before(ctx context.Context, tx *Tx, p *P) error {
	r := x.r
	if r.Kind != BelongsTo {
		return nil
	}
	c := r.GetOne(p)
	if c == nil {
		return nil
	}
	if any(c) == any(p) {
		return graphErr(r.Child.Name, r.Name+": the parent lists itself as its own child (cycle)")
	}
	if err := insertModel(ctx, tx, r.ChildMeta, c); err != nil {
		return err
	}
	key, ok := r.ChildKey(c)
	if !ok {
		return graphErr(r.Child.Name, r.Name+": child key is NULL")
	}
	r.SetParentKey(p, key)
	return nil
}

// after runs for HasOne, HasMany and ManyToMany once the parent has its identity.
func (x includedRel[P, C]) after(ctx context.Context, tx *Tx, p *P) error {
	r := x.r
	switch r.Kind {
	case HasOne:
		if c := r.GetOne(p); c != nil {
			return r.insertChildren(ctx, tx, p, []*C{c})
		}
	case HasMany:
		cs := r.GetMany(p)
		ptrs := make([]*C, len(cs))
		for i := range cs {
			ptrs[i] = &cs[i]
		}
		return r.insertChildren(ctx, tx, p, ptrs)
	case ManyToMany:
		cs := r.GetMany(p)
		ptrs := make([]*C, len(cs))
		for i := range cs {
			ptrs[i] = &cs[i]
			if any(ptrs[i]) == any(p) {
				return graphErr(r.Child.Name, r.Name+": the parent lists itself as its own child (cycle)")
			}
			if err := insertModel(ctx, tx, r.ChildMeta, ptrs[i]); err != nil {
				return err
			}
		}
		return r.Link(ctx, tx, p, ptrs...)
	}
	return nil
}

// insertChildren writes the parent's key (and a polymorphic discriminator) into
// each child, then inserts it.
func (r Relation[P, C]) insertChildren(ctx context.Context, tx *Tx, p *P, cs []*C) error {
	key, ok := r.ParentKey(p)
	if !ok {
		return graphErr(r.Child.Name, r.Name+": parent key is NULL")
	}
	for _, c := range cs {
		if any(c) == any(p) {
			return graphErr(r.Child.Name, r.Name+": the parent lists itself as its own child (cycle)")
		}
		r.SetChildKey(c, key)
		if err := insertModel(ctx, tx, r.ChildMeta, c); err != nil {
			return err
		}
	}
	return nil
}

// CreateGraph inserts p and the children of the listed relations in one
// transaction. Listed BelongsTo children are written before p and their key is
// copied into p; the other listed children are written after p, with p's key
// (read back with RETURNING when it is generated) copied into each child and,
// for a polymorphic relation, the discriminator set. ManyToMany children are
// inserted and then linked through the join table. Relations are processed in
// the order listed. Unlisted relations are not written, and the children's own
// relations are not followed.
//
// On a *DB the graph runs in a new transaction; on a *Tx it runs in a
// savepoint, so a failure undoes the whole graph and leaves the caller's
// transaction usable (a dialect without savepoints returns ErrUnsupported).
// A model that appears as its own child fails with ErrValidation.
//
// CreateGraph needs a transaction, so a *DB pinned with OnConn fails with
// ErrUnsupported (Op "begin"); run it on the pool or inside a *Tx. After a
// failure the database is unchanged, but p and the children keep the generated
// keys and foreign keys already written into them; do not reuse them as if
// they were unsaved.
func CreateGraph[P any, PP Modeler[P]](ctx context.Context, q Querier, p *P, includes ...Association[P]) error {
	if p == nil {
		return graphErr("", "CreateGraph(nil)")
	}
	fn := func(tx *Tx) error {
		for _, inc := range includes {
			if err := inc.before(ctx, tx, p); err != nil {
				return err
			}
		}
		if err := insertModel(ctx, tx, metaOf[P, PP](), p); err != nil {
			return err
		}
		for _, inc := range includes {
			if err := inc.after(ctx, tx, p); err != nil {
				return err
			}
		}
		return nil
	}
	return inTx(ctx, q, fn)
}

// insertModel inserts m through the ordinary insert path and reads generated
// columns back into it when the dialect has RETURNING.
func insertModel[T any](ctx context.Context, q Querier, meta *Meta[T], m *T) error {
	_, err := runInsert(ctx, q, InsertQuery[T]{q: q, meta: meta}.Model(m), m)
	return err
}

// runInsert executes iq (which holds exactly the model m). With RETURNING it
// scans the returned row into m and reports whether a row was inserted; an
// ON CONFLICT DO NOTHING that hit a conflict returns no row, so it reports
// false. A model with an identity column needs RETURNING to read it back.
func runInsert[T any](ctx context.Context, q Querier, iq InsertQuery[T], m *T) (bool, error) {
	if !q.Dialect().Returning {
		if iq.meta.Identity >= 0 {
			return false, &Error{Kind: ErrUnsupported, Op: "insert", Table: iq.meta.Table, Err: errors.New("identity read-back needs RETURNING")}
		}
		res, err := iq.Exec(ctx)
		return res.RowsAffected > 0, err
	}
	rows, err := iq.ReturningAll().Rows(ctx)
	if err != nil {
		return false, err
	}
	inserted := false
	if rows.Next() {
		if err := rows.ScanModel(m); err != nil {
			_ = rows.Close()
			return false, err
		}
		inserted = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	return inserted, rows.Close()
}

// FindOrCreate inserts m unless a row with the same key columns exists; then m
// holds the stored row. The statement is INSERT ... ON CONFLICT (key) DO
// NOTHING, so the unique constraint on key (which must exist) decides the
// winner and concurrent callers never create duplicates: the losers re-select
// the winner's row. created reports whether this call inserted. When the row
// exists, all of m's mapped columns are overwritten by the stored values; its
// relation fields are reset (the row is loaded as a plain model, soft-deleted
// rows included, since they still hold the key).
//
// Under READ COMMITTED the conflicting insert waits for a concurrent
// transaction and the re-select then sees its row. Under REPEATABLE READ or
// SERIALIZABLE the re-select can miss a row committed after the snapshot and
// returns ErrNotFound. Dialects without ON CONFLICT return ErrUnsupported.
func FindOrCreate[T any, PT Modeler[T]](ctx context.Context, q Querier, key []ColumnRef, m *T) (created bool, err error) {
	meta := metaOf[T, PT]()
	if m == nil {
		return false, &Error{Kind: ErrValidation, Op: "find or create", Table: meta.Table, Err: errors.New("FindOrCreate(nil)")}
	}
	if len(key) == 0 {
		return false, &Error{Kind: ErrValidation, Op: "find or create", Table: meta.Table, Err: errors.New("key needs at least one column")}
	}
	preds := make([]Predicate, len(key))
	for i, k := range key {
		ci := columnIndexOf(meta, k.ColumnName())
		if ci < 0 {
			return false, &Error{Kind: ErrValidation, Op: "find or create", Table: meta.Table, Column: k.ColumnName(), Err: errors.New("key column is not mapped by the model")}
		}
		preds[i] = binOp{k, " = ", argExpr{meta.Value(m, ci)}}
	}
	iq := InsertQuery[T]{q: q, meta: meta}.Model(m).OnConflict(key...).DoNothing()
	created, err = runInsert(ctx, q, iq, m)
	if err != nil || created {
		return created, err
	}
	row, err := Select[T, PT](q).WithDeleted().Where(preds...).One(ctx)
	if err != nil {
		return false, err
	}
	*m = row
	return false, nil
}

// UpsertByPK inserts m, or updates the row with the same primary key:
// INSERT ... ON CONFLICT (pk) DO UPDATE SET col = EXCLUDED.col for every column
// an update may change (the primary key, version, soft-delete and created-at
// columns are never overwritten; see Upsert for the exact rules and for an
// upsert on another conflict target). Generated columns are read back into m.
// It is the one-call form of Upsert(q).Model(m).
func UpsertByPK[T any, PT Modeler[T]](ctx context.Context, q Querier, m *T) error {
	meta := metaOf[T, PT]()
	if m == nil {
		return &Error{Kind: ErrValidation, Op: "upsert", Table: meta.Table, Err: errors.New("UpsertByPK(nil)")}
	}
	_, err := runInsert(ctx, q, Upsert[T, PT](q).Model(m), m)
	return err
}

// columnIndexOf returns the index of the column named name, or -1.
func columnIndexOf[T any](m *Meta[T], name string) int {
	for i, c := range m.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}
