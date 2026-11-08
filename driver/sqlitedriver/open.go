// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/azrtydxb/momo"
	"github.com/mattn/go-sqlite3"
)

const driverName = "momo-sqlite3"

var registerOnce sync.Once

func register() {
	registerOnce.Do(func() {
		sqlite_vec.Auto()
		sql.Register(driverName, &sqlite3.SQLiteDriver{ConnectHook: connectHook})
	})
}

// Open opens a SQLite database with mattn/go-sqlite3 and sqlite-vec. The
// caller owns the returned *sql.DB. Every new connection verifies
// sqlite_version() >= 3.35.0 and that load_extension is disabled; Open pings
// the database so a refused connection is reported here.
//
// dsn is a file path or a "file:" URI. Settings already present in its query
// string win over the defaults from cfg.
func Open(dsn string, cfg Config) (*sql.DB, momo.Driver, error) {
	register()
	full, err := buildDSN(dsn, cfg)
	if err != nil {
		return nil, nil, err
	}
	db, err := sql.Open(driverName, full)
	if err != nil {
		return nil, nil, err
	}
	n := cfg.MaxOpenConns
	if n <= 0 {
		n = 1
	}
	db.SetMaxOpenConns(n)
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, nil, ClassifyError(err)
	}
	return db, newDriver(db), nil
}

func buildDSN(dsn string, cfg Config) (string, error) {
	base, rawQuery, _ := strings.Cut(dsn, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("sqlitedriver: parse DSN query: %w", err)
	}
	busy := cfg.BusyTimeout
	if busy <= 0 {
		busy = 5 * time.Second
	}
	synchronous := cfg.Synchronous
	if synchronous == "" {
		synchronous = "NORMAL"
	}
	switch strings.ToUpper(synchronous) {
	case "NORMAL", "FULL", "EXTRA", "OFF":
	default:
		return "", fmt.Errorf("sqlitedriver: unknown synchronous mode %q", synchronous)
	}
	lock := cfg.TxLock
	if lock == "" {
		lock = "immediate"
	}
	switch strings.ToLower(lock) {
	case "immediate", "deferred", "exclusive":
	default:
		return "", fmt.Errorf("sqlitedriver: unknown transaction lock %q", lock)
	}
	for key := range q {
		if forbiddenDSNKeys[strings.ToLower(key)] {
			return "", fmt.Errorf("sqlitedriver: DSN parameter %q weakens the connection and is not allowed", key)
		}
	}
	if isMemory(base, q) && cfg.MaxOpenConns > 1 {
		return "", errors.New("sqlitedriver: an in-memory database is private to one connection, so MaxOpenConns must be 1 (or use a file)")
	}
	defaults := []struct {
		key, value string
		aliases    []string
	}{
		{"_busy_timeout", strconv.FormatInt(busy.Milliseconds(), 10), []string{"_timeout"}},
		{"_journal_mode", "WAL", []string{"_journal"}},
		{"_synchronous", synchronous, []string{"_sync"}},
		{"_foreign_keys", "on", []string{"_fk"}},
		{"_txlock", lock, nil},
	}
	for _, d := range defaults {
		present := q.Has(d.key)
		for _, a := range d.aliases {
			present = present || q.Has(a)
		}
		if !present {
			q.Set(d.key, d.value)
		}
	}
	return base + "?" + q.Encode(), nil
}

// forbiddenDSNKeys are mattn parameters that switch off a safety the adapter
// relies on: schema writes, schema triggers and views, read-only mode, CHECK
// enforcement and the authentication extension.
var forbiddenDSNKeys = map[string]bool{
	"_writable_schema":          true,
	"_trusted_schema":           true,
	"_query_only":               true,
	"_ignore_check_constraints": true,
	"_auth":                     true,
	"_auth_user":                true,
	"_auth_pass":                true,
	"_auth_crypt":               true,
	"_auth_salt":                true,
}

// isMemory reports whether the DSN names an in-memory database, which exists
// per connection.
func isMemory(base string, q url.Values) bool {
	return base == "" || base == ":memory:" || base == "file::memory:" || q.Get("mode") == "memory"
}

// The oldest SQLite the dialect description assumes (RETURNING).
const minMajor, minMinor = 3, 35

// checkVersion returns an error unless v is at least 3.35.0.
func checkVersion(v string) error {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) >= 2 {
		major, err1 := strconv.Atoi(parts[0])
		minor, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil && (major > minMajor || (major == minMajor && minor >= minMinor)) {
			return nil
		}
	}
	return fmt.Errorf("sqlitedriver: SQLite %q is older than %d.%d.0 (RETURNING is required)", v, minMajor, minMinor)
}

// checkLoadExtensionError interprets the result of the load_extension probe.
// SQLite answers "not authorized" when extension loading is disabled; a
// missing extension file fails differently, so only that message proves it.
func checkLoadExtensionError(err error) error {
	if err != nil {
		msg := err.Error()
		// "no such function" is a SQLite built without extension loading.
		if strings.Contains(msg, "not authorized") || strings.Contains(msg, "no such function: load_extension") {
			return nil
		}
	}
	return errors.New("sqlitedriver: load_extension is enabled on this connection; momo refuses it")
}

func connectHook(conn *sqlite3.SQLiteConn) error {
	rows, err := conn.Query("SELECT sqlite_version()", nil)
	if err != nil {
		return err
	}
	dest := make([]driver.Value, 1)
	err = rows.Next(dest)
	_ = rows.Close()
	if err != nil {
		return err
	}
	v, _ := dest[0].(string)
	if err := checkVersion(v); err != nil {
		return err
	}
	_, err = conn.Exec("SELECT load_extension('momo_probe')", nil)
	return checkLoadExtensionError(err)
}
