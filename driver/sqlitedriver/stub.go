// SPDX-License-Identifier: Apache-2.0

//go:build !cgo || windows

package sqlitedriver

import (
	"database/sql"
	"errors"

	"github.com/azrtydxb/momo"
)

// SQLiteVecVersion is empty in this build: sqlite-vec is not linked. The cgo
// build reports the release it is tested against. The constant exists so code
// that names it compiles with and without cgo.
const SQLiteVecVersion = ""

// Open reports momo.ErrUnsupported: this build has no cgo, or targets
// Windows, where the sqlite-vec C sources cannot be compiled. There is no
// cgo-free SQLite driver in this module yet.
func Open(dsn string, cfg Config) (*sql.DB, momo.Driver, error) {
	return nil, nil, &momo.Error{Kind: momo.ErrUnsupported, Op: "open", Err: errors.New("sqlitedriver: needs cgo and a non-Windows target")}
}

// ClassifyError returns err unchanged: this build links no SQLite driver, so
// there is no driver error to map. It exists so code that calls it compiles
// with and without cgo.
func ClassifyError(err error) error { return err }
