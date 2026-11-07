// SPDX-License-Identifier: Apache-2.0

//go:build !cgo || windows

package sqlitedriver

import (
	"database/sql"
	"errors"

	"github.com/azrtydxb/momo"
)

// Open reports momo.ErrUnsupported: this build has no cgo, or targets
// Windows, where the sqlite-vec C sources cannot be compiled. Use package
// sqlitepure for SQLite without cgo.
func Open(dsn string, cfg Config) (*sql.DB, momo.Driver, error) {
	return nil, nil, &momo.Error{Kind: momo.ErrUnsupported, Op: "open", Err: errors.New("sqlitedriver: needs cgo and a non-Windows target")}
}
