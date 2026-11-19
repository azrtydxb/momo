// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlshared_test

import (
	"context"
	"database/sql"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitedriver"
	"github.com/azrtydxb/momo/driver/sqlitepure"
	"github.com/azrtydxb/momo/internal/sqlshared"
)

func init() {
	openers = append(openers, opener{"sqlitedriver", func(dsn string, cfg sqlitepure.Config) (*sql.DB, momo.Driver, error) {
		d, err := sqlitedriver.Open(context.Background(), dsn, sqlitedriver.WithConfig(sqlitedriver.Config(cfg)))
		if err != nil {
			return nil, nil, err
		}
		return sqlshared.DB(d), d, nil
	}})
}
