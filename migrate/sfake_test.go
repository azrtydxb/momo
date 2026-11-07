// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// sfDB is a small in-memory stand-in for a PostgreSQL database that only
// understands the statements the migration engine writes to its history table.
// It lets the apply, history and rehash tests run without a server. It records
// every statement, the session options Setup received, and the history rows.
type sfDB struct {
	mu          sync.Mutex
	tableExists bool
	hist        []migrate.HistoryRow
	execs       []string
	setups      []migrate.SessionOptions
	// failOn, when it is a substring of a statement, makes that statement fail
	// with failErr.
	failOn  string
	failErr error
	// metaFormat is the checksum of the metadata row; empty means "1".
	metaFormat string
}

type sfSQLState string

func (s sfSQLState) Error() string    { return "relation \"momo_migrations\" does not exist" }
func (s sfSQLState) SQLState() string { return string(s) }

func (d *sfDB) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }

func (d *sfDB) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	return d.query(sql)
}

func (d *sfDB) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	return d.exec(sql, args)
}

func (d *sfDB) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return &sfTx{db: d, snapshot: slices.Clone(d.hist)}, nil
}

func (d *sfDB) Pin(ctx context.Context) (momo.PinnedConn, error) { return &sfConn{db: d}, nil }

func (d *sfDB) history() []migrate.HistoryRow {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.hist)
}

func (d *sfDB) row(version string) (migrate.HistoryRow, bool) {
	for _, r := range d.history() {
		if r.Version == version {
			return r, true
		}
	}
	return migrate.HistoryRow{}, false
}

func (d *sfDB) execLog() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.execs)
}

func (d *sfDB) query(sql string) (momo.Rows, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case strings.Contains(sql, "current_schema()"):
		return &sfRows{rows: [][]any{{"public"}}}, nil
	case strings.Contains(sql, "WHERE status = 'meta'"):
		if !d.tableExists {
			// Like pgx, report the missing table when the rows are read.
			return &sfRows{err: sfSQLState("42P01")}, nil
		}
		f := d.metaFormat
		if f == "" {
			f = "1"
		}
		return &sfRows{rows: [][]any{{f}}}, nil
	case strings.Contains(sql, "FROM momo_migrations WHERE status <> 'meta'"):
		if !d.tableExists {
			return nil, sfSQLState("42P01")
		}
		hist := slices.Clone(d.hist)
		slices.SortFunc(hist, func(a, b migrate.HistoryRow) int { return strings.Compare(a.Version, b.Version) })
		var rows [][]any
		for _, r := range hist {
			rows = append(rows, []any{r.Version, r.Name, r.Checksum, r.Status, r.StatementsApplied, r.Error, "2026-01-01T00:00:00Z", r.ExecutionMS, r.ApplyID, r.Kind})
		}
		return &sfRows{rows: rows}, nil
	}
	return nil, fmt.Errorf("sfDB: unexpected query %q", sql)
}

