// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
)

// Association helpers write the link between a parent and its children
// explicitly. They issue the documented statements and nothing else; none of
// them touches the in-memory values passed in.
//
// The helpers write directly with SQL: they do not run model hooks (BeforeUpdate
// and friends), do not run field hooks, and do not touch an UpdatedAt column.
// Use the model's own update path when those must fire.
//
// Statement contract (one statement per chunk; a chunk holds as many keys as
// fit under the dialect's bind limit, see WithMaxParams, so a small call is one
// statement):
//
//   - HasOne/HasMany Link: UPDATE child SET fk = ? WHERE pk IN (...). For a
//     polymorphic relation the discriminator column is set in the same UPDATE.
//   - HasOne/HasMany Unlink: UPDATE child SET fk = NULL WHERE fk = ? AND pk IN
//     (...) (and the discriminator, for a polymorphic relation). The foreign
//     key columns must be nullable; a NOT NULL violation is reported as
//     ErrNotNullViolation with a message that says so.
//   - ManyToMany Link: INSERT INTO join table (parent cols, child cols) VALUES
//     (...), ... that skips pairs which already exist (ON CONFLICT DO NOTHING;
//     MySQL: ON DUPLICATE KEY UPDATE with a no-op assignment of the first
//     column to itself). This needs a primary or unique key over the join
//     columns: without one, nothing detects an existing pair and Link inserts a
//     duplicate row.
//   - ManyToMany Unlink: DELETE FROM join table WHERE parent cols = ? AND
//     (child cols) IN (...).
//   - Replace: one transaction. With no children it clears the whole set in one
//     statement. Otherwise it SELECTs the current child keys, diffs them in Go
//     against the requested set (NOT IN cannot be chunked), unlinks only the
//     removed keys and links only the added keys, in chunks. Kept children are
//     not rewritten. On a *DB the helper opens the transaction; on a *Tx it runs
//     in a savepoint.
//     Before reading, it locks the parent row with SELECT 1 FROM parent WHERE
//     key = ? FOR UPDATE on dialects with row locks (PostgreSQL, MySQL), so
//     concurrent Replace calls on one parent are serialised; SQLite serialises
//     writers and skips it. A parent row that does not exist locks nothing, so
//     concurrent Replace calls on a missing parent are not serialised. Link and
//     Unlink take no parent lock and can interleave with a concurrent Replace.
//   - Count: one SELECT COUNT(*) over the related children (soft-deleted
//     children are not counted).
//   - On BelongsTo every helper returns ErrUnsupported: set the foreign key on
//     the parent instead.
//
// Link, Unlink and Replace return ErrValidation when the parent key (or, for
// ManyToMany, a child key) is NULL. Link and Unlink with no children are
// no-ops that issue no statement. When Link or Unlink needs more than one chunk
// and q is a *DB, the chunks run in one transaction, so the call is atomic. On a
// *Tx the chunks run in the caller's transaction without a savepoint: if a later
// chunk fails, earlier chunks stay applied until the caller rolls back.

// Link associates the listed children with p: HasOne and HasMany set the
// children's foreign key (UPDATE), ManyToMany inserts the join rows (INSERT,
// existing pairs are kept; the join columns need a unique or primary key). Sets
// larger than the bind limit are split into one statement per chunk. It writes
// directly: no model hooks, field hooks or UpdatedAt. BelongsTo returns
// ErrUnsupported.
func (r Relation[P, C]) Link(ctx context.Context, q Querier, p *P, cs ...*C) error {
	switch r.Kind {
	case HasOne, HasMany:
		pkey, keys, err := r.childPKs(p, cs)
		if err != nil || len(keys) == 0 {
			return err
		}
		return r.runChunks(ctx, q, keys, r.chunkSize(q, len(r.ChildPK), len(r.ChildCols)+r.discArgs()),
			func(x Querier, ks [][]any) error { return r.updateChunk(ctx, x, pkey, ks, linkSet) })
	case ManyToMany:
		pkey, keys, err := r.joinKeys(p, cs)
		if err != nil || len(keys) == 0 {
			return err
		}
		return r.runChunks(ctx, q, keys, r.chunkSize(q, len(r.Through.ParentCols)+len(r.Through.ChildCols), 0),
			func(x Querier, ks [][]any) error { return r.insertJoinChunk(ctx, x, pkey, ks) })
	}
	return r.notLinkable("link")
}

