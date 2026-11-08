// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
)

const historyTableName = "momo_migrations"

// HistoryDDL returns the momo_migrations DDL for the bare table name. The
// engine uses HistoryDDLFor with the qualified reference from HistoryTable.
func (d Dialect) HistoryDDL() []string { return d.HistoryDDLFor(quoteIdent(historyTableName)) }

// HistoryDDLFor returns the history table DDL for table, a SQL-ready
// reference such as the one HistoryTable returns. Keys are VARCHAR because
// MySQL cannot index TEXT without a prefix length.
func (Dialect) HistoryDDLFor(table string) []string {
	return []string{`CREATE TABLE IF NOT EXISTS ` + table + ` (
version VARCHAR(255) NOT NULL PRIMARY KEY,
name VARCHAR(255) NOT NULL,
checksum VARCHAR(255) NOT NULL,
status VARCHAR(32) NOT NULL,
statements_applied INT NOT NULL DEFAULT 0,
error TEXT NOT NULL,
applied_at VARCHAR(64) NOT NULL,
execution_ms BIGINT NOT NULL DEFAULT 0,
apply_id VARCHAR(255) NOT NULL DEFAULT '',
kind VARCHAR(16) NOT NULL DEFAULT 'sql'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`}
}

// HistoryTable returns the quoted, database-qualified reference of the
// momo_migrations table in the connection's default database. The reference is
// fixed before the first migration runs, so a migration that executes USE
// cannot redirect the history writes. When the connection has no default
// database the error wraps migrate.ErrNoHistoryTable.
func (Dialect) HistoryTable(ctx context.Context, c momo.PinnedConn) (string, error) {
	db, err := currentDatabase(ctx, c)
	if err != nil {
		return "", err
	}
	if db == "" {
		return "", fmt.Errorf("%w: the connection has no default database; name one in the DSN", migrate.ErrNoHistoryTable)
	}
	if err := checkHistoryKind(ctx, c, db); err != nil {
		return "", err
	}
	return quoteIdent(db) + "." + quoteIdent(historyTableName), nil
}

// checkHistoryKind refuses a momo_migrations in db that is not a base table: a
// view would pass CREATE TABLE IF NOT EXISTS and then be written through.
func checkHistoryKind(ctx context.Context, c momo.PinnedConn, db string) (err error) {
	rows, err := c.Query(ctx, `SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`, []any{db, historyTableName})
	if err != nil {
		return fmt.Errorf("migrate/mysql: locate the history table: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("migrate/mysql: locate the history table: %w", cerr)
		}
	}()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return fmt.Errorf("migrate/mysql: locate the history table: %w", err)
		}
		return nil
	}
	var kind string
	if err := rows.Scan(&kind); err != nil {
		return fmt.Errorf("migrate/mysql: locate the history table: %w", err)
	}
	if kind != "BASE TABLE" {
		return &momo.Error{Kind: momo.ErrValidation, Op: "migrate/mysql", Table: historyTableName, Err: fmt.Errorf("%s.%s is a %s, not a base table; drop it or rename it before running migrations", quoteIdent(db), historyTableName, strings.ToLower(kind))}
	}
	return nil
}

// HistoryLockKey returns the lock key for the history table reference that
// HistoryTable returned: "momo:<database>.momo_migrations". GET_LOCK names are
// server-wide, so the database keeps two databases on one server independent.
func (Dialect) HistoryLockKey(table string) string {
	rest := strings.TrimPrefix(table, "`")
	var sb strings.Builder
	for i := 0; i < len(rest); i++ {
		if rest[i] == '`' {
			if i+1 < len(rest) && rest[i+1] == '`' {
				sb.WriteByte('`')
				i++
				continue
			}
			break
		}
		sb.WriteByte(rest[i])
	}
	return "momo:" + sb.String() + "." + historyTableName
}

// Setup applies the session timeouts on the pinned connection.
//
// LockTimeout becomes lock_wait_timeout, in whole seconds rounded up: how long
// a statement waits for a metadata lock, which is what an ALTER TABLE queued
// behind a long transaction waits on. StatementTimeout becomes MariaDB's
// max_statement_time, which bounds every statement; MySQL's equivalent
// (max_execution_time) bounds SELECT only, so a positive StatementTimeout on
// MySQL is refused with momo.ErrUnsupported instead of being ignored. A zero
// duration leaves the setting at the server default. The returned reset
// restores both to the defaults, ignores cancellation of ctx, and discards the
// connection when the restore fails. If Setup fails part-way it resets what it
// applied before it returns.
func (Dialect) Setup(ctx context.Context, c momo.PinnedConn, s migrate.SessionOptions) (reset func() error, err error) {
	var applied []string
	reset = func() error {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
		defer cancel()
		var errs []error
		for _, v := range applied {
			if _, err := c.Exec(rctx, "SET SESSION "+v+" = DEFAULT", nil); err != nil {
				errs = append(errs, err)
			}
		}
		applied = nil
		err := errors.Join(errs...)
		if err != nil {
			err = errors.Join(err, discard(c))
		}
		return err
	}
	set := func(variable, value string) error {
		if _, err := c.Exec(ctx, "SET SESSION "+variable+" = "+value, nil); err != nil {
			return fmt.Errorf("migrate/mysql: set %s: %w", variable, err)
		}
		applied = append(applied, variable)
		return nil
	}
	if s.LockTimeout > 0 {
		if err := set("lock_wait_timeout", fmt.Sprint(ceilSeconds(s.LockTimeout))); err != nil {
			return nil, errors.Join(err, reset())
		}
	}
	if s.StatementTimeout > 0 {
		fl, err := serverFlavor(ctx, c)
		if err != nil {
			return nil, errors.Join(err, reset())
		}
		if fl != dialect.FlavorMariaDB {
			err := &momo.Error{Kind: momo.ErrUnsupported, Op: "migrate/mysql", Err: errors.New("a statement timeout cannot bound DDL on MySQL (max_execution_time applies to SELECT only); use MariaDB or leave the statement timeout unset")}
			return nil, errors.Join(err, reset())
		}
		if err := set("max_statement_time", fmt.Sprintf("%.3f", s.StatementTimeout.Seconds())); err != nil {
			return nil, errors.Join(err, reset())
		}
	}
	return reset, nil
}

func ceilSeconds(d time.Duration) int64 {
	return int64((d + time.Second - 1) / time.Second)
}
