// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"

	"github.com/azrtydxb/momo"
)

const serverVersionSQL = "SELECT current_setting('server_version_num')::int"

// ServerVersion returns server_version_num (for example 170002). It
// implements momo.ServerVersioner; MERGE gates (PostgreSQL 15 or later) read it.
func (d *driver) ServerVersion(ctx context.Context) (int, error) {
	if d.modeErr != nil {
		return 0, d.modeErr
	}
	var v int
	if err := d.pool.QueryRow(ctx, serverVersionSQL, d.mode).Scan(&v); err != nil {
		return 0, ClassifyError(err)
	}
	return v, nil
}

// ServerVersion returns server_version_num through database/sql. It implements
// momo.ServerVersioner.
func (d *sqlDriver) ServerVersion(ctx context.Context) (int, error) {
	if d.err != nil {
		return 0, d.err
	}
	var v int
	if err := d.db.QueryRowContext(ctx, serverVersionSQL).Scan(&v); err != nil {
		return 0, ClassifyError(err)
	}
	return v, nil
}

var (
	_ momo.ServerVersioner = (*driver)(nil)
	_ momo.ServerVersioner = (*sqlDriver)(nil)
)
