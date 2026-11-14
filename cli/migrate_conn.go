// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/mysqldriver"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/driver/sqlitepure"
	"github.com/azrtydxb/momo/migrate"
)

// maxMySQLConns bounds the pool of a MySQL or MariaDB run: the engine pins one
// connection for the lock and uses others for its reads.
const maxMySQLConns = 4

// openDriver opens the database at dsn with the momo driver of the chosen
// dialect and returns it with the func that closes its pool. A nil driver comes
// with the exit code to return: 2 for a URL that cannot be used, 3 for a
// database that cannot be reached.
//
// SQLite uses driver/sqlitepure, the pure-Go driver, so the momo binary needs
// no cgo and no C toolchain; it has no vector functions, which a migration does
// not need.
func (l *cli) openDriver(ctx context.Context, dsn string) (momo.Driver, func(), int) {
	switch dialect.Name(l.dialectName) {
	case dialect.Postgres:
		pool, err := pgxdriver.NewPool(ctx, dsn)
		if err != nil {
			return nil, nil, l.errf(exitDatabase, "connect: %v", err)
		}
		return pgxdriver.New(pool), pool.Close, -1
	case dialect.SQLite:
		target, err := sqliteTarget(dsn)
		if err != nil {
			return nil, nil, l.errf(exitUsage, "%v", err)
		}
		db, drv, err := sqlitepure.Open(target, sqlitepure.Config{})
		if err != nil {
			return nil, nil, l.errf(exitDatabase, "connect: %v", err)
		}
		return drv, func() { _ = db.Close() }, -1
	case dialect.MySQL:
		return l.openMySQL(ctx, dsn)
	}
	return nil, nil, l.errf(exitUsage, "no driver for dialect %q", l.dialectName)
}

// openMySQL opens a MySQL or MariaDB server. The server, not the URL, decides
// the flavor: mysqldriver.Open takes one, so it opens as MySQL and the driver is
// rebuilt as MariaDB when VERSION() says so.
func (l *cli) openMySQL(ctx context.Context, raw string) (momo.Driver, func(), int) {
	cfg, err := mysqlConfig(raw)
	if err != nil {
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	if cfg.DBName == "" {
		return nil, nil, l.errf(exitUsage, "the MySQL URL names no database: the migration history and the lock live in one database, so add /dbname")
	}
	dsn := cfg.FormatDSN()
	l.secrets = append(l.secrets, dsn)
	db, drv, err := mysqldriver.Open(dsn, dialect.FlavorMySQL)
	if err != nil {
		return nil, nil, l.errf(exitDatabase, "connect: %v", err)
	}
	db.SetMaxOpenConns(maxMySQLConns)
	maria, err := isMariaDB(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, nil, l.errf(exitDatabase, "connect: read the server version: %v", err)
	}
	if maria {
		drv = mysqldriver.New(db, dialect.FlavorMariaDB)
	}
	return drv, func() { _ = db.Close() }, -1
}

// isMariaDB reports whether the server behind db is MariaDB.
func isMariaDB(ctx context.Context, db *sql.DB) (bool, error) {
	var v string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v); err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(v), "mariadb"), nil
}

// urlScheme matches the scheme of a URL; a go-sql-driver DSN has none. A
// scheme taken from user input is never quoted in an error: in a DSN the text
// before "://" can be a user name.
var urlScheme = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*)://`)

// windowsDrivePath matches the path of a sqlite:///C:/x.db URL.
var windowsDrivePath = regexp.MustCompile(`^/[A-Za-z]:[/\\]`)

