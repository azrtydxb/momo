// SPDX-License-Identifier: Apache-2.0

package sqlitepure

import (
	"database/sql"
	"errors"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/sqlshared"
	"modernc.org/sqlite"
)

// sqlDriver is the shared SQLite driver plus the two things that differ here.
type sqlDriver struct{ *sqlshared.Driver }

func newDriver(db *sql.DB) momo.Driver {
	return &sqlDriver{sqlshared.New(db, sqlshared.Hooks{
		Classify: classifyOp,
		IsDriverErr: func(err error) bool {
			var se *sqlite.Error
			return errors.As(err, &se)
		},
	})}
}

func (d *sqlDriver) Dialect() *dialect.Dialect { return dialect.For(dialect.SQLite) }

// Capabilities reports no vector support: this driver has no sqlite-vec. There
// is no COPY or batch path.
func (d *sqlDriver) Capabilities() momo.Capabilities { return momo.Capabilities{Vectors: false} }

var _ momo.CapabilityReporter = (*sqlDriver)(nil)
