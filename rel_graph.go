// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// Graph writes are explicit and opt-in. CreateGraph writes a parent and the
// children of the relations the caller lists, FindOrCreate inserts a row unless
// its key exists, and UpsertByPK inserts or updates by primary key. Nothing
// here saves a relation the caller did not name.
//
// Every row goes through the ordinary insert path (InsertQuery), so field
// hooks, automatic timestamps, Go-side defaults and validation run exactly as
// for a plain Insert, and generated columns are read back with RETURNING (on
// MariaDB with INSERT ... RETURNING, on MySQL with LAST_INSERT_ID, see
// CreateGraph).

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
	return newError(ErrValidation, "create graph", table, errors.New(msg))
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
		c := r.GetOne(p)
		if c == nil {
			return nil
		}
		key, err := r.childKey(p)
		if err != nil {
			return err
		}
		if err := r.bindChild(p, c, key); err != nil {
			return err
		}
		return insertModel(ctx, tx, r.ChildMeta, c)
	case HasMany:
		cs := r.GetMany(p)
		key, err := r.childKey(p)
		if err != nil {
			return err
		}
		for i := range cs {
			if err := r.bindChild(p, &cs[i], key); err != nil {
				return err
			}
		}
		return insertModels(ctx, tx, r.ChildMeta, cs)
	case ManyToMany:
		cs := r.GetMany(p)
		ptrs := make([]*C, len(cs))
		for i := range cs {
			ptrs[i] = &cs[i]
			if any(ptrs[i]) == any(p) {
				return graphErr(r.Child.Name, r.Name+": the parent lists itself as its own child (cycle)")
			}
		}
		if err := insertModels(ctx, tx, r.ChildMeta, cs); err != nil {
			return err
		}
		return r.Link(ctx, tx, p, ptrs...)
	}
	return nil
}

// childKey returns the parent's key, which HasOne and HasMany copy into their
// children.
func (r Relation[P, C]) childKey(p *P) ([]any, error) {
	key, ok := r.ParentKey(p)
	if !ok {
		return nil, graphErr(r.Child.Name, r.Name+": parent key is NULL")
	}
	return key, nil
}

