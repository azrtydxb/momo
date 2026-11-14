// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/azrtydxb/momo/driver/sqlitepure"
	"github.com/azrtydxb/momo/migrate"
)

// openSQLiteEngine builds an engine on the SQLite database at dsn with the
// pure-Go driver, so the CLI needs no cgo. Like every momo SQLite connection it
// puts the database in WAL mode, which a read-only command such as
// generate --from-db leaves behind.
func (l *cli) openSQLiteEngine(dsn string, opts []migrate.Option) (*migrate.Engine, func(), int) {
	db, drv, err := sqlitepure.Open(sqliteDSN(dsn), sqlitepure.Config{})
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

// requireSQLiteFile refuses a database file that does not exist: opening one
// creates an empty database, and a mistyped path would then read as a database
// with no tables. A file: URI and :memory: are not checked.
func requireSQLiteFile(dsn string) error {
	path := sqliteDSN(dsn)
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	path, _, _ = strings.Cut(path, "?")
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
