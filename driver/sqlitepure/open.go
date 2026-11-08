// SPDX-License-Identifier: Apache-2.0

// Package sqlitepure adapts the pure-Go SQLite driver modernc.org/sqlite to
// momo. It needs no cgo and no C toolchain and builds on every platform Go
// targets, Windows included. It has no vector support: Capabilities reports
// Vectors=false and a statement that calls an sqlite-vec function fails with
// momo.ErrUnsupported. Use driver/sqlitedriver for vectors.
//
// The package does not import driver/sqlitedriver, so a build that uses it
// never links mattn/go-sqlite3 or sqlite-vec. Config is the same type as
// sqlitedriver.Config.
//
// Open uses the database/sql driver name "sqlite", which modernc registers
// when it is imported, and so does not register anything of its own. The
// package adds no process-wide hook: every setting travels in the DSN.
package sqlitepure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/sqliteconf"
)

// Config holds the recommended SQLite settings. It is the same type as
// sqlitedriver.Config; see that type for the defaults.
type Config = sqliteconf.Config

const driverName = "sqlite"

// Open opens a SQLite database with modernc.org/sqlite. The caller owns the
// returned *sql.DB. Open pings the database and checks that the bundled SQLite
// is at least 3.35.0, so a refused connection is reported here.
//
// dsn is a file path or a "file:" URI. The query string may carry only
// _txlock, mode and cache; the transaction lock in it overrides cfg. The busy
// timeout, journal mode, synchronous and foreign keys always come from cfg and
// the adapter, so a DSN cannot switch them off, and a "_pragma" parameter is
// refused for the same reason. The mode and cache parameters are SQLite URI
// parameters, which SQLite sees only for a "file:" DSN, so any other DSN
// carrying them is refused. mode=ro cannot open a database that is not in WAL
// mode yet, because the adapter sets journal_mode=WAL on every connection:
// create the database read-write first. An in-memory database with
// MaxOpenConns above 1 needs cache=shared, and then a statement can fail with
// SQLITE_LOCKED, which ClassifyError reports as momo.ErrBusy.
func Open(dsn string, cfg Config) (*sql.DB, momo.Driver, error) {
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
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, ClassifyError(err)
	}
	var version string
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		_ = db.Close()
		return nil, nil, ClassifyError(err)
	}
	if err := checkVersion(version); err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	return db, newDriver(db), nil
}

func buildDSN(dsn string, cfg Config) (string, error) {
	base, rawQuery, _ := strings.Cut(dsn, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("sqlitepure: parse DSN query: %w", err)
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
		return "", fmt.Errorf("sqlitepure: unknown synchronous mode %q", synchronous)
	}
	lock := cfg.TxLock
	if lock == "" {
		lock = "immediate"
	}
	switch strings.ToLower(lock) {
	case "immediate", "deferred", "exclusive":
	default:
		return "", fmt.Errorf("sqlitepure: unknown transaction lock %q", lock)
	}
	for key, vals := range q {
		if !allowedDSNKeys[key] {
			return "", fmt.Errorf("sqlitepure: DSN parameter %q is not allowed; the adapter sets the busy timeout, foreign keys, journal mode and synchronous itself (use Config), and other parameters are refused", key)
		}
		if len(vals) != 1 {
			return "", fmt.Errorf("sqlitepure: DSN parameter %s is given more than once", key)
		}
		switch key {
		case "mode", "cache":
			if !strings.HasPrefix(base, "file:") {
				return "", fmt.Errorf("sqlitepure: DSN parameter %s needs a \"file:\" DSN; SQLite does not see the query of a plain path", key)
			}
			if !allowedDSNValues[key][vals[0]] {
				return "", fmt.Errorf("sqlitepure: DSN parameter %s has an unsupported value", key)
			}
		case "_txlock":
			switch strings.ToLower(vals[0]) {
			case "immediate", "deferred", "exclusive":
			default:
				return "", errors.New("sqlitepure: DSN parameter _txlock has an unsupported value")
			}
		}
	}
	if isMemory(base, q) && cfg.MaxOpenConns > 1 && q.Get("cache") != "shared" {
		return "", errors.New("sqlitepure: an in-memory database is private to one connection, so MaxOpenConns must be 1 (or use a file, or cache=shared)")
	}
	if !q.Has("_txlock") {
		q.Set("_txlock", lock)
	}
	// modernc writes time.Time with Go's String layout unless told otherwise;
	// the "sqlite" layout is the one the scanners in timescan.go read.
	q.Set("_time_format", "sqlite")
	q["_pragma"] = []string{
		"busy_timeout(" + strconv.FormatInt(busy.Milliseconds(), 10) + ")",
		"journal_mode(WAL)",
		"synchronous(" + synchronous + ")",
		"foreign_keys(1)",
	}
	return base + "?" + q.Encode(), nil
}

// allowedDSNKeys is an allowlist: any other parameter, including ones a later
// modernc release adds, is refused. The pragmas are not on it because Config
// owns them, so a DSN cannot switch them off.
var allowedDSNKeys = map[string]bool{
	"_txlock": true,
	"mode":    true,
	"cache":   true,
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
	return base == "" || base == ":memory:"
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
	return fmt.Errorf("sqlitepure: SQLite %q is older than %d.%d.0 (RETURNING is required)", v, minMajor, minMinor)
}