// bindChild rejects a child that is the parent itself, then writes the
// parent's key (and a polymorphic discriminator) into it.
func (r Relation[P, C]) bindChild(p *P, c *C, key []any) error {
	if any(c) == any(p) {
		return graphErr(r.Child.Name, r.Name+": the parent lists itself as its own child (cycle)")
	}
	r.SetChildKey(c, key)
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
// The children of a HasMany or ManyToMany relation are inserted with the
// chunked bulk insert (InsertQuery.Models): one multi-row statement per
// bind-limit chunk instead of one statement per child, so 1000 children cost
// one statement, not 1000. A HasOne or BelongsTo child is a single insert.
//
// Every listed child is inserted as a new row, including a ManyToMany child
// that is already stored: it is not linked, it is inserted again and fails with
// ErrUniqueViolation (undoing the whole graph) when its primary key exists. To
// associate children that already exist, use Relation.Link (or insert them
// first and Link them) instead of listing the relation here.
//
// A bulk insert returns its rows in insertion order, and CreateGraph matches
// them to the children by position. If the database returns a different number
// of rows than were inserted (for example a BEFORE INSERT trigger skipped one),
// the graph fails with ErrValidation and is undone, rather than leaving the
// remaining children without their generated keys.
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
//
// Generated keys are read back with RETURNING on PostgreSQL and SQLite and with
// INSERT ... RETURNING on MariaDB. MySQL has no RETURNING, so the keys come
// from LAST_INSERT_ID, read after each statement: a has-many or many-to-many
// bulk is inserted one row per statement inside the graph's transaction (a
// savepoint on a Tx), because the keys of one multi-row INSERT are not
// guaranteed consecutive under innodb_autoinc_lock_mode 2, the MySQL 8
// default. Keys are never guessed. The statements share one clock read, so the
// children get the same automatic timestamps as a bulk insert. On MySQL a bulk
// whose children mix zero and explicit keys fails with ErrValidation, and a
// generated key on a non-integer field fails with ErrUnsupported.
func CreateGraph[P any, PP Modeler[P]](ctx context.Context, q Querier, p *P, includes ...Association[P]) error {
	if p == nil {
		return graphErr("", "CreateGraph(nil)")
	}
	if d := q.Dialect(); !d.Returning && !d.InsertReturning && !d.OnDuplicateKey {
		return &Error{Kind: ErrUnsupported, Op: "create graph", Table: metaOf[P, PP]().Table, Err: errors.New("CreateGraph needs RETURNING or LAST_INSERT_ID to read generated keys back")}
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
	return insertModelAt(ctx, q, meta, m, time.Time{})
}

// insertModelAt is insertModel with the statement clock fixed to clock (the
// zero time reads it per statement), so several one-row inserts of a graph
// share the automatic timestamps of one bulk insert.
func insertModelAt[T any](ctx context.Context, q Querier, meta *Meta[T], m *T, clock time.Time) error {
	iq := InsertQuery[T]{q: q, meta: meta, clock: clock}.Model(m)
	_, err := runInsert(ctx, q, iq, m)
	return err
}

// insertModels inserts ms with the chunked bulk insert and reads generated
// columns back into each element, in order. It needs RETURNING, or on MySQL
// LAST_INSERT_ID (CreateGraph checks that before it opens a transaction).
func insertModels[T any](ctx context.Context, q Querier, meta *Meta[T], ms []T) error {
	switch len(ms) {
	case 0:
		return nil
	case 1:
		return insertModel(ctx, q, meta, &ms[0])
	}
	if d := q.Dialect(); !d.Returning && !d.InsertReturning {
		return insertModelsLastID(ctx, q, meta, ms)
	}
	rows, err := InsertQuery[T]{q: q, meta: meta}.Models(ms).ReturningAll().Rows(ctx)
	if err != nil {
		return err
	}
	n := 0
	for rows.Next() {
		if n >= len(ms) {
			_ = rows.Close()
			return newError(ErrValidation, "insert", meta.Table, errors.New("bulk insert returned more rows than were inserted"))
		}
		if err := rows.ScanModel(&ms[n]); err != nil {
			_ = rows.Close()
			return err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if n != len(ms) {
		// A trigger that skips a row would leave the tail children without
		// their generated keys; do not report success.
		return &Error{Kind: ErrValidation, Op: "insert", Table: meta.Table,
			Err: fmt.Errorf("bulk insert returned %d rows for %d inserted models", n, len(ms))}
	}
	return nil
}

// insertModelsLastID is insertModels for MySQL, which has no RETURNING: it
// reads the generated keys through LAST_INSERT_ID. A model without a generated
// identity key, or whose key is generated Go-side, or whose rows all carry
// explicit keys, is a plain bulk insert. Otherwise every key must be zero (a
// mix fails with ErrValidation) and the rows are inserted one per statement,
// each followed by its own LAST_INSERT_ID, so every key is exact whatever
// innodb_autoinc_lock_mode is (with mode 2 the keys of one multi-row statement
// are not guaranteed consecutive). The statements share one clock read, so the
// rows get the same automatic timestamps as one bulk insert would give.
func insertModelsLastID[T any](ctx context.Context, q Querier, meta *Meta[T], ms []T) error {
	bulk := InsertQuery[T]{q: q, meta: meta}
	id := meta.Identity
	if id < 0 || id >= len(meta.Columns) || meta.Columns[id].Flags&FlagIdentity == 0 || bulk.isIDGenColumn(id) {
		_, err := bulk.Models(ms).Exec(ctx)
		return err
	}
	zero := 0
	for i := range ms {
		if reflect.ValueOf(meta.Value(&ms[i], id)).IsZero() {
			zero++
		}
	}
	switch zero {
	case 0:
		_, err := bulk.Models(ms).Exec(ctx) // explicit keys are written and kept
		return err
	case len(ms):
	default:
		return &Error{Kind: ErrValidation, Op: "insert", Table: meta.Table, Column: meta.Columns[id].Name,
			Err: errors.New("models mix explicit and zero generated keys; MySQL cannot read the generated keys of such a batch back: split them into separate calls")}
	}
	clock := q.conn().now()
	for i := range ms {
		if err := insertModelAt(ctx, q, meta, &ms[i], clock); err != nil {
			return err
		}
	}
	return nil
}

// runInsert executes iq (which holds exactly the model m). With RETURNING it
// scans the returned row into m and reports whether a row was inserted; an
// ON CONFLICT DO NOTHING that hit a conflict returns no row, so it reports
// false. A model with an identity column needs RETURNING to read it back
// (INSERT ... RETURNING on MariaDB). MySQL has none: a plain insert reads the
// key with LAST_INSERT_ID, and an insert with a conflict clause cannot.
func runInsert[T any](ctx context.Context, q Querier, iq InsertQuery[T], m *T) (bool, error) {
	if d := q.Dialect(); !d.Returning && !d.InsertReturning {
		if iq.meta.Identity >= 0 && (!d.OnDuplicateKey || iq.i.upsert || iq.i.conflict != conflictNone) {
			return false, newError(ErrUnsupported, "insert", iq.meta.Table, errors.New("identity read-back needs RETURNING; MySQL reads it only for a plain insert (LAST_INSERT_ID), not for one with a conflict clause"))
		}
		if iq.meta.Identity >= 0 && iq.leavesKeyToServer() && !identityIsInteger(iq.meta) {
			return false, newError(ErrUnsupported, "insert", iq.meta.Table, errors.New("the generated identity key is not an integer field, and MySQL reads it only as an integer (LAST_INSERT_ID)"))
		}
		res, err := iq.Exec(ctx)
		if err == nil && iq.meta.Identity >= 0 && iq.leavesKeyToServer() && res.InsertID == 0 {
			return false, newError(ErrUnsupported, "insert", iq.meta.Table, errors.New("the driver did not report the generated key (it must implement InsertIDExecer)"))
		}
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
		// A single-model INSERT returns at most one row. Read to the end
		// anyway: the rows of a write on a WithImplicitTx handle must be read
		// completely, or the write is rolled back.
		for rows.Next() {
		}
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
// relation fields are reset (the row is loaded as a plain model).
//
// A soft-deleted row does not count as found: FindOrCreate never returns or
// restores a row the application deleted. The insert is attempted. When the
// unique constraint on key covers soft-deleted rows (a plain unique constraint
// or index, the case FindOrCreate supports), it still holds the deleted row's
// key, the insert conflicts and the call fails with ErrUniqueViolation (Op
// "find or create") instead of returning the deleted row: a soft-deleted match
// is an error, not a surprise resurrection. To reuse the old row, restore it
// first (update the soft-delete marker back) or pick a new key. A partial
// unique index (WHERE deleted_at IS NULL) would let the new row in, but
// FindOrCreate cannot target it: ON CONFLICT (key) without the index predicate
// does not match a partial index and the database rejects the statement. For a
// key that soft-deleted rows may release, use
// Insert(...).OnConflict(key).Where(indexPredicate).DoNothing() and re-select
// yourself. A model without a
// soft-delete column is unaffected.
//
// Under READ COMMITTED the conflicting insert waits for a concurrent
// transaction and the re-select then sees its row. Under REPEATABLE READ or
// SERIALIZABLE the re-select can miss a row committed after the snapshot and
// returns ErrNotFound.
//
// MySQL and MariaDB have no ON CONFLICT DO NOTHING that reports whether a row
// was inserted: with a no-op ON DUPLICATE KEY UPDATE the driver's
// clientFoundRows makes an inserted row and an untouched existing row both
// report one affected row (and MariaDB's RETURNING returns the row either
// way), so created could not be told. FindOrCreate therefore runs in a
// transaction (a savepoint on a Tx) as a plain INSERT, which on MariaDB reads
// the generated key with INSERT ... RETURNING and on MySQL with LAST_INSERT_ID;
// when the insert fails with a unique violation, it re-selects the row by key
// with a shared-lock read (FOR SHARE, LOCK IN SHARE MODE on MariaDB), which
// reads the latest committed row at any isolation level, so the REPEATABLE
// READ miss above does not occur. Shared locks do not conflict with each
// other, so callers that lost the same race do not block one another there; if
// the first inserter rolls back instead of committing, the waiting callers can
// still deadlock (MySQL error 1213, ErrSerialization) and one retries. On a
// caller's Tx the savepoint release does not free the locks: InnoDB keeps the
// locks taken after a savepoint until the outer transaction ends. The cost is
// one extra round trip (the failed insert, then the select) when the row
// exists. A unique violation on another key than key still fails with
// ErrUniqueViolation. A DB pinned with OnConn cannot open the transaction and
// fails with ErrUnsupported.
func FindOrCreate[T any, PT Modeler[T]](ctx context.Context, q Querier, key []ColumnRef, m *T) (created bool, err error) {
	meta := metaOf[T, PT]()
	if m == nil {
		return false, newError(ErrValidation, "find or create", meta.Table, errors.New("FindOrCreate(nil)"))
	}
	if len(key) == 0 {
		return false, newError(ErrValidation, "find or create", meta.Table, errors.New("key needs at least one column"))
	}
	preds := make([]Predicate, len(key))
	for i, k := range key {
		ci := columnIndexOf(meta, k.ColumnName())
		if ci < 0 {
			return false, &Error{Kind: ErrValidation, Op: "find or create", Table: meta.Table, Column: k.ColumnName(), Err: errors.New("key column is not mapped by the model")}
		}
		preds[i] = binOp{k, " = ", argExpr{v: meta.Value(m, ci), flags: meta.Columns[ci].Flags}}
	}
	d := q.Dialect()
	if d.OnDuplicateKey && !d.OnConflict {
		return findOrCreateMySQL[T, PT](ctx, q, meta, preds, m)
	}
	if !d.OnConflict || !d.Returning {
		return false, newError(ErrUnsupported, "find or create", meta.Table, errors.New("FindOrCreate needs ON CONFLICT and RETURNING"))
	}
	iq := InsertQuery[T]{q: q, meta: meta}.Model(m).OnConflict(key...).DoNothing()
	created, err = runInsert(ctx, q, iq, m)
	if err != nil || created {
		return created, err
	}
	row, err := Select[T, PT](q).Where(preds...).One(ctx)
	if errors.Is(err, ErrNotFound) && meta.SoftDelete >= 0 {
		// The conflict came from a soft-deleted row that still holds the key.
		n, cerr := Select[T, PT](q).WithDeleted().Where(preds...).Count(ctx)
		if cerr != nil {
			return false, cerr
		}
		if n > 0 {
			return false, &Error{Kind: ErrUniqueViolation, Op: "find or create", Table: meta.Table,
				Err: errors.New("the key is held by a soft-deleted row; FindOrCreate does not return or restore deleted rows")}
		}
	}
	if err != nil {
		return false, err
	}
	*m = row
	return false, nil
}

// findOrCreateMySQL is FindOrCreate for MySQL and MariaDB (see FindOrCreate):
// a plain insert in an atomic scope, then, on a unique violation, a locking
// select by key. preds select the row by key from m.
func findOrCreateMySQL[T any, PT Modeler[T]](ctx context.Context, q Querier, meta *Meta[T], preds []Predicate, m *T) (created bool, err error) {
	err = runAtomic(ctx, q, "find or create", meta.Table, func(sq Querier) error {
		var ierr error
		created, ierr = runInsert(ctx, sq, InsertQuery[T]{q: sq, meta: meta}.Model(m), m)
		if ierr == nil {
			return nil
		}
		created = false
		if !errors.Is(ierr, ErrUniqueViolation) {
			return ierr
		}
		row, serr := Select[T, PT](sq).Where(preds...).ForShare().One(ctx)
		switch {
		case serr == nil:
			*m = row
			return nil
		case !errors.Is(serr, ErrNotFound):
			return serr
		}
		if meta.SoftDelete >= 0 {
			n, cerr := Select[T, PT](sq).WithDeleted().Where(preds...).Count(ctx)
			if cerr != nil {
				return cerr
			}
			if n > 0 {
				return &Error{Kind: ErrUniqueViolation, Op: "find or create", Table: meta.Table,
					Err: errors.New("the key is held by a soft-deleted row; FindOrCreate does not return or restore deleted rows")}
			}
		}
		return ierr // the violation was on another unique key than the one FindOrCreate was given
	})
	if err != nil {
		return false, err
	}
	return created, nil
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
		return newError(ErrValidation, "upsert", meta.Table, errors.New("UpsertByPK(nil)"))
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
