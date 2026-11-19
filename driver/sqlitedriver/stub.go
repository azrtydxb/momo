// SPDX-License-Identifier: Apache-2.0

//go:build !cgo || windows

package sqlitedriver

import (
	"context"
	"database/sql"
	"errors"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// VecVersion is empty in this build: sqlite-vec is not linked. The cgo build
// reports the release it is tested against. The constant exists so code that
// names it compiles with and without cgo.
const VecVersion = ""

// SQLiteVecVersion is the old name of VecVersion.
//
// Deprecated: use VecVersion. It is removed in the next release.
const SQLiteVecVersion = VecVersion

// errUnsupported is what every call of this build reports.
func errUnsupported(op string) error {
	return &momo.Error{Kind: momo.ErrUnsupported, Op: op, Err: errors.New("sqlitedriver: needs cgo and a non-Windows target")}
}

// Open reports momo.ErrUnsupported: this build has no cgo, or targets
// Windows, where the sqlite-vec C sources cannot be compiled. There is no
// cgo-free SQLite driver in this module yet; driver/sqlitepure is one. The
// signature is the one of the cgo build, so code compiles with and without
// cgo.
func Open(_ context.Context, _ string, opts ...Option) (momo.Driver, error) {
	_ = config(opts)
	return nil, errUnsupported("open")
}

// New returns a driver whose every call reports momo.ErrUnsupported, for the
// reason Open does. The signature is the one of the cgo build.
func New(_ *sql.DB, _ ...Option) momo.Driver { return unsupportedDriver{} }

// unsupportedDriver is the driver of a build without cgo.
type unsupportedDriver struct{}

func (unsupportedDriver) Dialect() *dialect.Dialect { return dialect.For(dialect.SQLite) }

func (unsupportedDriver) Query(context.Context, string, []any) (momo.Rows, error) {
	return nil, errUnsupported("select")
}

func (unsupportedDriver) Exec(context.Context, string, []any) (int64, error) {
	return 0, errUnsupported("exec")
}

func (unsupportedDriver) Begin(context.Context, momo.TxOptions) (momo.DriverTx, error) {
	return nil, errUnsupported("begin")
}

// ClassifyError returns err unchanged: this build links no SQLite driver, so
// there is no driver error to map. It exists so code that calls it compiles
// with and without cgo.
func ClassifyError(err error) error { return err }
