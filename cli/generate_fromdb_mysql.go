// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/mysqldriver"
	"github.com/azrtydxb/momo/gen/fromdb"
	"github.com/azrtydxb/momo/migrate"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
	"github.com/go-sql-driver/mysql"
)

// mysqlSource is what --from-db learns about the MySQL or MariaDB server it
// opened: the flavor, and the facts the inspected schema cannot carry.
type mysqlSource struct {
	flavor dialect.Flavor
	extra  func(ctx context.Context) ([]fromdb.Finding, error)
}

// looksLikeMySQLDSN reports whether a --from-db value is a go-sql-driver DSN
// (user:pass@tcp(host:3306)/db, or tcp(host:3306)/db, or one with a unix
// socket), which has neither "://" nor "=". An environment variable name holds
// neither an "@" nor a parenthesis.
func looksLikeMySQLDSN(src string) bool { return strings.ContainsAny(src, "@(") }

// mysqlURLParams are the query parameters a mysql:// URL may carry. Anything
// else would reach the server as a session variable, so it is refused: use a
// go-sql-driver DSN for driver options.
var mysqlURLParams = map[string]bool{
	"tls": true, "timeout": true, "readTimeout": true, "writeTimeout": true,
	"charset": true, "collation": true, "maxAllowedPacket": true, "allowNativePasswords": true,
	"allowCleartextPasswords": true, "serverPubKey": true,
}

// mysqlDSN turns a mysql:// or mariadb:// URL into the DSN go-sql-driver reads;
// any other value is already a DSN and is returned as it is. A URL has no
// socket form: use a DSN such as user:pass@unix(/run/mysqld.sock)/db.
func mysqlDSN(raw string) (string, error) {
	scheme, _, ok := strings.Cut(raw, "://")
	if !ok || (scheme != "mysql" && scheme != "mariadb") {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("the MySQL URL is not valid")
	}
	cfg := mysql.NewConfig()
	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Passwd, _ = u.User.Password()
	}
	cfg.Net = "tcp"
	host, port := u.Hostname(), u.Port()
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "3306"
	}
	cfg.Addr = net.JoinHostPort(host, port)
	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	keys := make([]string, 0, len(u.Query()))
	for k := range u.Query() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !mysqlURLParams[k] {
			return "", fmt.Errorf("the MySQL URL has the query parameter %q, which a URL cannot carry: use a go-sql-driver DSN (user:pass@tcp(host:3306)/db?param=value) for driver options and sockets", k)
		}
		if cfg.Params == nil {
			cfg.Params = map[string]string{}
		}
		cfg.Params[k] = u.Query().Get(k)
	}
	return cfg.FormatDSN(), nil
}

// noteMySQLURL registers the value, the DSN made of it and its password as
// secrets to redact, and warns when a remote server is reached without tls.
func (l *cli) noteMySQLURL(u string) {
	l.secrets = append(l.secrets, u)
	conn, err := mysqlDSN(u)
	if err != nil {
		return
	}
	if conn != u {
		l.secrets = append(l.secrets, conn)
	}
	cfg, err := mysql.ParseDSN(conn)
	if err != nil {
		return
	}
	// A bare password shorter than four characters is not registered: plain
	// replacement would mangle every message that holds those characters. The
	// full value and the converted DSN, registered above, still cover it.
	if len(cfg.Passwd) >= 4 {
		l.secrets = append(l.secrets, cfg.Passwd)
	}
	if w := mysqlTLSWarning(cfg); w != "" {
		_, _ = fmt.Fprintf(l.errOut, "momo: warning: %s\n", w)
	}
}

// mysqlTLSWarning names a remote host reached over tcp with no tls setting. It
// never carries the credentials.
func mysqlTLSWarning(cfg *mysql.Config) string {
	if !strings.HasPrefix(cfg.Net, "tcp") {
		return ""
	}
	host := cfg.Addr
	if h, _, err := net.SplitHostPort(cfg.Addr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "", "localhost", "127.0.0.1", "::1":
		return ""
	}
	if cfg.TLS != nil || (cfg.TLSConfig != "" && cfg.TLSConfig != "false") {
		return ""
	}
	return "the MySQL connection to " + host + " sets no tls: the password and the rows cross the network in clear text; add tls=true (or skip-verify, preferred, or a registered config) to the DSN"
}

