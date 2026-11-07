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
//	mtx := db.AdoptTx(pgxdriver.PgxTx(pgxTx))
//
// Statements use pgx.QueryExecModeExec like New does by default. Momo never
// commits or rolls the transaction back (Tx.Commit and Tx.Rollback return
// momo.ErrAdoptedTx); the caller does. PgxTx panics when tx is nil.
func PgxTx(tx pgx.Tx) momo.DriverTx {
	if tx == nil {
		panic("pgxdriver: PgxTx called with a nil pgx.Tx")
	}
	return &pgxTx{tx: tx, d: &driver{mode: pgx.QueryExecModeExec}}
}

// UnwrapSQLTx returns the *sql.Tx behind tx, so legacy database/sql code can
// run statements in the same transaction. It works for a Tx begun on a
// FromSQLDB driver, for a Tx from AdoptTx with SQLTx, and for a savepoint
// scope (which returns the outer transaction). The boolean is false when tx
// is nil or belongs to another driver kind, such as the pgxpool driver.
// Do not call Commit or Rollback on the result of a momo-begun Tx: finish it
// through momo.
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
// FromSQLDB. Do not call Commit or Rollback on the result of a momo-begun Tx:
// finish it through momo.
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
