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
// dsn is a file path or a "file:" URI. The query string may carry only the
// parameters on allowedDSNKeys, each given once; a repeated parameter, a
// _txlock other than immediate, deferred or exclusive, a _busy_timeout or
// _timeout that is not a non-negative integer, a _cache_size that is not an
// integer (a negative one is a size in KiB) and a _loc that is neither "auto"
// nor a time zone name are refused with momo.ErrValidation. The busy timeout and the transaction lock in
// it override cfg; foreign keys, WAL and synchronous always come from cfg and
// the adapter. The mode and cache parameters are SQLite URI parameters, which
// SQLite sees only for a "file:" DSN, so any other DSN carrying them is
// refused. mode=ro cannot open a database that is not in WAL mode yet, because
// the adapter sets journal_mode=WAL on every connection and a read-only
// connection cannot change it: create the database read-write first. An
// in-memory database with MaxOpenConns above 1 needs cache=shared, and then a
// statement can fail with SQLITE_LOCKED, which ClassifyError reports as
// momo.ErrBusy.
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
	var vecVersion string
	if err := db.QueryRowContext(context.Background(), "SELECT vec_version()").Scan(&vecVersion); err != nil {
		_ = db.Close()
		return nil, nil, ClassifyError(err)
	}
	if err := checkVecVersion(vecVersion); err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	return db, newDriver(db), nil
}

// dsnInvalid is a momo.ErrValidation for a DSN parameter. The message never
// quotes the offending value.
func dsnInvalid(format string, args ...any) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "open", Err: fmt.Errorf("sqlitedriver: "+format, args...)}
}

func buildDSN(dsn string, cfg Config) (string, error) {
	base, rawQuery, _ := strings.Cut(dsn, "?")
	if base == "" {
		return "", &momo.Error{Kind: momo.ErrValidation, Op: "open", Err: errors.New("sqlitedriver: the DSN names no database; pass a file path, a \"file:\" URI or \":memory:\"")}
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("sqlitedriver: parse DSN query: %w", err)
	}
	if cfg.ReadOnly {
		return "", &momo.Error{Kind: momo.ErrValidation, Op: "open", Err: errors.New("sqlitedriver: Config.ReadOnly is not supported; use driver/sqlitepure")}
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
	for key, vals := range q {
		if !allowedDSNKeys[key] {
			return "", fmt.Errorf("sqlitedriver: DSN parameter %q is not allowed; the adapter sets foreign keys, journal mode and synchronous itself (use Config), and other parameters are refused", key)
		}
		if len(vals) != 1 {
			return "", dsnInvalid("DSN parameter %s is given more than once", key)
		}
		switch key {
		case "mode", "cache":
			if !strings.HasPrefix(base, "file:") {
				return "", fmt.Errorf("sqlitedriver: DSN parameter %s needs a \"file:\" DSN; SQLite does not see the query of a plain path", key)
			}
			if !allowedDSNValues[key][vals[0]] {
				return "", fmt.Errorf("sqlitedriver: DSN parameter %s has an unsupported value", key)
			}
		case "_txlock":
			switch strings.ToLower(vals[0]) {
			case "immediate", "deferred", "exclusive":
			default:
				return "", dsnInvalid("DSN parameter _txlock has an unsupported value")
			}
		case "_busy_timeout", "_timeout":
			if n, err := strconv.ParseInt(vals[0], 10, 64); err != nil || n < 0 {
				return "", dsnInvalid("DSN parameter %s must be a non-negative integer (milliseconds)", key)
			}
		case "_cache_size":
			// Negative is valid: SQLite reads it as a size in KiB.
			if _, err := strconv.ParseInt(vals[0], 10, 64); err != nil {
				return "", dsnInvalid("DSN parameter _cache_size must be an integer")
			}
		case "_loc":
			if vals[0] != "auto" {
				if _, err := time.LoadLocation(vals[0]); err != nil {
					return "", dsnInvalid("DSN parameter _loc must be \"auto\" or a time zone name")
				}
			}
		}
	}
	if isMemory(base, q) && cfg.MaxOpenConns > 1 && q.Get("cache") != "shared" {
		return "", errors.New("sqlitedriver: an in-memory database is private to one connection, so MaxOpenConns must be 1 (or use a file, or cache=shared)")
	}
	q.Set("_busy_timeout", busyValue(q, busy))
	q.Del("_timeout")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", synchronous)
	q.Set("_foreign_keys", "on")
	if !q.Has("_txlock") {
		q.Set("_txlock", lock)
	}
	return base + "?" + q.Encode(), nil
}

// busyValue is the busy timeout in milliseconds: the DSN's own value (under
// either name) when given, else the configured one.
func busyValue(q url.Values, busy time.Duration) string {
	for _, k := range []string{"_busy_timeout", "_timeout"} {
		if q.Has(k) {
			return q.Get(k)
		}
	}
	return strconv.FormatInt(busy.Milliseconds(), 10)
}

// allowedDSNKeys is an allowlist: any other parameter, including ones a later
// mattn release adds, is refused. Foreign keys, journal mode and synchronous
// are not on it because Config owns them (and their aliases), so a DSN cannot
// switch them off.
var allowedDSNKeys = map[string]bool{
	"_busy_timeout": true,
	"_timeout":      true,
	"_txlock":       true,
	"_loc":          true,
	"_cache_size":   true,
	"mode":          true,
	"cache":         true,
}

var allowedDSNValues = map[string]map[string]bool{
	"mode":  {"ro": true, "rw": true, "rwc": true, "memory": true},
	"cache": {"shared": true, "private": true},
}

// isMemory reports whether the DSN names an in-memory database, which exists
// per connection unless cache=shared. mode and cache count only for a "file:"
// DSN (buildDSN refuses them elsewhere); a bare path or ":memory:" is judged
// by the path alone.
func isMemory(base string, q url.Values) bool {
	if strings.HasPrefix(base, "file:") {
		return base == "file::memory:" || q.Get("mode") == "memory"
	}
	return base == ":memory:"
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