// sqliteTarget turns the --url of the sqlite dialect into the DSN of
// sqlitepure. It accepts a file path, a "file:" URI (passed on as it is) and a
// sqlite:// URL: sqlite:///abs/path.db is an absolute path, sqlite://rel.db a
// relative one and sqlite:///C:/x.db a Windows drive path; the path of a URL is
// percent-decoded. A query string on a sqlite:// URL is passed on as a "file:"
// URI (mode and cache are URI parameters). A plain path takes no query string:
// a "?" in it is an error, as a file: URI with %3F is the way to name it. A
// memory database is refused, because each command would open its own empty
// one. Errors never quote the value.
func sqliteTarget(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("the SQLite URL names no database file")
	}
	target := raw
	if m := urlScheme.FindStringSubmatch(raw); m != nil {
		switch strings.ToLower(m[1]) {
		case "sqlite":
			path, query, hasQuery := strings.Cut(raw[len(m[0]):], "?")
			if path == "" {
				return "", errors.New("the SQLite URL names no database file: use sqlite:///absolute/path.db or sqlite://relative.db")
			}
			dec, err := url.PathUnescape(path)
			if err != nil {
				return "", errors.New("the SQLite URL path has a bad percent escape")
			}
			if windowsDrivePath.MatchString(dec) {
				dec = dec[1:]
			}
			target = dec
			if hasQuery {
				target = "file:" + strings.ReplaceAll(dec, "?", "%3F") + "?" + query
			}
		case "file":
			target = raw
		default:
			return "", errors.New("unsupported URL scheme for the sqlite dialect: use a file path, a file: URI or sqlite:///path.db")
		}
	} else if strings.Contains(raw, "?") && !strings.HasPrefix(strings.ToLower(raw), "file:") {
		return "", errors.New("a SQLite file path cannot contain \"?\": percent-encode it as %3F in a file: URI, or rename the file")
	}
	if isMemorySQLite(target) {
		return "", errors.New("an in-memory SQLite database lasts one command and cannot hold migrations: name a database file")
	}
	return target, nil
}

// isMemorySQLite reports whether the SQLite target names a memory database:
// the path ":memory:" (with or without the file: prefix) or the URI parameter
// mode=memory. A file whose name merely contains those words is not one.
func isMemorySQLite(target string) bool {
	path, query, _ := strings.Cut(target, "?")
	path = strings.TrimPrefix(path, "file:")
	if path == ":memory:" || path == "" && strings.HasPrefix(target, "file:") {
		return true
	}
	vals, err := url.ParseQuery(query)
	if err != nil {
		return false
	}
	return vals.Get("mode") == "memory"
}

// mysqlConfig parses the --url of the mysql dialect. It accepts a mysql:// or
// mariadb:// URL (user:password@host:port/database?options) and a
// go-sql-driver DSN. ?socket=/path names a unix socket and cannot be combined
// with a host; the other options are go-sql-driver parameters such as tls=true,
// and one given twice is an error. mysqldriver then forces parseTime=true,
// loc=UTC, clientFoundRows=true, interpolateParams=false and
// multiStatements=false whatever the URL says. Errors never quote the value,
// which carries the password.
func mysqlConfig(raw string) (*mysql.Config, error) {
	raw = strings.TrimSpace(raw)
	m := urlScheme.FindStringSubmatch(raw)
	if m == nil {
		cfg, err := mysql.ParseDSN(raw)
		if err != nil {
			return nil, errors.New("the MySQL DSN is not valid: want user:password@tcp(host:3306)/database or a mysql:// URL")
		}
		return cfg, nil
	}
	if s := strings.ToLower(m[1]); s != "mysql" && s != "mariadb" {
		return nil, errors.New("unsupported URL scheme for the mysql dialect: use mysql://, mariadb:// or a go-sql-driver DSN")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("the MySQL URL does not parse: percent-encode special characters in the user name and password")
	}
	q := u.Query()
	for _, vals := range q {
		if len(vals) > 1 {
			return nil, errors.New("the MySQL URL repeats a query parameter")
		}
	}
	socket := q.Get("socket")
	if socket != "" && u.Host != "" {
		return nil, errors.New("the MySQL URL has both a host and ?socket=: give one")
	}
	q.Del("socket")
	tmp := "/"
	if enc := q.Encode(); enc != "" {
		tmp += "?" + enc
	}
	cfg, err := mysql.ParseDSN(tmp)
	if err != nil {
		return nil, errors.New("the MySQL URL has an option that go-sql-driver rejects")
	}
	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Passwd, _ = u.User.Password()
	}
	if socket != "" {
		cfg.Net, cfg.Addr = "unix", socket
	} else {
		host, port := u.Hostname(), u.Port()
		if host == "" {
			host = "127.0.0.1"
		}
		if port == "" {
			port = "3306"
		}
		cfg.Net, cfg.Addr = "tcp", net.JoinHostPort(host, port)
	}
	db := strings.TrimPrefix(u.Path, "/")
	if strings.Contains(db, "/") {
		return nil, errors.New("the MySQL URL path must be one database name")
	}
	cfg.DBName = db
	// Parse the assembled DSN again so that what depends on the address, the
	// TLS server name above all, is derived from the final one.
	cfg, err = mysql.ParseDSN(cfg.FormatDSN())
	if err != nil {
		return nil, errors.New("the MySQL URL does not make a valid DSN")
	}
	return cfg, nil
}

