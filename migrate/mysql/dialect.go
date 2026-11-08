// SPDX-License-Identifier: Apache-2.0

// Package mysql is the MySQL and MariaDB migration dialect: it reads a live
// database into a schema.Schema through information_schema, renders DDL, and
// serializes migration runs with GET_LOCK.
//
// DDL implies a commit on these servers, so a migration cannot be rolled back
// as a whole: the engine runs every MySQL migration statement by statement and
// records the number of statements applied (see migrate.RunMigration).
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Dialect is the MySQL and MariaDB implementation of migrate.Dialect. It holds
// no state; every method works on the pinned connection it is given, and the
// server flavor is read from VERSION() when a method needs it.
type Dialect struct{}

// New returns the MySQL dialect.
func New() Dialect { return Dialect{} }

// Dialect describes MySQL: identifier quoting, placeholders and types. The
// flavor-specific capabilities of the connected server come from the driver.
func (Dialect) Dialect() *dialect.Dialect { return dialect.For(dialect.MySQL) }

func quoteIdent(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// serverFlavor reports whether the server behind c is MariaDB or MySQL.
func serverFlavor(ctx context.Context, c momo.PinnedConn) (dialect.Flavor, error) {
	var v sql.NullString
	if err := queryRow(ctx, c, `SELECT VERSION()`, nil, &v); err != nil {
		return "", fmt.Errorf("migrate/mysql: read the server version: %w", err)
	}
	if strings.Contains(strings.ToLower(v.String), "mariadb") {
		return dialect.FlavorMariaDB, nil
	}
	return dialect.FlavorMySQL, nil
}

// currentDatabase returns DATABASE(), the schema the connection works in. It
// is empty when the DSN names none.
func currentDatabase(ctx context.Context, c momo.PinnedConn) (string, error) {
	var db sql.NullString
	if err := queryRow(ctx, c, `SELECT DATABASE()`, nil, &db); err != nil {
		return "", fmt.Errorf("migrate/mysql: read the current database: %w", err)
	}
	return db.String, nil
}

// queryRow runs q and scans the first row into dest. A query without a row is
// an error.
func queryRow(ctx context.Context, c momo.PinnedConn, q string, args []any, dest ...any) (err error) {
	rows, err := c.Query(ctx, q, args)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := rows.Close(); err == nil {
			err = cerr
		}
	}()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return errors.New("the query returned no row")
	}
	return rows.Scan(dest...)
}

// forEach runs q and calls fn for every row.
func forEach(ctx context.Context, c momo.PinnedConn, q string, args []any, fn func(momo.Rows) error) (err error) {
	rows, err := c.Query(ctx, q, args)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := rows.Close(); err == nil {
			err = cerr
		}
	}()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// discard destroys the pinned connection when its driver supports it, and
// reports why when it does not.
func discard(c momo.PinnedConn) error {
	d, ok := c.(momo.Discarder)
	if !ok {
		return errors.New("migrate/mysql: pinned connection cannot be discarded; its session state may leak into the pool")
	}
	return d.Discard()
}