// What the inspector does not read. ON UPDATE and INVISIBLE live in EXTRA; a
// prefix primary key is a STATISTICS row of PRIMARY with a SUB_PART.
const (
	qMySQLColumnExtras = `SELECT TABLE_NAME, COLUMN_NAME, EXTRA FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND (EXTRA LIKE '%on update%' OR EXTRA LIKE '%invisible%') ORDER BY TABLE_NAME, ORDINAL_POSITION`
	qMySQLPrefixPK = `SELECT TABLE_NAME, COLUMN_NAME FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE() AND INDEX_NAME = 'PRIMARY' AND SUB_PART IS NOT NULL ORDER BY TABLE_NAME, SEQ_IN_INDEX`
)

// mysqlExtras reads the flavor and the findings the schema cannot carry. It
// returns nothing for another dialect.
func (l *cli) mysqlExtras(ctx context.Context) (dialect.Flavor, []fromdb.Finding, error) {
	if l.fromMySQL == nil {
		return "", nil, nil
	}
	finds, err := l.fromMySQL.extra(ctx)
	return l.fromMySQL.flavor, finds, err
}

// wanted reports whether a table of the database gets a model, so a finding
// about a skipped table is not printed.
func (l *cli) wanted(table string) bool {
	in := l.file.Introspect
	if len(in.Skip) > 0 && schema.MatchObject(in.Skip, "", table) {
		return false
	}
	return len(in.Only) == 0 || schema.MatchObject(in.Only, "", table)
}

// openFromDBEngine opens the engine --from-db inspects through: the MySQL
// family here, every other dialect through openEngineAt.
func (l *cli) openFromDBEngine(ctx context.Context, dsn string) (*migrate.Engine, func(), int) {
	if l.dialectName != string(dialect.MySQL) {
		return l.openEngineAt(ctx, dsn)
	}
	opts, err := l.engineOptions(mmysql.New())
	if err != nil {
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	conn, err := mysqlDSN(dsn)
	if err != nil {
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	sdb, drv, err := mysqldriver.Open(conn, dialect.FlavorMySQL)
	if err != nil {
		return nil, nil, l.errf(exitDatabase, "connect: %v", err)
	}
	var version string
	if err := sdb.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		_ = sdb.Close()
		return nil, nil, l.errf(exitDatabase, "connect: read the server version: %v", err)
	}
	flavor := dialect.FlavorMySQL
	if strings.Contains(strings.ToLower(version), "mariadb") {
		flavor = dialect.FlavorMariaDB
		drv = mysqldriver.New(sdb, flavor)
	}
	e, err := migrate.NewEngine(drv, dirFS(l.migrationDir()), opts...)
	if err != nil {
		_ = sdb.Close()
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	l.fromMySQL = &mysqlSource{flavor: flavor, extra: func(ctx context.Context) ([]fromdb.Finding, error) {
		var out []fromdb.Finding
		rows, err := sdb.QueryContext(ctx, qMySQLColumnExtras)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var table, column, extra string
			if err := rows.Scan(&table, &column, &extra); err != nil {
				return nil, err
			}
			if !l.wanted(table) {
				continue
			}
			msg := "INVISIBLE is not modeled: a database rebuilt from the models has the column visible"
			if strings.Contains(strings.ToLower(extra), "on update") {
				msg = "ON UPDATE is not modeled: a database rebuilt from the models does not change the column on UPDATE"
			}
			out = append(out, fromdb.Finding{Severity: fromdb.SevWarn, Object: table + "." + column, Msg: msg})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		pk, err := sdb.QueryContext(ctx, qMySQLPrefixPK)
		if err != nil {
			return nil, err
		}
		defer func() { _ = pk.Close() }()
		for pk.Next() {
			var table, column string
			if err := pk.Scan(&table, &column); err != nil {
				return nil, err
			}
			if !l.wanted(table) {
				continue
			}
			out = append(out, fromdb.Finding{Severity: fromdb.SevError, Object: table + "." + column,
				Msg: "the primary key uses a column prefix, which the models cannot declare: nothing is written; add the table to introspect.skip"})
		}
		return out, pk.Err()
	}}
	return e, func() { _ = sdb.Close() }, -1
}
