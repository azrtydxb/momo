// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
)

// Association helpers write the link between a parent and its children
// explicitly. Each helper issues the single documented statement and nothing
// else; none of them touches the in-memory values passed in.
//
// Statement contract:
//
//   - HasOne/HasMany Link: one UPDATE child SET fk = ? WHERE pk IN (...). For a
//     polymorphic relation the discriminator column is set in the same UPDATE.
//   - HasOne/HasMany Unlink: one UPDATE child SET fk = NULL WHERE fk = ? AND pk
//     IN (...) (and the discriminator, for a polymorphic relation). The foreign
//     key columns must be nullable.
//   - ManyToMany Link: one INSERT INTO join table (parent cols, child cols)
//     VALUES (...), ... that skips pairs which already exist (ON CONFLICT DO
//     NOTHING; MySQL: ON DUPLICATE KEY UPDATE with a no-op assignment of the
//     first column to itself).
//   - ManyToMany Unlink: one DELETE FROM join table WHERE parent cols = ? AND
//     (child cols) IN (...).
//   - Replace: one transaction with two statements: (1) clear the associations
//     that are not in the new set; (2) link the new set (skipped when the set is
//     empty). On a *DB the helper opens a transaction; on a *Tx it runs in a
//     savepoint.
//   - Count: one SELECT COUNT(*) over the related children (soft-deleted
//     children are not counted).
//   - On BelongsTo every helper returns ErrUnsupported: set the foreign key on
//     the parent instead.
//
// Link, Unlink and Replace return ErrValidation when the parent key (or, for
// ManyToMany, a child key) is NULL. Link and Unlink with no children are
// no-ops that issue no statement. Each statement binds every key, so a set that
// would exceed the dialect's bind limit (see WithMaxParams) is rejected with
// ErrValidation instead of being split into several statements.

// Link associates the listed children with p: HasOne and HasMany set the
// children's foreign key (one UPDATE), ManyToMany inserts the join rows (one
// INSERT, existing pairs are kept). BelongsTo returns ErrUnsupported.
func (r Relation[P, C]) Link(ctx context.Context, q Querier, p *P, cs ...*C) error {
	switch r.Kind {
	case HasOne, HasMany:
		if len(cs) == 0 {
			return nil
		}
		return r.updateChildren(ctx, q, p, cs, linkSet)
	case ManyToMany:
		if len(cs) == 0 {
			return nil
		}
		return r.insertJoinRows(ctx, q, p, cs)
	}
	return r.notLinkable("link")
}

// Unlink removes the association between p and the listed children: HasOne and
// HasMany clear the children's foreign key (one UPDATE), ManyToMany deletes the
// join rows (one DELETE). Children that are not associated with p are left
// alone. BelongsTo returns ErrUnsupported.
func (r Relation[P, C]) Unlink(ctx context.Context, q Querier, p *P, cs ...*C) error {
	switch r.Kind {
	case HasOne, HasMany:
		if len(cs) == 0 {
			return nil
		}
		return r.updateChildren(ctx, q, p, cs, linkClear)
	case ManyToMany:
		if len(cs) == 0 {
			return nil
		}
		return r.deleteJoinRows(ctx, q, p, cs, false)
	}
	return r.notLinkable("unlink")
}

