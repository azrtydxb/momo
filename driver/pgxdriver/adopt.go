// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"database/sql"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5"
)

// SQLTx wraps a *sql.Tx the caller began so momo can run inside it:
//
//	mtx := db.AdoptTx(pgxdriver.SQLTx(sqlTx))
//
// The database must be backed by the pgx stdlib driver. Momo never commits
// or rolls the transaction back (Tx.Commit and Tx.Rollback return
// momo.ErrAdoptedTx); the caller does, and may keep using sqlTx directly
// between momo statements. SQLTx panics when tx is nil.
func SQLTx(tx *sql.Tx) momo.DriverTx {
	if tx == nil {
		panic("pgxdriver: SQLTx called with a nil *sql.Tx")
	}
	return &sqlTx{tx: tx}
}

// PgxTx wraps a pgx.Tx the caller began so momo can run inside it:
//
//	mtx := db.AdoptTx(pgxdriver.PgxTx(pgxTx, opts...))
//
// Options are the ones New accepts: WithExecMode (default
// pgx.QueryExecModeExec) and WithVectorFormat (default VectorText). A mode
// that WithExecMode rejects makes PgxTx panic, since there is no error to
// return. Momo never commits or rolls the transaction back (Tx.Commit and
// Tx.Rollback return momo.ErrAdoptedTx); the caller does. PgxTx panics when
// tx is nil.
//
// The wrapper implements momo.Copier and momo.Batcher, so batch pipelining
// (relation loading) and COPY work on the adopted transaction. COPY always
// uses pgvector's binary format, and any vector column needs momo's codec on
// the caller's connection: build the pool with ConfigureVectors, or register
// the types in your own AfterConnect. Without it COPY of vector columns
// fails; plain statements still work, because VectorText sends text.
func PgxTx(tx pgx.Tx, opts ...Option) momo.DriverTx {
	if tx == nil {
		panic("pgxdriver: PgxTx called with a nil pgx.Tx")
	}
	d := &driver{mode: pgx.QueryExecModeExec}
	for _, o := range opts {
		o(d)
	}
	if d.modeErr != nil {
		panic("pgxdriver: PgxTx: " + d.modeErr.Error())
	}
	return &pgxTx{tx: tx, d: d}
}

// UnwrapSQLTx returns the *sql.Tx behind tx, so legacy database/sql code can
// run statements in the same transaction. It works for a Tx begun on a
// FromSQLDB driver, for a Tx from AdoptTx with SQLTx, and for a savepoint
// scope (which returns the outer transaction). The boolean is false when tx
// is nil or belongs to another driver kind, such as the pgxpool driver.
//
// Do not call Commit or Rollback on the handle of a momo-begun Tx: momo still
// owns that transaction (its done flag, hooks and, on a pinned session, the
// registration that Release waits for), so finish it through momo. Only the
// owner of an adopted transaction commits it.
func UnwrapSQLTx(tx *momo.Tx) (*sql.Tx, bool) {
	if tx == nil {
		return nil, false
	}
	t, ok := tx.DriverTx().(*sqlTx)
	if !ok {
		return nil, false
	}
	return t.tx, true
}

// UnwrapPgxTx returns the pgx.Tx behind tx, so code written against pgx can
// run statements in the same transaction. It works for a Tx begun on a New
// driver, for a Tx from AdoptTx with PgxTx, and for a savepoint scope. The
// boolean is false when tx is nil or belongs to another driver kind, such as
// FromSQLDB.
//
// Do not call Commit or Rollback on the handle of a momo-begun Tx: momo still
// owns that transaction (its done flag and hooks), so finish it through momo.
// Only the owner of an adopted transaction commits it.
func UnwrapPgxTx(tx *momo.Tx) (pgx.Tx, bool) {
	if tx == nil {
		return nil, false
	}
	t, ok := tx.DriverTx().(*pgxTx)
	if !ok {
		return nil, false
	}
	return t.tx, true
}