// Unlink removes the association between p and the listed children: HasOne and
// HasMany clear the children's foreign key (UPDATE; the key columns must be
// nullable), ManyToMany deletes the join rows (DELETE). Children that are not
// associated with p are left alone. Sets larger than the bind limit are split
// into one statement per chunk. It writes directly: no model hooks, field hooks
// or UpdatedAt. BelongsTo returns ErrUnsupported.
func (r Relation[P, C]) Unlink(ctx context.Context, q Querier, p *P, cs ...*C) error {
	switch r.Kind {
	case HasOne, HasMany:
		pkey, keys, err := r.childPKs(p, cs)
		if err != nil || len(keys) == 0 {
			return err
		}
		return r.runChunks(ctx, q, keys, r.chunkSize(q, len(r.ChildPK), len(r.ChildCols)+r.discArgs()),
			func(x Querier, ks [][]any) error { return r.updateChunk(ctx, x, pkey, ks, linkClear) })
	case ManyToMany:
		pkey, keys, err := r.joinKeys(p, cs)
		if err != nil || len(keys) == 0 {
			return err
		}
		return r.runChunks(ctx, q, keys, r.chunkSize(q, len(r.Through.ChildCols), len(r.Through.ParentCols)),
			func(x Querier, ks [][]any) error { return r.deleteJoinChunk(ctx, x, pkey, ks) })
	}
	return r.notLinkable("unlink")
}

// Replace makes the listed children the complete set associated with p, in one
// transaction (a savepoint on a *Tx). It first locks the parent row (SELECT 1
// FROM parent WHERE key = ? FOR UPDATE) on dialects with row locks (PostgreSQL,
// MySQL), so concurrent Replace calls on the same parent run one after the
// other and each reads the set the previous one committed; without that, two
// callers under READ COMMITTED could produce unique violations, duplicate join
// rows or a mix of both sets. SQLite serialises writers and skips the lock. On a
// *Tx the lock is held until the caller's transaction ends, because the
// savepoint does not release it. The lock is a row lock, so a parent row that
// does not exist locks nothing: concurrent Replace calls on a missing parent
// are not serialised. Link and Unlink take no parent lock, so they can
// interleave with a concurrent Replace on the same parent. With no children it
// clears the whole set in one statement. Otherwise it reads the current child
// keys, diffs them in Go against cs, unlinks only the removed keys and links
// only the added keys, each in chunks that respect the bind limit, so a set of
// any size works and kept children are not rewritten. It writes directly: no
// model hooks, field hooks or UpdatedAt. HasOne/HasMany need a nullable
// foreign key. BelongsTo returns ErrUnsupported. Replace needs a transaction,
// so a *DB pinned with OnConn fails with ErrUnsupported (Op "begin"); run it on
// the pool or inside a *Tx. Link, Unlink and Count accept a pinned handle.
func (r Relation[P, C]) Replace(ctx context.Context, q Querier, p *P, cs ...*C) error {
	switch r.Kind {
	case HasOne, HasMany, ManyToMany:
	default:
		return r.notLinkable("replace")
	}
	// Validate everything before opening a transaction for a call that cannot succeed.
	var pkey []any
	var want [][]any
	var err error
	if r.Kind == ManyToMany {
		pkey, want, err = r.joinKeys(p, cs)
	} else {
		pkey, want, err = r.childPKs(p, cs)
	}
	if err != nil {
		return err
	}
	fn := func(tx *Tx) error { return r.replaceIn(ctx, tx, pkey, want) }
	return inTx(ctx, q, fn)
}

