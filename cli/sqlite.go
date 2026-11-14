// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/azrtydxb/momo/driver/sqlitepure"
	"github.com/azrtydxb/momo/migrate"
)

// openSQLiteEngine builds an engine on the SQLite database at dsn with the
// pure-Go driver, so the CLI needs no cgo. Like every momo SQLite connection it
// puts the database in WAL mode, except when l.readOnly is set (generate
// --from-db): then the file is opened read-only and keeps its journal mode.
func (l *cli) openSQLiteEngine(dsn string, opts []migrate.Option) (*migrate.Engine, func(), int) {
	db, drv, err := sqlitepure.Open(sqliteDSN(dsn), sqlitepure.Config{ReadOnly: l.readOnly})
	if err != nil {
		return nil, nil, l.errf(exitDatabase, "connect: %v", err)
	}
	e, err := migrate.NewEngine(drv, dirFS(l.migrationDir()), opts...)
	if err != nil {
		_ = db.Close()
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	return e, func() { _ = db.Close() }, -1
}

// sqliteDSN turns a SQLite database URL into the DSN sqlitepure opens: a
// sqlite: or sqlite:// prefix is dropped, so sqlite:///var/app.db is the path
// /var/app.db and sqlite://app.db is the relative path app.db. A file: URI, a
// plain path and :memory: are used as they are. A relative path is relative to
// the working directory.
func sqliteDSN(raw string) string {
	raw = strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(raw, "sqlite://"); ok {
		return rest
	}
	if rest, ok := strings.CutPrefix(raw, "sqlite:"); ok {
		return rest
	}
	return raw
}

// looksLikeSQLiteSource reports whether the --from-db value names a SQLite
// database itself and not an environment variable: a URL or file: URI, a path
// (it holds a separator or a database file extension) or an existing file.
func looksLikeSQLiteSource(src string) bool {
	switch {
	case strings.HasPrefix(src, "sqlite:"), strings.HasPrefix(src, "file:"), src == ":memory:":
		return true
	case strings.ContainsAny(src, `/\`):
		return true
	}
	switch strings.ToLower(filepath.Ext(src)) {
	case ".db", ".sqlite", ".sqlite3", ".db3":
		return true
	}
	_, err := os.Stat(src)
	return err == nil
}

// sqliteFile reads the database file a --from-db value names: the path of a
// plain path, of a sqlite: URL, or of a file: URI (the file: prefix, an empty
// authority and the query dropped, percent escapes decoded). memory is true for
// :memory:, file::memory: and mode=memory, which name no file.
func sqliteFile(dsn string) (path string, memory bool) {
	p := sqliteDSN(dsn)
	if p == ":memory:" {
		return "", true
	}
	p, rawQuery, _ := strings.Cut(p, "?")
	rest, isURI := strings.CutPrefix(p, "file:")
	if !isURI {
		return p, false
	}
	if q, err := url.ParseQuery(rawQuery); err == nil && q.Get("mode") == "memory" {
		return "", true
	}
	rest = strings.TrimPrefix(rest, "//")
	if dec, err := url.PathUnescape(rest); err == nil {
		rest = dec
	}
	if rest == ":memory:" {
		return "", true
	}
	return rest, false
}

// requireSQLiteFile refuses a database that is not an existing file: an
// in-memory database holds nothing, and opening a missing file creates an empty
// one that a mistyped path would then pass off as a database with no tables.
func requireSQLiteFile(dsn string) error {
	path, memory := sqliteFile(dsn)
	if memory {
		return errors.New("--from-db: an in-memory SQLite database is empty; name a database file")
	}
	fi, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("--from-db: the SQLite database %q does not exist", path)
	case err != nil:
		return fmt.Errorf("--from-db: %w", err)
	case fi.IsDir():
		return fmt.Errorf("--from-db: %q is a directory, not a SQLite database file", path)
	}
	return nil
}
