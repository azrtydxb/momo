// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"database/sql"

	"github.com/azrtydxb/momo/dialect"
)

// Driver is the connection-adapter contract. Adapters live in
// github.com/azrtydxb/momo/driver/... (pgx native, database/sql). The adapter owns
// the connection pool; Momo never opens or closes connections on its own.
type Driver interface {
	Dialect() *dialect.Dialect
	Query(ctx context.Context, sql string, args []any) (Rows, error)
	Exec(ctx context.Context, sql string, args []any) (int64, error)
	Begin(ctx context.Context, opts TxOptions) (DriverTx, error)
}

// DriverTx is a driver-level transaction.
type DriverTx interface {
	Query(ctx context.Context, sql string, args []any) (Rows, error)
	Exec(ctx context.Context, sql string, args []any) (int64, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Rows is a forward-only result cursor. Close must be safe to call more
// than once.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

// TxOptions configures a transaction.
type TxOptions struct {
	// Isolation is passed through to the driver. SQLite supports only
	// serializable semantics; any other non-default level is rejected
	// with ErrUnsupported rather than silently ignored.
	Isolation sql.IsolationLevel
	ReadOnly  bool
}