func (d *sfDB) exec(sql string, args []any) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.execs = append(d.execs, sql)
	if d.failOn != "" && strings.Contains(sql, d.failOn) {
		return 0, d.failErr
	}
	switch {
	case strings.HasPrefix(sql, "CREATE TABLE IF NOT EXISTS momo_migrations"):
		d.tableExists = true
	case strings.HasPrefix(sql, "INSERT INTO momo_migrations"):
		if args[3] == migrate.StatusMeta {
			return 1, nil
		}
		r := migrate.HistoryRow{Version: args[0].(string), Name: args[1].(string), Checksum: args[2].(string), Status: args[3].(string),
			StatementsApplied: args[4].(int), Error: args[5].(string), ApplyID: args[8].(string), Kind: args[9].(string)}
		d.hist = slices.DeleteFunc(d.hist, func(x migrate.HistoryRow) bool { return x.Version == r.Version && strings.Contains(sql, "ON CONFLICT") })
		d.hist = append(d.hist, r)
	case strings.HasPrefix(sql, "DELETE FROM momo_migrations"):
		d.hist = slices.DeleteFunc(d.hist, func(x migrate.HistoryRow) bool {
			return x.Version == args[0].(string) && x.Status == migrate.StatusRolledBack
		})
	case strings.HasPrefix(sql, "UPDATE momo_migrations SET statements_applied"):
		d.update(args[1].(string), func(r *migrate.HistoryRow) { r.StatementsApplied = args[0].(int) })
	case strings.HasPrefix(sql, "UPDATE momo_migrations SET status"):
		d.update(args[4].(string), func(r *migrate.HistoryRow) {
			r.Status, r.StatementsApplied, r.Error = args[0].(string), args[1].(int), args[2].(string)
		})
	case strings.HasPrefix(sql, "UPDATE momo_migrations SET checksum"):
		var n int64
		d.update(args[2].(string), func(r *migrate.HistoryRow) {
			if r.Status == migrate.StatusApplied && r.Checksum == args[3].(string) {
				r.Checksum, r.Error = args[0].(string), args[1].(string)
				n++
			}
		})
		return n, nil
	}
	return 1, nil
}

func (d *sfDB) update(version string, f func(*migrate.HistoryRow)) {
	for i := range d.hist {
		if d.hist[i].Version == version {
			f(&d.hist[i])
		}
	}
}

type sfConn struct{ db *sfDB }

func (c *sfConn) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	return c.db.query(sql)
}
func (c *sfConn) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	return c.db.exec(sql, args)
}
func (c *sfConn) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	return c.db.Begin(ctx, opts)
}
func (c *sfConn) Release() error { return nil }

type sfTx struct {
	db       *sfDB
	snapshot []migrate.HistoryRow
}

func (t *sfTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	return t.db.query(sql)
}
func (t *sfTx) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	return t.db.exec(sql, args)
}
func (t *sfTx) Commit(ctx context.Context) error { return nil }
func (t *sfTx) Rollback(ctx context.Context) error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	t.db.hist = t.snapshot
	return nil
}

type sfRows struct {
	rows [][]any
	i    int
	err  error
}

func (r *sfRows) Next() bool { r.i++; return r.i <= len(r.rows) }
func (r *sfRows) Err() error { return r.err }
func (r *sfRows) Close() error {
	return nil
}
func (r *sfRows) Scan(dest ...any) error {
	for i, d := range dest {
		switch p := d.(type) {
		case *string:
			*p = r.rows[r.i-1][i].(string)
		case *int:
			*p = r.rows[r.i-1][i].(int)
		case *int64:
			*p = r.rows[r.i-1][i].(int64)
		default:
			return errors.New("sfRows: unsupported scan target")
		}
	}
	return nil
}

// sfDialect is a dialect whose Setup and Lock only record what they were asked.
type sfDialect struct{ db *sfDB }

func (sfDialect) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }
func (sfDialect) Inspect(context.Context, momo.PinnedConn, migrate.InspectOptions) (*schema.Schema, error) {
	return &schema.Schema{}, nil
}
func (sfDialect) Render([]migrate.Change) ([]migrate.Statement, error) { return nil, nil }
func (sfDialect) HistoryDDL() []string {
	return []string{"CREATE TABLE IF NOT EXISTS momo_migrations (version text)"}
}
func (d sfDialect) Setup(_ context.Context, _ momo.PinnedConn, s migrate.SessionOptions) (func() error, error) {
	d.db.mu.Lock()
	defer d.db.mu.Unlock()
	d.db.setups = append(d.db.setups, s)
	return func() error { return nil }, nil
}
func (sfDialect) Lock(context.Context, momo.PinnedConn, string) (func() error, func(context.Context) (string, error), error) {
	return func() error { return nil }, nil, nil
}

// sfSnap is a minimal snapshot file.
func sfSnap(version, parent string) string {
	return `{"format_version":1,"version":"` + version + `","parent":"` + parent + `","schema":{"format_version":1,"dialect":"postgres","tables":[]}}`
}
