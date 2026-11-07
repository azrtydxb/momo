// SPDX-License-Identifier: Apache-2.0

// Package sqlitedriver adapts mattn/go-sqlite3 and sqlite-vec to momo. It needs
// cgo; the pure-Go alternative is package sqlitepure, which has no vectors.
//
// Importing the package has no side effect. The first Open registers the
// database/sql driver "momo-sqlite3" and calls sqlite_vec.Auto, which installs
// sqlite-vec with sqlite3_auto_extension. That registration is process-wide:
// every SQLite connection opened afterwards by any package in the process
// that links the same SQLite library gets the vec functions.
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

// Config holds the recommended SQLite settings. Zero values select the defaults.
type Config struct {
	BusyTimeout  time.Duration // default 5 * time.Second
	Synchronous  string        // "NORMAL" (default), "FULL", "EXTRA", "OFF"
	TxLock       string        // "immediate" (default), "deferred", "exclusive"
	MaxOpenConns int           // default 1 (single-writer guidance); 0 means 1
}

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
	defaults := [][2]string{
		{"_busy_timeout", strconv.FormatInt(busy.Milliseconds(), 10)},
		{"_journal_mode", "WAL"},
		{"_synchronous", synchronous},
		{"_foreign_keys", "on"},
		{"_txlock", lock},
	}
	for _, kv := range defaults {
		if !q.Has(kv[0]) {
			q.Set(kv[0], kv[1])
		}
	}
	return base + "?" + q.Encode(), nil
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
	if err != nil && strings.Contains(err.Error(), "not authorized") {
		return nil
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
