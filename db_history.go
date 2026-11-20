// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"time"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// The operations a HistoryEntry records.
const (
	HistoryInsert = "INSERT"
	HistoryUpdate = "UPDATE"
	HistoryDelete = "DELETE"
)

// HistoryEntry is one recorded version of a row of a model that declares
// Config.History. Row holds the row as it was after an INSERT or UPDATE, and
// as it was when a DELETE removed it.
type HistoryEntry[T any] struct {
	// ID is the history id, which orders the entries of one row.
	ID int64
	// Operation is HistoryInsert, HistoryUpdate or HistoryDelete.
	Operation string
	// ChangedAt is the wall-clock time of the change, from clock_timestamp().
	ChangedAt time.Time
	// ChangedBy is the actor SetActor set in the transaction that made the
	// change, or nil when it set none (a change made outside momo, say).
	ChangedBy *string
	// Row is the version of the row.
	Row T
}

// SetActor records who is changing data in the rest of the transaction: the
// audit history trigger of a model that declares Config.History copies it into
// history_changed_by. It is a transaction-local setting (set_config with
// is_local = true), so it ends with the transaction, never reaches a pooled
// connection and is safe behind PgBouncer in transaction mode. Call it first in
// the transaction, or from a WithTxSetup function to do it for every
// transaction. A change made in a transaction that never called it records a
// NULL actor. It returns ErrUnsupported on SQLite, MySQL and MariaDB, which
// have no transaction-local setting and no audit history.
func SetActor(ctx context.Context, tx *Tx, actor string) error {
	return tx.SetLocal(ctx, schema.HistoryActorSetting, actor)
}

// HistoryOf returns every recorded version of the row of model T with the given
// primary key, in the order of the changes (the history id). key holds one
// value per primary key column, in the key's column order. A row with no
// recorded version gives an empty result. A model that never declared
// Config.History has no history table, and the call fails with the database's
// error for the missing table.
//
// A Predicate among the key values narrows the read like a Where does: pass
// TenantFilter(ctx, col) to scope it to the tenant in ctx, and a missing or
// mistyped tenant fails with ErrValidation. The history table is aliased as the
// model's table, so a predicate built from the model's column descriptors reads
// the history column of the same name.
//
// The versions are read from <table>_history. They are not filtered by
// soft-delete or interceptors, because the history is an audit log. It
// returns ErrUnsupported on SQLite, MySQL and MariaDB.
func HistoryOf[T any, PT Modeler[T]](ctx context.Context, q Querier, key ...any) ([]HistoryEntry[T], error) {
	return readHistory[T, PT](ctx, q, key, nil)
}

// HistoryAsOf returns the state of the row of model T with the given primary
// key at the time at: the latest version recorded at or before at. The time of
// a version is the moment its trigger fired (clock_timestamp()), not the moment
// its transaction committed. It returns an error matching ErrNotFound when the
// row did not exist then, either because its first version is later or because
// the latest version is a DELETE (a soft delete is an UPDATE, so the row still
// exists). See HistoryOf for key, tenant scoping and the rest of the rules.
func HistoryAsOf[T any, PT Modeler[T]](ctx context.Context, q Querier, at time.Time, key ...any) (HistoryEntry[T], error) {
	rows, err := readHistory[T, PT](ctx, q, key, &at)
	if err != nil {
		return HistoryEntry[T]{}, err
	}
	if len(rows) == 0 || rows[0].Operation == HistoryDelete {
		return HistoryEntry[T]{}, newError(ErrNotFound, "history", schema.HistoryTableName(metaOf[T, PT]().Table), errors.New("the row did not exist at that time"))
	}
	return rows[0], nil
}

// readHistory runs the history query of model T. A nil at reads every version
// oldest first; otherwise it reads the one version in force at *at.
func readHistory[T any, PT Modeler[T]](ctx context.Context, q Querier, key []any, at *time.Time) ([]HistoryEntry[T], error) {
	if q == nil || q.Dialect() == nil {
		return nil, noQuerier("history", "")
	}
	if q.Dialect().Name != dialect.Postgres {
		return nil, &Error{Kind: ErrUnsupported, Op: "history", Err: errors.New("audit history needs PostgreSQL")}
	}
	// A Predicate among the key values narrows the read, as it does a Where of
	// Select: TenantFilter(ctx, col) scopes the history to one tenant, and a
	// context without a tenant of the column's type fails with ErrValidation.
	var preds []Predicate
	var vals []any
	for _, k := range key {
		if p, ok := k.(Predicate); ok {
			preds = append(preds, p)
			continue
		}
		vals = append(vals, k)
	}
	key = vals
	meta := metaOf[T, PT]()
	switch {
	case meta == nil || meta.Table == "":
		return nil, validation("history", "", "%T is not a table model", *new(T))
	case meta.View:
		return nil, validation("history", meta.Table, "%T is a view model and has no history", *new(T))
	case len(meta.PK) == 0:
		return nil, validation("history", meta.Table, "%T has no primary key, so its versions cannot be found", *new(T))
	case len(key) != len(meta.PK):
		return nil, validation("history", meta.Table, "%T has %d primary key columns but %d key values were given", *new(T), len(meta.PK), len(key))
	}
	table := schema.HistoryTableName(meta.Table)
	c, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("SELECT ")
		for i, col := range meta.Columns {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Ident(col.Name)
		}
		for _, name := range []string{schema.HistoryIDColumn, schema.HistoryOperationColumn, schema.HistoryChangedAtColumn, schema.HistoryChangedByColumn} {
			b.WriteString(", ")
			b.Ident(name)
		}
		// The history table is aliased as the model's table, so a predicate
		// built on the model's columns (TenantFilter) reads the history column
		// of the same name.
		b.WriteString(" FROM ")
		b.QualifiedIdent(meta.Schema, table)
		b.WriteString(" AS ")
		b.Ident(meta.Table)
		b.WriteString(" WHERE ")
		for i, pk := range meta.PK {
			if i > 0 {
				b.WriteString(" AND ")
			}
			b.Ident(meta.Columns[pk].Name)
			b.WriteString(" = ")
			b.Arg(key[i])
		}
		for _, p := range preds {
			b.WriteString(" AND ")
			p.AppendSQL(b)
		}
		if at == nil {
			// The history id is the sequence of the changes, so it orders them.
			b.WriteString(" ORDER BY ")
			b.Ident(schema.HistoryIDColumn)
			return
		}
		b.WriteString(" AND ")
		b.Ident(schema.HistoryChangedAtColumn)
		b.WriteString(" <= ")
		b.Arg(*at)
		b.WriteString(" ORDER BY ")
		b.Ident(schema.HistoryChangedAtColumn)
		b.WriteString(" DESC, ")
		b.Ident(schema.HistoryIDColumn)
		b.WriteString(" DESC LIMIT 1")
	})
	if err != nil {
		return nil, err
	}
	e := q.conn()
	r, err := e.query(ctx, "select", table, "", c)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	var out []HistoryEntry[T]
	for r.Next() {
		var h HistoryEntry[T]
		dest := make([]any, 0, len(meta.Columns)+4)
		for i := range meta.Columns {
			dest = append(dest, meta.Ptr(&h.Row, i))
		}
		dest = append(dest, &h.ID, &h.Operation, &h.ChangedAt, &h.ChangedBy)
		if err := r.Scan(dest...); err != nil {
			return nil, e.wrapErr("select", table, err)
		}
		out = append(out, h)
	}
	if err := r.Err(); err != nil {
		return nil, e.wrapErr("select", table, err)
	}
	return out, r.Close()
}
