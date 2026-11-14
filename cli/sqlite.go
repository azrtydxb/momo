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

// openSQLiteFromDB builds the engine --from-db inspects through on the SQLite
// database target (as requireSQLiteFile returns it). The file is opened
// read-only with the pure-Go driver, so inspecting never writes the database,
// not even to switch it to WAL mode.
func (l *cli) openSQLiteFromDB(target string) (*migrate.Engine, func(), int) {
	d, err := l.dialectImpl()
	if err != nil {
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	opts, err := l.engineOptions(d)
	if err != nil {
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	db, drv, err := sqlitepure.Open(target, sqlitepure.Config{ReadOnly: true})
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

// sqliteTargetPath is the file path a sqliteTarget result names: the target
// itself for a plain path, or a file: URI with the prefix, the query and an
// empty or localhost authority dropped and the percent escapes decoded. Any
// other authority names a host, which a local file cannot be on: an error.
func sqliteTargetPath(target string) (string, error) {
	p, _, _ := strings.Cut(target, "?")
	rest, isURI := strings.CutPrefix(p, "file:")
	if !isURI {
		return p, nil
	}
	if auth, ok := strings.CutPrefix(rest, "//"); ok {
		host, path, _ := strings.Cut(auth, "/")
		if host != "" && !strings.EqualFold(host, "localhost") {
			return "", errors.New("the file: URI names a host other than localhost; a SQLite file must be local")
		}
		rest = "/" + path
		if path == "" {
			rest = ""
		}
	}
	if dec, err := url.PathUnescape(rest); err == nil {
		rest = dec
	}
	return rest, nil
}

// requireSQLiteFile turns the --from-db value into the target sqlitepure opens
// and refuses a database that is not an existing file: an in-memory database
// holds nothing, and opening a missing file would create an empty one that a
// mistyped path would then pass off as a database with no tables. The parsing is
// sqliteTarget's, the one `momo migrate` uses.
func requireSQLiteFile(src string) (string, error) {
	src = strings.TrimSpace(src)
	// A "sqlite:" prefix without slashes names a path, as it always did here.
	if rest, ok := strings.CutPrefix(src, "sqlite:"); ok && !strings.HasPrefix(rest, "//") {
		src = rest
	}
	if isMemorySQLite(src) {
		return "", errors.New("--from-db: an in-memory SQLite database is empty; name a database file")
	}
	target, err := sqliteTarget(src)
	if err != nil {
		return "", fmt.Errorf("--from-db: %w", err)
	}
	path, err := sqliteTargetPath(target)
	if err != nil {
		return "", fmt.Errorf("--from-db: %w", err)
	}
	fi, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("--from-db: the SQLite database %q does not exist", path)
	case err != nil:
		return "", fmt.Errorf("--from-db: %w", err)
	case fi.IsDir():
		return "", fmt.Errorf("--from-db: %q is a directory, not a SQLite database file", path)
	}
	return target, nil
}
