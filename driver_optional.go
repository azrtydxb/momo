// SPDX-License-Identifier: Apache-2.0

package momo

import "context"

// PinnedConn is one database connection checked out of a driver's pool.
// Statements on it run on the same session. Begin starts a transaction on that
// session; Release returns the connection to the pool.
type PinnedConn interface {
	Query(ctx context.Context, sql string, args []any) (Rows, error)
	Exec(ctx context.Context, sql string, args []any) (int64, error)
	Begin(ctx context.Context, opts TxOptions) (DriverTx, error)
	Release() error
}

// Pinner is implemented by drivers that can pin one connection.
type Pinner interface {
	Pin(ctx context.Context) (PinnedConn, error)
}

// Copier is implemented by drivers with a native bulk-copy path.
type Copier interface {
	CopyFrom(ctx context.Context, table string, cols []string, rows [][]any) (int64, error)
}

// Batcher is implemented by drivers that send several statements in one round trip.
// Every statement is executed with Query; a statement without result rows yields
// a Rows whose Next returns false.
type Batcher interface {
	SendBatch(ctx context.Context, stmts []Compiled) ([]Rows, error)
}

// PoolStats is a snapshot of a driver's connection pool.
type PoolStats struct {
	MaxConns           int
	TotalConns         int
	AcquiredConns      int
	IdleConns          int
	AcquireCount       int64
	EmptyAcquireCount  int64
	EmptyAcquireWaitNS int64
}

// PoolStater is implemented by drivers that expose pool statistics.
type PoolStater interface {
	PoolStats() PoolStats
}

// ServerVersioner is implemented by drivers that report the server version
// (PostgreSQL server_version_num, e.g. 170002). MERGE gates read it.
type ServerVersioner interface {
	ServerVersion(ctx context.Context) (int, error)
}

// Capabilities describes what a driver can execute beyond the dialect.
type Capabilities struct {
	Vectors bool // sqlite-vec or pgvector functions are available
	Batch   bool
	Copy    bool
}

// CapabilityReporter is implemented by drivers that report driver-level capabilities.
// Vector builders check Vectors and return ErrUnsupported when it is false.
type CapabilityReporter interface {
	Capabilities() Capabilities
}
