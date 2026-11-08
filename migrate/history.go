// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/azrtydxb/momo"
)

// History row statuses. Dirty means StatusRunning or StatusFailed; a
// StatusRolledBack row records a transactional failure and is not dirty.
const (
	StatusRunning    = "running"
	StatusApplied    = "applied"
	StatusFailed     = "failed"
	StatusRolledBack = "rolled_back"
	StatusMeta       = "meta"
)

// MetaVersion is the version of the history table's metadata row.
const MetaVersion = "__momo_meta__"

// FormatVersion is the format version of the momo_migrations history table.
// It is stored in the checksum column of the metadata row and is independent
// of the module version.
const FormatVersion = 1

const timeLayout = time.RFC3339Nano

// HistoryRow is one row of the momo_migrations history table.
type HistoryRow struct {
	Version, Name, Checksum, Status, Error, ApplyID, Kind string
	StatementsApplied                                     int
	AppliedAt                                             time.Time
	ExecutionMS                                           int64
}

func (r HistoryRow) dirty() bool { return r.Status == StatusRunning || r.Status == StatusFailed }

// EnsureHistory creates the history table from d.HistoryDDL() and inserts the
// metadata row when it is missing. It refuses a table written with a newer
// history format.
func EnsureHistory(ctx context.Context, c momo.PinnedConn, d Dialect) error {
	tbl, err := resolveHistoryTable(ctx, c, d)
	if err != nil {
		return err
	}
	return ensureHistory(ctx, c, d, tbl)
}

// HistoryLocator is implemented by dialects that can name the history table
// schema-qualified. HistoryTable returns the SQL-ready, quoted reference of the
// table, resolved against the connection before any migration runs, and
// refuses a name that resolves to something that is not a regular table. A
// migration may change search_path later; the reference does not move.
// HistoryDDLFor is HistoryDDL for that reference. A dialect without it keeps
// the bare table name.
type HistoryLocator interface {
	HistoryTable(ctx context.Context, c momo.PinnedConn) (string, error)
	HistoryDDLFor(table string) []string
}

// ErrNoHistoryTable is returned (wrapped) by HistoryLocator.HistoryTable when
// no history table exists and the connection has no current schema to create
// one in. Read-only callers treat it as an empty history.
var ErrNoHistoryTable = errors.New("migrate: no history table and no schema to create it in")

// historyLockKeyer is implemented by locators that derive the advisory-lock key
// from the resolved history table reference.
type historyLockKeyer interface {
	HistoryLockKey(table string) string
}

// resolveHistoryTable returns the SQL reference of the history table of d on c.
func resolveHistoryTable(ctx context.Context, c momo.PinnedConn, d Dialect) (string, error) {
	if l, ok := d.(HistoryLocator); ok {
		return l.HistoryTable(ctx, c)
	}
	return historyTable, nil
}

func historyDDL(d Dialect, tbl string) []string {
	if l, ok := d.(HistoryLocator); ok {
		return l.HistoryDDLFor(tbl)
	}
	return d.HistoryDDL()
}

func ensureHistory(ctx context.Context, c momo.PinnedConn, d Dialect, tbl string) error {
	for _, ddl := range historyDDL(d, tbl) {
		if _, err := c.Exec(ctx, ddl, nil); err != nil {
			return fmt.Errorf("migrate: create history table: %w", err)
		}
	}
	// CREATE TABLE IF NOT EXISTS adopts a table that is already there. Resolve
	// again so that table passes the same checks as one found at the start
	// (kind, persistence, owner), and so it is the one named by tbl.
	again, err := resolveHistoryTable(ctx, c, d)
	if err != nil {
		return err
	}
	if again != tbl {
		return fmt.Errorf("migrate: the history table resolves to %s after it was created, not %s; the search_path changed under the migration", again, tbl)
	}
	ins := fmt.Sprintf(`INSERT INTO %s (version, name, checksum, status, applied_at) VALUES (%s, %s, %s, %s, %s) ON CONFLICT (version) DO NOTHING`,
		tbl, ph(d, 1), ph(d, 2), ph(d, 3), ph(d, 4), ph(d, 5))
	if _, err := c.Exec(ctx, ins, []any{MetaVersion, "meta", strconv.Itoa(FormatVersion), StatusMeta, time.Now().UTC().Format(timeLayout)}); err != nil {
		return fmt.Errorf("migrate: write history metadata: %w", err)
	}
	return checkHistoryFormat(ctx, c, tbl)
}

// checkHistoryFormat refuses a history table written with a newer format. It
// only reads: a database without the table or without a metadata row passes.
func checkHistoryFormat(ctx context.Context, c momo.PinnedConn, tbl string) error {
	rows, err := c.Query(ctx, `SELECT checksum FROM `+tbl+` WHERE status = 'meta'`, nil)
	if err != nil {
		if isUndefinedTable(err) {
			return nil
		}
		return fmt.Errorf("migrate: read history metadata: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		// A driver may report a missing table here instead of from Query.
		if err := rows.Err(); err != nil && !isUndefinedTable(err) {
			return fmt.Errorf("migrate: read history metadata: %w", err)
		}
		return nil
	}
	var raw string
	if err := rows.Scan(&raw); err != nil {
		return fmt.Errorf("migrate: read history metadata: %w", err)
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("migrate: history format %q is not a number; upgrade momo or repair the %s row", raw, MetaVersion)
	}
	if v > FormatVersion {
		return fmt.Errorf("migrate: the history table has format %d but this momo reads up to format %d; upgrade momo", v, FormatVersion)
	}
	return nil
}

// isUndefinedTable reports whether err is PostgreSQL's "relation does not exist".
func isUndefinedTable(err error) bool {
	var s interface{ SQLState() string }
	return errors.As(err, &s) && s.SQLState() == undefinedTable
}

// ReadHistory returns the migration rows of the history table ordered by
// version. The metadata row is not included. It names the table unqualified,
// so the connection's search_path decides which table it reads; the Engine
// does not use it and reads through a qualified reference instead.
func ReadHistory(ctx context.Context, c momo.PinnedConn) ([]HistoryRow, error) {
	return readHistoryRows(ctx, c, historyTable)
}

// readHistoryRows is ReadHistory for a history table reference.
func readHistoryRows(ctx context.Context, c momo.PinnedConn, tbl string) ([]HistoryRow, error) {
	rows, err := c.Query(ctx, `SELECT version, name, checksum, status, statements_applied, error, applied_at, execution_ms, apply_id, kind
FROM `+tbl+` WHERE status <> 'meta' ORDER BY version`, nil)
	if err != nil {
		return nil, fmt.Errorf("migrate: read history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []HistoryRow
	for rows.Next() {
		var r HistoryRow
		var at string
		if err := rows.Scan(&r.Version, &r.Name, &r.Checksum, &r.Status, &r.StatementsApplied, &r.Error, &at, &r.ExecutionMS, &r.ApplyID, &r.Kind); err != nil {
			return nil, fmt.Errorf("migrate: read history: %w", err)
		}
		r.AppliedAt, _ = time.Parse(timeLayout, at)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ph returns the n-th bind placeholder of d.
func ph(d Dialect, n int) string { return string(d.Dialect().AppendPlaceholder(nil, n)) }