func (r Relation[P, C]) replaceIn(ctx context.Context, tx *Tx, pkey []any, want [][]any) error {
	m2m := r.Kind == ManyToMany
	if err := r.lockParent(ctx, tx, pkey); err != nil {
		return err
	}
	if len(want) == 0 { // clear the whole set: one statement, no SELECT
		if m2m {
			return r.deleteJoinChunk(ctx, tx, pkey, nil)
		}
		return r.updateChunk(ctx, tx, pkey, nil, linkClear)
	}
	have, err := r.currentKeys(ctx, tx, pkey)
	if err != nil {
		return err
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, k := range want {
		wantSet[keyString(k)] = struct{}{}
	}
	haveSet := make(map[string]struct{}, len(have))
	var removed, added [][]any
	for _, k := range have {
		s := keyString(k)
		if _, dup := haveSet[s]; dup {
			continue
		}
		haveSet[s] = struct{}{}
		if _, ok := wantSet[s]; !ok {
			removed = append(removed, k)
		}
	}
	seen := make(map[string]struct{}, len(want))
	for _, k := range want {
		s := keyString(k)
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		if _, ok := haveSet[s]; !ok {
			added = append(added, k)
		}
	}
	if m2m {
		if err := r.runChunks(ctx, tx, removed, r.chunkSize(tx, len(r.Through.ChildCols), len(r.Through.ParentCols)),
			func(x Querier, ks [][]any) error { return r.deleteJoinChunk(ctx, x, pkey, ks) }); err != nil {
			return err
		}
		return r.runChunks(ctx, tx, added, r.chunkSize(tx, len(r.Through.ParentCols)+len(r.Through.ChildCols), 0),
			func(x Querier, ks [][]any) error { return r.insertJoinChunk(ctx, x, pkey, ks) })
	}
	if err := r.runChunks(ctx, tx, removed, r.chunkSize(tx, len(r.ChildPK), len(r.ChildCols)+r.discArgs()),
		func(x Querier, ks [][]any) error { return r.updateChunk(ctx, x, pkey, ks, linkClear) }); err != nil {
		return err
	}
	return r.runChunks(ctx, tx, added, r.chunkSize(tx, len(r.ChildPK), len(r.ChildCols)+r.discArgs()),
		func(x Querier, ks [][]any) error { return r.updateChunk(ctx, x, pkey, ks, linkSet) })
}

// lockParent serialises concurrent Replace calls on one parent: it locks the
// parent row with SELECT 1 FROM parent WHERE key = ? FOR UPDATE, so a second
// Replace waits for the first to commit and then reads the committed key set.
// It runs on dialects whose Locking capability is set (PostgreSQL, MySQL); it is
// skipped on SQLite, which serialises writers. A parent row that does not exist
// locks nothing and is not an error.
func (r Relation[P, C]) lockParent(ctx context.Context, tx *Tx, pkey []any) error {
	if !tx.Dialect().Locking {
		return nil
	}
	tbl := r.Parent.Qualifier()
	c, err := compile(tx.Dialect(), func(b *Builder) {
		b.WriteString("SELECT 1 FROM ")
		r.Parent.appendFrom(b)
		b.WriteString(" WHERE ")
		inTuple{table: tbl, cols: r.ParentCols, keys: [][]any{pkey}, flags: keyFlags(r.ParentMeta, r.ParentCols)}.AppendSQL(b)
		b.WriteString(" FOR UPDATE")
	})
	if err != nil {
		return err
	}
	rows, err := tx.conn().query(ctx, "select", r.Parent.Name, "", c)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	err = rows.Err()
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	return err
}

// currentKeys reads the keys of the children currently associated with the
// parent key: the child columns of the join table for ManyToMany, the child
// primary key (scoped by the discriminator) for HasOne/HasMany. Soft-deleted
// children are included, as Replace clears them too.
func (r Relation[P, C]) currentKeys(ctx context.Context, q Querier, pkey []any) ([][]any, error) {
	tr, match, cols := r.Child, r.ChildCols, r.ChildPK
	if r.Through != nil {
		tr, match, cols = r.Through.Table, r.Through.ParentCols, r.Through.ChildCols
	}
	tbl := tr.Qualifier()
	c, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("SELECT ")
		for i, col := range cols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(tbl, col)
		}
		b.WriteString(" FROM ")
		tr.appendFrom(b)
		b.WriteString(" WHERE ")
		inTuple{table: tbl, cols: match, keys: [][]any{pkey}}.AppendSQL(b)
		if r.Through == nil && r.Discriminator != nil {
			b.WriteString(" AND ")
			b.QualifiedIdent(tbl, r.Discriminator.Column)
			b.WriteString(" = ")
			b.Arg(r.Discriminator.Value)
		}
	})
	if err != nil {
		return nil, err
	}
	rows, err := q.conn().query(ctx, "select", tr.Name, "", c)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out [][]any
	for rows.Next() {
		k := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range k {
			dest[i] = &k[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		if !hasNilPart(k) {
			out = append(out, k)
		}
	}
	return out, rows.Err()
}

// Count returns the number of children associated with p in one SELECT
// COUNT(*). A NULL parent key counts zero. For ManyToMany it counts join rows
// (joined to the far table only when that table has a soft-delete column, so
// soft-deleted children are not counted). BelongsTo returns ErrUnsupported.
func (r Relation[P, C]) Count(ctx context.Context, q Querier, p *P) (int64, error) {
	switch r.Kind {
	case HasOne, HasMany, ManyToMany:
	default:
		return 0, r.notLinkable("count")
	}
	key, ok := r.ParentKey(p)
	if !ok || hasNilPart(key) {
		return 0, nil
	}
	tr, cols := r.Child, r.ChildCols
	joined := true
	if r.Through != nil {
		tr, cols = r.Through.Table, r.Through.ParentCols
		joined = r.ChildMeta != nil && r.ChildMeta.SoftDelete >= 0
	}
	c, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("SELECT COUNT(*) FROM ")
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
		inTuple{table: tr.Qualifier(), cols: cols, keys: [][]any{key}}.AppendSQL(b)
		if joined {
			r.writeScope(b, &loadConfig{})
		}
	})
	if err != nil {
		return 0, err
	}
	rows, err := q.conn().query(ctx, "select", tr.Name, "", c)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	var n int64
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
	}
	return n, rows.Err()
}