// mysqlTLSWarning returns a non-empty warning when the MySQL connection string
// raw may reach a remote host without TLS: tls is not set (go-sql-driver then
// uses none), false or preferred (which falls back to plaintext). A local host
// and a unix socket stay quiet, and so does skip-verify, which encrypts. It
// names the host only, never the URL.
func mysqlTLSWarning(raw string) string {
	cfg, err := mysqlConfig(raw)
	if err != nil || cfg.Net != "tcp" {
		return ""
	}
	if cfg.TLS != nil && !cfg.AllowFallbackToPlaintext {
		return ""
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		host = cfg.Addr
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "", "localhost", "127.0.0.1", "::1":
		return ""
	}
	shown := "tls is not set; go-sql-driver then uses none"
	if cfg.TLSConfig != "" {
		shown = "tls=" + cfg.TLSConfig
	}
	return "TLS is not enforced (" + shown + ") for remote host " + host +
		": credentials and data can cross the network unencrypted; use tls=true"
}

// connWarning returns the TLS warning of the chosen dialect for the connection
// string u. A SQLite file has no network.
func (l *cli) connWarning(u string) string {
	switch dialect.Name(l.dialectName) {
	case dialect.SQLite:
		return ""
	case dialect.MySQL:
		return mysqlTLSWarning(u)
	}
	return tlsWarning(u, l.env)
}

// warnDialectConfig tells the user about the migrate: keys of momo.yaml that
// belong to another database than the chosen one. The keys are shared by every
// dialect of the file, so they are not errors, but they do nothing here.
func (l *cli) warnDialectConfig() {
	m := l.file.Migrate
	name := dialect.Name(l.dialectName)
	if (name == dialect.SQLite || name == dialect.MySQL) && len(m.ExternalExtensions) > 0 {
		_, _ = fmt.Fprintf(l.errOut, "momo: warning: migrate.external_extensions is PostgreSQL only and is ignored for %s\n", name)
	}
	if name == dialect.SQLite && (m.LockTimeout != "" || m.StatementTimeout != "") {
		_, _ = fmt.Fprintln(l.errOut, "momo: warning: migrate.lock_timeout and migrate.statement_timeout have no effect on SQLite; its busy handling is the driver's busy_timeout")
	}
}

// primaryFirst returns names with first at the front, added when it is missing.
func primaryFirst(names []dialect.Name, first dialect.Name) []dialect.Name {
	out := []dialect.Name{first}
	for _, n := range names {
		if n != first {
			out = append(out, n)
		}
	}
	return out
}

// failureWithHint is failure plus, on MySQL and MariaDB, the way out of a
// migration that failed half-way: DDL there is not transactional.
func (l *cli) failureWithHint(err error) int {
	code := l.failure(err)
	if code == exitDatabase && l.dialectName == string(dialect.MySQL) && !errors.Is(err, migrate.ErrLocked) {
		_, _ = fmt.Fprintln(l.errOut, "momo: hint: MySQL and MariaDB do not roll back DDL: if statements of the failed migration ran before the error, they stay applied. Run momo migrate status; a dirty migration needs the database repaired by hand, then momo migrate resolve --version <version> --applied|--rolled-back")
	}
	return code
}
