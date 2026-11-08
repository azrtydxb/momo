// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"database/sql"
	"errors"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/sqlshared"
	"github.com/mattn/go-sqlite3"
)

// sqlDriver is the shared SQLite driver plus the two things that differ here.
type sqlDriver struct{ *sqlshared.Driver }

func newDriver(db *sql.DB) momo.Driver {
	return &sqlDriver{sqlshared.New(db, sqlshared.Hooks{
		Classify: func(_ string, err error) error { return ClassifyError(err) },
		IsDriverErr: func(err error) bool {
			var se sqlite3.Error
			return errors.As(err, &se)
		},
	})}
}

func (d *sqlDriver) Dialect() *dialect.Dialect { return dialect.For(dialect.SQLite) }

// Capabilities reports vector support through sqlite-vec. There is no COPY or
// batch path.
func (d *sqlDriver) Capabilities() momo.Capabilities { return momo.Capabilities{Vectors: true} }

var _ momo.CapabilityReporter = (*sqlDriver)(nil)