type linkMode uint8

const (
	linkSet linkMode = iota
	linkClear
)

func (r Relation[P, C]) notLinkable(op string) error {
	return &Error{Kind: ErrUnsupported, Op: op, Table: r.Child.Name,
		Err: errors.New(r.Name + ": " + op + " is not defined for " + r.Kind.String() + "; set the foreign key on the parent")}
}

func (r Relation[P, C]) assocErr(table, msg string) error {
	return &Error{Kind: ErrValidation, Op: "assoc", Table: table, Err: errors.New(r.Name + ": " + msg)}
}

// parentKey returns the non-NULL parent key.
func (r Relation[P, C]) parentKey(p *P, table string) ([]any, error) {
	key, ok := r.ParentKey(p)
	if !ok || hasNilPart(key) {
		return nil, r.assocErr(table, "parent key is NULL")
	}
	return key, nil
}

// pkIndexes maps ChildPK names to Meta column indexes. Missing names are an ErrValidation.
func pkIndexes[C any](m *Meta[C], names []string) ([]int, error) {
	if m == nil || len(names) == 0 {
		return nil, &Error{Kind: ErrValidation, Op: "assoc", Err: errors.New("relation has no child primary key")}
	}
	out := make([]int, len(names))
	for i, n := range names {
		found := -1
		for j, col := range m.Columns {
			if col.Name == n {
				found = j
				break
			}
		}
		if found < 0 {
			return nil, &Error{Kind: ErrValidation, Op: "assoc", Err: errors.New("primary key column not in model: " + n)}
		}
		out[i] = found
	}
	return out, nil
}