// Replace makes the listed children the complete set associated with p. In one
// transaction it clears every association that is not in cs and then links cs
// (the second statement is skipped when cs is empty). On a *DB it opens the
// transaction, on a *Tx it uses a savepoint. BelongsTo returns ErrUnsupported.
func (r Relation[P, C]) Replace(ctx context.Context, q Querier, p *P, cs ...*C) error {
	switch r.Kind {
	case HasOne, HasMany, ManyToMany:
	default:
		return r.notLinkable("replace")
	}
	if _, err := r.parentKey(p, r.Child.Name); err != nil {
		return err // no transaction for a call that cannot succeed
	}
	fn := func(tx *Tx) error {
		if err := r.unlinkExcept(ctx, tx, p, cs); err != nil {
			return err
		}
		if len(cs) == 0 {
			return nil
		}
		return r.Link(ctx, tx, p, cs...)
	}
	switch qq := q.(type) {
	case *DB:
		return qq.Tx(ctx, TxOptions{}, fn)
	case *Tx:
		return qq.Tx(ctx, fn)
	}
	return &Error{Kind: ErrUnsupported, Op: "replace", Table: r.Child.Name, Err: errors.New("unknown Querier")}
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

// unlinkExcept removes every association of p that is not in cs (one UPDATE or
// one DELETE). Replace calls it before linking the new set.
func (r Relation[P, C]) unlinkExcept(ctx context.Context, q Querier, p *P, cs []*C) error {
	switch r.Kind {
	case HasOne, HasMany:
		return r.updateChildren(ctx, q, p, cs, linkClearExcept)
	case ManyToMany:
		return r.deleteJoinRows(ctx, q, p, cs, true)
	}
	return r.notLinkable("replace")
}

type linkMode uint8

const (
	linkSet linkMode = iota
	linkClear
	linkClearExcept
)

func (r Relation[P, C]) notLinkable(op string) error {
	return &Error{Kind: ErrUnsupported, Op: op, Table: r.Child.Name,
		Err: errors.New(r.Name + ": " + op + " is not defined for " + r.Kind.String() + "; set the foreign key on the parent")}
}

func (r Relation[P, C]) assocErr(table, msg string) error {
	return &Error{Kind: ErrValidation, Op: "assoc", Table: table, Err: errors.New(r.Name + ": " + msg)}
}

// checkBinds rejects a statement that binds more arguments than the dialect allows.
func (r Relation[P, C]) checkBinds(q Querier, c Compiled, table string) error {
	if m := q.Dialect().MaxParams; m > 0 && len(c.Args) > m {
		return r.assocErr(table, "statement would bind more arguments than the dialect limit; link fewer children per call")
	}
	return nil
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

// updateChildren writes the foreign key of cs (linkSet), clears it on cs
// (linkClear), or clears it on every child of p that is not in cs
// (linkClearExcept). Children are addressed by their primary key.
func (r Relation[P, C]) updateChildren(ctx context.Context, q Querier, p *P, cs []*C, mode linkMode) error {
	key, err := r.parentKey(p, r.Child.Name)
	if err != nil {
		return err
	}
	idx, err := pkIndexes(r.ChildMeta, r.ChildPK)
	if err != nil {
		return err
	}
	keys := make([][]any, len(cs))
	for i, c := range cs {
		keys[i] = pkValues(r.ChildMeta, idx, c)
		if hasNilPart(keys[i]) {
			return r.assocErr(r.Child.Name, "child primary key is NULL")
		}
	}
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
			inTuple{table: tbl, cols: r.ChildCols, keys: [][]any{key}}.AppendSQL(b)
			b.WriteString(" AND ")
			if r.Discriminator != nil {
				b.QualifiedIdent(tbl, r.Discriminator.Column)
				b.WriteString(" = ")
				b.Arg(r.Discriminator.Value)
				b.WriteString(" AND ")
			}
		}
		if mode == linkClearExcept {
			b.WriteString("NOT (")
		}
		inTuple{table: tbl, cols: r.ChildPK, keys: keys}.AppendSQL(b)
		if mode == linkClearExcept {
			b.WriteString(")")
		}
	})
	if err != nil {
		return err
	}
	if err := r.checkBinds(q, st, r.Child.Name); err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "update", r.Child.Name, st)
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

// insertJoinRows inserts one join row per child in one INSERT. Existing pairs are
// skipped with ON CONFLICT DO NOTHING, or with a no-op ON DUPLICATE KEY UPDATE on
// MySQL. A dialect with neither capability gets a plain INSERT.
func (r Relation[P, C]) insertJoinRows(ctx context.Context, q Querier, p *P, cs []*C) error {
	pkey, ckeys, err := r.joinKeys(p, cs)
	if err != nil {
		return err
	}
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
		for i := range cs {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("(")
			vals := append(append([]any{}, pkey...), ckeys[i]...)
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
	if err := r.checkBinds(q, st, r.Through.Table.Name); err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "insert", r.Through.Table.Name, st)
	return err
}

// deleteJoinRows deletes the join rows of p for cs (keep false), or the join rows
// of p for every child not in cs (keep true, used by Replace). One DELETE either way.
func (r Relation[P, C]) deleteJoinRows(ctx context.Context, q Querier, p *P, cs []*C, keep bool) error {
	pkey, ckeys, err := r.joinKeys(p, cs)
	if err != nil {
		return err
	}
	tbl := r.Through.Table.Qualifier()
	st, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("DELETE FROM ")
		r.Through.Table.appendFrom(b)
		b.WriteString(" WHERE ")
		inTuple{table: tbl, cols: r.Through.ParentCols, keys: [][]any{pkey}}.AppendSQL(b)
		b.WriteString(" AND ")
		if keep {
			b.WriteString("NOT (")
		}
		inTuple{table: tbl, cols: r.Through.ChildCols, keys: ckeys}.AppendSQL(b)
		if keep {
			b.WriteString(")")
		}
	})
	if err != nil {
		return err
	}
	if err := r.checkBinds(q, st, r.Through.Table.Name); err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "delete", r.Through.Table.Name, st)
	return err
}
