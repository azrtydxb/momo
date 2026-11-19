// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"context"
	"database/sql"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/sqlshared"
)

// openWith opens dsn through Open with cfg and returns the pool behind the
// driver next to it, for tests that run SQL on the pool.
func openWith(dsn string, cfg Config) (*sql.DB, momo.Driver, error) {
	d, err := Open(context.Background(), dsn, WithConfig(cfg))
	if err != nil {
		return nil, nil, err
	}
	return sqlshared.DB(d), d, nil
}