// pkValues returns the primary key tuple of c, in ChildPK order.
func pkValues[C any](m *Meta[C], idx []int, c *C) []any {
	out := make([]any, len(idx))
	for i, j := range idx {
		out[i] = m.Value(c, j)
	}
	return out
}

// discArgs is the number of bind arguments the discriminator adds to a statement.
func (r Relation[P, C]) discArgs() int {
	if r.Discriminator != nil {
		return 1
	}
	return 0
}

// chunkSize is how many key tuples (ncols binds each) fit in one statement
// alongside reserved other binds.
func (r Relation[P, C]) chunkSize(q Querier, ncols, reserved int) int {
	return keysPerStatement(q.Dialect(), ncols, reserved)
}

// runChunks splits keys into chunks of size n and runs fn once per chunk. When
// q is a *DB and more than one chunk is needed the chunks share one transaction,
// so the call either applies in full or not at all.
func (r Relation[P, C]) runChunks(ctx context.Context, q Querier, keys [][]any, n int, fn func(Querier, [][]any) error) error {
	if len(keys) == 0 {
		return nil
	}
	run := func(x Querier) error {
		for i := 0; i < len(keys); i += n {
			if err := fn(x, keys[i:min(i+n, len(keys))]); err != nil {
				return err
			}
		}
		return nil
	}
	if db, ok := q.(*DB); ok && len(keys) > n {
		return db.Tx(ctx, TxOptions{}, func(tx *Tx) error { return run(tx) })
	}
	return run(q)
}

// nullableFK wraps a NOT NULL violation from clearing a foreign key.
func (r Relation[P, C]) nullableFK(err error) error {
	if err != nil && errors.Is(err, ErrNotNullViolation) {
		return &Error{Kind: ErrNotNullViolation, Op: "assoc", Table: r.Child.Name,
			Err: fmt.Errorf("%s: Unlink and Replace need a nullable foreign key: %w", r.Name, err)}
	}
	return err
}

// childPKs returns the non-NULL parent key and the child primary keys of cs.
func (r Relation[P, C]) childPKs(p *P, cs []*C) ([]any, [][]any, error) {
	key, err := r.parentKey(p, r.Child.Name)
	if err != nil {
		return nil, nil, err
	}
	idx, err := pkIndexes(r.ChildMeta, r.ChildPK)
	if err != nil {
		return nil, nil, err
	}
	keys := make([][]any, len(cs))
	for i, c := range cs {
		keys[i] = pkValues(r.ChildMeta, idx, c)
		if hasNilPart(keys[i]) {
			return nil, nil, r.assocErr(r.Child.Name, "child primary key is NULL")
		}
	}
	return key, keys, nil
}

// updateChunk writes the foreign key of the children with the given primary
// keys (linkSet) or clears it on them (linkClear). With no keys, linkClear
// clears every child of the parent key (Replace with an empty set).
func (r Relation[P, C]) updateChunk(ctx context.Context, q Querier, key []any, keys [][]any, mode linkMode) error {
	tbl := r.Child.Qualifier()
	st, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("UPDATE ")
		r.Child.appendFrom(b)
		b.WriteString(" SET ")
		for i, col := range r.ChildCols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Ident(col)
			b.WriteString(" = ")
			if mode == linkSet {
				b.Arg(key[i])
			} else {
				b.WriteString("NULL")
			}
		}
		if r.Discriminator != nil && mode == linkSet {
			b.WriteString(", ")
			b.Ident(r.Discriminator.Column)
			b.WriteString(" = ")
			b.Arg(r.Discriminator.Value)
		}
		b.WriteString(" WHERE ")
		if mode != linkSet {
			inTuple{table: tbl, cols: r.ChildCols, keys: [][]any{key}, flags: keyFlags(r.ChildMeta, r.ChildCols)}.AppendSQL(b)
			if r.Discriminator != nil {
				b.WriteString(" AND ")
				b.QualifiedIdent(tbl, r.Discriminator.Column)
				b.WriteString(" = ")
				b.Arg(r.Discriminator.Value)
			}
			if len(keys) > 0 {
				b.WriteString(" AND ")
			}
		}
		if len(keys) > 0 {
			inTuple{table: tbl, cols: r.ChildPK, keys: keys, flags: keyFlags(r.ChildMeta, r.ChildPK)}.AppendSQL(b)
		}
	})
	if err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "update", r.Child.Name, st)
	if mode != linkSet {
		err = r.nullableFK(err)
	}
	return err
}

// joinKeys returns the non-NULL parent key and the child keys of cs.
func (r Relation[P, C]) joinKeys(p *P, cs []*C) ([]any, [][]any, error) {
	if r.Through == nil {
		return nil, nil, r.assocErr(r.Child.Name, "many-to-many relation has no join table")
	}
	pkey, err := r.parentKey(p, r.Through.Table.Name)
	if err != nil {
		return nil, nil, err
	}
	ckeys := make([][]any, len(cs))
	for i, c := range cs {
		k, ok := r.ChildKey(c)
		if !ok || hasNilPart(k) {
			return nil, nil, r.assocErr(r.Child.Name, "child key is NULL")
		}
		ckeys[i] = k
	}
	return pkey, ckeys, nil
}

// insertJoinChunk inserts one join row per child key in one INSERT. Existing
// pairs are skipped with ON CONFLICT DO NOTHING, or with a no-op ON DUPLICATE KEY
// UPDATE on MySQL. A dialect with neither capability gets a plain INSERT.
func (r Relation[P, C]) insertJoinChunk(ctx context.Context, q Querier, pkey []any, ckeys [][]any) error {
	d := q.Dialect()
	cols := append(append([]string{}, r.Through.ParentCols...), r.Through.ChildCols...)
	st, err := compile(d, func(b *Builder) {
		b.WriteString("INSERT INTO ")
		r.Through.Table.appendFrom(b)
		b.WriteString(" (")
		for i, col := range cols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Ident(col)
		}
		b.WriteString(") VALUES ")
		for i, ck := range ckeys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("(")
			vals := append(append([]any{}, pkey...), ck...)
			for j, v := range vals {
				if j > 0 {
					b.WriteString(", ")
				}
				b.Arg(v)
			}
			b.WriteString(")")
		}
		switch {
		case d.OnConflict:
			b.WriteString(" ON CONFLICT DO NOTHING")
		case d.OnDuplicateKey:
			b.WriteString(" ON DUPLICATE KEY UPDATE ")
			b.Ident(cols[0])
			b.WriteString(" = ")
			b.Ident(cols[0])
		}
	})
	if err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "insert", r.Through.Table.Name, st)
	return err
}

// deleteJoinChunk deletes the join rows of the parent key for the given child
// keys; with no keys it deletes every join row of the parent key.
func (r Relation[P, C]) deleteJoinChunk(ctx context.Context, q Querier, pkey []any, ckeys [][]any) error {
	tbl := r.Through.Table.Qualifier()
	st, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("DELETE FROM ")
		r.Through.Table.appendFrom(b)
		b.WriteString(" WHERE ")
		inTuple{table: tbl, cols: r.Through.ParentCols, keys: [][]any{pkey}}.AppendSQL(b)
		if len(ckeys) > 0 {
			b.WriteString(" AND ")
			inTuple{table: tbl, cols: r.Through.ChildCols, keys: ckeys}.AppendSQL(b)
		}
	})
	if err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "delete", r.Through.Table.Name, st)
	return err
}
