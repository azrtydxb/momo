// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
)

// dialectSource is a model package with one column of each common kind. The
// SQL a dialect renders for it names the dialect's own types. The MySQL variant
// adds the kinds that MySQL stores in a type of their own (uuid, json, decimal,
// unsigned integers); the other dialects do not map unsigned integers.
func dialectSource(d string) string {
	imports, extra := "", ""
	if d == "mysql" {
		imports = "\t\"github.com/azrtydxb/momo/types\"\n"
		extra = "\tExternalID  types.UUID ~momo:^notnull^~\n" +
			"\tPayload     types.JSON[map[string]string]\n" +
			"\tPrice       types.Numeric ~momo:^precision:12,scale:2^~\n" +
			"\tSmall       uint8 ~momo:^notnull^~\n" +
			"\tCounter     uint64 ~momo:^notnull^~\n"
	}
	src := "package simple\n\nimport (\n\t\"time\"\n\n\t\"github.com/azrtydxb/momo\"\n" + imports + ")\n\n" +
		"// Tenant is the only model.\ntype Tenant struct {\n" +
		"\tmomo.Model ~momo:^table:tenants^~\n" +
		"\tID         int64 ~momo:^pk,identity^~\n" +
		"\tName       string ~momo:^size:200,notnull,unique^~\n" +
		"\tActive     bool ~momo:^notnull^~\n" +
		"\tSeen       time.Time ~momo:^notnull^~\n" +
		"\tScore      float64 ~momo:^notnull^~\n" +
		"\tNote       *string\n\tBlob       []byte\n" + extra + "}\n"
	return strings.NewReplacer("~", "`", "^", "\"").Replace(src)
}

// dialectModule copies the models of dialect d into a temporary module and returns its root.
func dialectModule(t *testing.T, d string) string {
	t.Helper()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "models.go"), dialectSource(d))
	return copyModule(t, src)
}

// dialectConfig is a momo.yaml for one dialect; extra is appended to its migrate section.
func dialectConfig(d, extra string) string {
	return "version: 1\ndialects: [" + d + "]\nmigrate:\n  dir:\n    " + d + ": migrations/" + d + "\n" + extra
}

// Catches: `momo migrate` still answering "not available yet" for SQLite,
// MySQL and MariaDB, or choosing the PostgreSQL dialect for them.
func TestDialectImplByName(t *testing.T) {
	cases := []struct {
		name string
		want migrate.Dialect
	}{
		{"postgres", mpg.New()},
		{"sqlite", msqlite.New()},
		{"mysql", mmysql.New()},
	}
	for _, c := range cases {
		l := &cli{dialectName: c.name}
		got, err := l.dialectImpl()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Dialect().Name != c.want.Dialect().Name {
			t.Errorf("%s: got the %s migration dialect, want %s", c.name, got.Dialect().Name, c.want.Dialect().Name)
		}
	}
}

// Catches: a project that lists several dialects, or none, building the models
// for the wrong one: the column types come from the first dialect, and a
// --dialect that does not lead would render PostgreSQL types for SQLite.
func TestPrimaryFirst(t *testing.T) {
	got := primaryFirst([]dialect.Name{dialect.Postgres, dialect.MySQL}, dialect.MySQL)
	if len(got) != 2 || got[0] != dialect.MySQL || got[1] != dialect.Postgres {
		t.Errorf("primaryFirst moved wrongly: %v", got)
	}
	got = primaryFirst(nil, dialect.SQLite)
	if len(got) != 1 || got[0] != dialect.SQLite {
		t.Errorf("primaryFirst of nothing: %v", got)
	}
}

// Catches: --dialect sqlite or mysql being refused as unknown, a configured
// single dialect not being picked up, and "mariadb" silently selecting
// something.
func TestChooseDialectNewDialects(t *testing.T) {
	cases := []struct {
		flag, yaml string
		want       string
		wantErr    bool
	}{
		{"sqlite", "", "sqlite", false},
		{"mysql", "", "mysql", false},
		{"", "dialects: [mysql]\n", "mysql", false},
		{"", "dialects: [sqlite]\n", "sqlite", false},
		{"", "migrate:\n  dir:\n    sqlite: m\n", "sqlite", false},
		{"", "dialects: [sqlite, mysql]\n", "", true},
		{"mariadb", "", "", true},
	}
	for _, c := range cases {
		dir := t.TempDir()
		cfg := yamlPath(t, dir, c.yaml)
		var out, errOut bytes.Buffer
		l := &cli{out: &out, errOut: &errOut, env: noEnv}
		l.c.config, l.c.dialect = cfg, c.flag
		l.cwd = dir
		f, _, err := loadConfigFile(cfg, dir)
		if err != nil {
			t.Fatal(err)
		}
		l.file = f
		got, err := l.chooseDialect()
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("flag %q yaml %q: got %q, %v; want %q, error %v", c.flag, c.yaml, got, err, c.want, c.wantErr)
		}
	}
}

// Catches: the SQLite URL forms drifting apart from what the usage text
// promises, a relative sqlite:// path read as absolute, and an in-memory
// database being accepted although each command would see an empty one.
func TestSQLiteTarget(t *testing.T) {
	ok := []struct{ in, want string }{
		{"app.db", "app.db"},
		{"/var/lib/app.db", "/var/lib/app.db"},
		{"sqlite:///var/lib/app.db", "/var/lib/app.db"},
		{"sqlite://data/app.db", "data/app.db"},
		{"SQLITE://data/app.db", "data/app.db"},
		{"sqlite:///var/lib/app.db?mode=rwc", "file:/var/lib/app.db?mode=rwc"},
		{"file:app.db?cache=shared", "file:app.db?cache=shared"},
		{"file:///var/lib/app.db", "file:///var/lib/app.db"},
		{`C:\data\app.db`, `C:\data\app.db`},
		{"sqlite://data/my%20app.db", "data/my app.db"},
		{"sqlite:///C:/data/app.db", "C:/data/app.db"},
		{"sqlite:///tmp/a%3Fb.db", "/tmp/a?b.db"},
		{"sqlite:///tmp/a%3Fb.db?mode=ro", "file:/tmp/a%3Fb.db?mode=ro"},
		// A name that merely contains the words is not a memory database.
		{"memory.db", "memory.db"},
		{"sqlite:///tmp/mode=memory.db", "/tmp/mode=memory.db"},
		{"/data/:memory:.db", "/data/:memory:.db"},
	}
	for _, c := range ok {
		got, err := sqliteTarget(c.in)
		if err != nil || got != c.want {
			t.Errorf("sqliteTarget(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{"", "  ", ":memory:", "file::memory:", "file:x?mode=memory", "sqlite://", "sqlite://?mode=ro", "sqlite://:memory:", "sqlite:///x.db?mode=memory", "data?.db", "sqlite:///a%zz.db", "postgres://u:secretpw@h/db", "mysql://u:secretpw@h/db", "secretuser://secretpw@h/x.db"} {
		_, err := sqliteTarget(in)
		if err == nil {
			t.Errorf("sqliteTarget(%q) accepted it", in)
		} else if strings.Contains(err.Error(), "secretpw") || strings.Contains(err.Error(), "secretuser") {
			t.Errorf("sqliteTarget(%q) error quotes the input: %v", in, err)
		}
	}
}

// Catches: a mysql:// URL turning into the wrong host, port, database, user or
// password (a password with an @ or a slash), a socket URL being dialled over
// tcp, and a go-sql-driver DSN no longer passing through.
func TestMySQLConfig(t *testing.T) {
	cases := []struct {
		in                           string
		user, pass, net, addr, dbase string
	}{
		{"mysql://root:momo@db.example.com:3307/app", "root", "momo", "tcp", "db.example.com:3307", "app"},
		{"mariadb://root:momo@db.example.com/app", "root", "momo", "tcp", "db.example.com:3306", "app"},
		{"mysql://root@/app", "root", "", "tcp", "127.0.0.1:3306", "app"},
		{"mysql://root:p%40ss%2Fw%3Ard@[::1]:3306/app", "root", "p@ss/w:rd", "tcp", "[::1]:3306", "app"},
		{"mysql://root:momo@/app?socket=/run/mysqld/mysqld.sock", "root", "momo", "unix", "/run/mysqld/mysqld.sock", "app"},
		{"root:momo@tcp(127.0.0.1:3306)/app?tls=false", "root", "momo", "tcp", "127.0.0.1:3306", "app"},
		{"root:momo@unix(/tmp/mysql.sock)/app", "root", "momo", "unix", "/tmp/mysql.sock", "app"},
		{"mysql://root:p%25ss%40w%3Ard%2F@db.example.com/app", "root", "p%ss@w:rd/", "tcp", "db.example.com:3306", "app"},
	}
	for _, c := range cases {
		cfg, err := mysqlConfig(c.in)
		if err != nil {
			t.Errorf("mysqlConfig(%q): %v", c.in, err)
			continue
		}
		if cfg.User != c.user || cfg.Passwd != c.pass || cfg.Net != c.net || cfg.Addr != c.addr || cfg.DBName != c.dbase {
			t.Errorf("mysqlConfig(%q) = user %q pass %q net %q addr %q db %q; want %q %q %q %q %q",
				c.in, cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName, c.user, c.pass, c.net, c.addr, c.dbase)
		}
	}
	// tls=true keeps the verified server name of the real host, not the placeholder
	// one the options were parsed with.
	cfg, err := mysqlConfig("mysql://root:momo@db.example.com/app?tls=true")
	if err != nil || cfg.TLS == nil || cfg.TLS.ServerName != "db.example.com" {
		t.Errorf("tls=true: %+v, %v; want TLS verified against db.example.com", cfg, err)
	}
	for _, in := range []string{"postgres://u:secretpw@h/db", "sqlite://x.db", "mysql://u:secretpw@h/a/b", "mysql://u:secretpw@h:notaport/db", "not a dsn secretpw",
		"mysql://u:secretpw@h/app?socket=/run/m.sock", "mysql://u:secretpw@h/app?tls=true&tls=false", "secretuser://secretpw@tcp(h)/db"} {
		_, err := mysqlConfig(in)
		if err == nil {
			t.Errorf("mysqlConfig(%q) accepted it", in)
		} else if strings.Contains(err.Error(), "secretpw") || strings.Contains(err.Error(), "secretuser") {
			t.Errorf("mysqlConfig(%q) error quotes the input: %v", in, err)
		}
	}
}

// Catches: a plaintext connection to a remote MySQL server passing without a
// warning, a local or TLS one warning for nothing, and the warning echoing the
// credentials.
func TestMySQLTLSWarning(t *testing.T) {
	warn := []string{
		"mysql://app:secretpw@db.example.com/app",
		"mysql://app:secretpw@10.0.0.5:3306/app?tls=false",
		"mysql://app:secretpw@db.example.com/app?tls=preferred",
		"app:secretpw@tcp(db.example.com:3306)/app",
		"app:secretpw@tcp(db.example.com:3306)/app?tls=preferred",
	}
	for _, u := range warn {
		w := mysqlTLSWarning(u)
		if !strings.Contains(w, "TLS") || !strings.Contains(w, "tls=true") || strings.Contains(w, "secretpw") || strings.Contains(w, "/app") {
			t.Errorf("mysqlTLSWarning(%q) = %q; want a warning naming the host and tls=true, without the URL", u, w)
		}
	}
	quiet := []string{
		"mysql://app:secretpw@localhost/app",
		"mysql://app:secretpw@127.0.0.1:3306/app",
		"mysql://app:secretpw@[::1]/app",
		"mysql://app:secretpw@/app?socket=/run/mysqld.sock",
		"mysql://app:secretpw@db.example.com/app?tls=true",
		"mysql://app:secretpw@db.example.com/app?tls=skip-verify",
		"app:secretpw@tcp(db.example.com)/app?tls=true",
		"app:secretpw@unix(/tmp/mysql.sock)/app",
		"not a connection string",
	}
	for _, u := range quiet {
		if w := mysqlTLSWarning(u); w != "" {
			t.Errorf("mysqlTLSWarning(%q) = %q; want none", u, w)
		}
	}
	// The warning of the chosen dialect is the one printed: SQLite has none and
	// a PostgreSQL URL keeps its own.
	l := &cli{dialectName: "sqlite", env: noEnv}
	if w := l.connWarning("postgres://u:p@db.example.com/x?sslmode=disable"); w != "" {
		t.Errorf("sqlite connWarning = %q, want none", w)
	}
	l.dialectName = "postgres"
	if w := l.connWarning("postgres://u:p@db.example.com/x?sslmode=disable"); w == "" {
		t.Error("postgres connWarning lost its warning")
	}
	l.dialectName = "mysql"
	if w := l.connWarning("mysql://u:p@db.example.com/x"); w == "" {
		t.Error("mysql connWarning has no warning for a remote plaintext host")
	}
}

// Catches: the URL checks running after a connection attempt, or an error
// message quoting the password of a MySQL URL, a DSN or a SQLite URL.
func TestMigrateURLUsageErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		dialect string
		url     string
		want    int
	}{
		{"sqlite in memory", "sqlite", ":memory:", 2},
		{"sqlite with a postgres url", "sqlite", "postgres://u:secretpw@127.0.0.1:1/x", 2},
		{"mysql without a database", "mysql", "mysql://root:secretpw@127.0.0.1:1/", 2},
		{"mysql with a postgres url", "mysql", "postgres://u:secretpw@127.0.0.1:1/x", 2},
		{"mysql dsn without a database", "mysql", "root:secretpw@tcp(127.0.0.1:1)/", 2},
		{"mysql unreachable", "mysql", "mysql://root:secretpw@127.0.0.1:1/app", 3},
		{"mysql dsn unreachable", "mysql", "root:secretpw@tcp(127.0.0.1:1)/app", 3},
		{"mysql url with a special password", "mysql", "mysql://root:secretpw%40x%2Fy%25z@127.0.0.1:1/app", 3},
	}
	for _, c := range cases {
		cfg := yamlPath(t, dir, "dialects: ["+c.dialect+"]\n")
		var out, errOut bytes.Buffer
		code := runMigrate(&out, &errOut, []string{"--config", cfg, "status"}, func(k string) string {
			if k == "MOMO_DATABASE_URL" {
				return c.url
			}
			return ""
		})
		if code != c.want {
			t.Errorf("%s: exit %d, want %d (%s)", c.name, code, c.want, errOut.String())
		}
		if all := out.String() + errOut.String(); strings.Contains(all, "secretpw") || strings.Contains(all, "secretpw@x/y%z") {
			t.Errorf("%s: output leaks the password: %s", c.name, all)
		}
	}
}

// Catches: offline `diff` (the models, the dialect and no database) failing or
// rendering another dialect's column types for SQLite, MySQL and MariaDB. With
// the PostgreSQL types the first apply on a real server fails.
func TestOfflineDiffRendersDialectTypes(t *testing.T) {
	cases := []struct {
		dialect string
		want    []string
		not     []string
	}{
		{"sqlite", []string{"create table", "integer", "real", "text", "blob"}, []string{"timestamp with time zone", "bigint", "boolean"}},
		{"mysql", []string{"create table", "bigint", "varchar(200)", "tinyint(1)", "datetime(6)", "double", "blob", "char(36)", "json", "decimal(12,2)", "tinyint unsigned", "bigint unsigned"}, []string{"timestamp with time zone", "boolean", "bytea", "double precision"}},
	}
	for _, c := range cases {
		t.Run(c.dialect, func(t *testing.T) {
			root := dialectModule(t, c.dialect)
			writeFile(t, filepath.Join(root, "momo.yaml"), dialectConfig(c.dialect, ""))
			chdir(t, root)
			var out, errOut bytes.Buffer
			if code := runMigrate(&out, &errOut, []string{"diff", "init"}, noEnv); code != 0 {
				t.Fatalf("diff: exit %d: %s", code, errOut.String())
			}
			ups, err := filepath.Glob(filepath.Join(root, "migrations", c.dialect, "*_init.up.sql"))
			if err != nil || len(ups) != 1 {
				t.Fatalf("want one up file, got %v, %v\n%s", ups, err, out.String())
			}
			b, err := os.ReadFile(ups[0]) //nolint:gosec // test path
			if err != nil {
				t.Fatal(err)
			}
			sql := strings.ToLower(string(b))
			for _, w := range c.want {
				if !strings.Contains(sql, w) {
					t.Errorf("the %s migration lacks %q:\n%s", c.dialect, w, b)
				}
			}
			for _, w := range c.not {
				if strings.Contains(sql, w) {
					t.Errorf("the %s migration holds %q, another dialect's type:\n%s", c.dialect, w, b)
				}
			}
			out.Reset()
			errOut.Reset()
			if code := runMigrate(&out, &errOut, []string{"diff", "again"}, noEnv); code != 0 || !strings.Contains(out.String(), "no changes") {
				t.Fatalf("a second diff must find nothing: exit %d, out %q, err %q", code, out.String(), errOut.String())
			}
			if code := runMigrate(&out, &errOut, []string{"validate"}, noEnv); code != 0 {
				t.Fatalf("validate: exit %d: %s", code, errOut.String())
			}
		})
	}
}

// Catches: a PostgreSQL-only momo.yaml key being applied silently to a
// database it does not exist on, and the SQLite timeouts that do nothing being
// accepted without a word.
func TestWarnDialectConfig(t *testing.T) {
	cases := []struct {
		dialect, yaml string
		want          string
	}{
		{"sqlite", "migrate:\n  external_extensions: [vector]\n", "external_extensions"},
		{"mysql", "migrate:\n  external_extensions: [vector]\n", "external_extensions"},
		{"sqlite", "migrate:\n  lock_timeout: 2s\n", "busy_timeout"},
		{"postgres", "migrate:\n  external_extensions: [vector]\n  lock_timeout: 2s\n", ""},
		{"mysql", "migrate:\n  lock_timeout: 2s\n", ""},
	}
	for _, c := range cases {
		dir := t.TempDir()
		cfg := yamlPath(t, dir, c.yaml)
		f, _, err := loadConfigFile(cfg, dir)
		if err != nil {
			t.Fatal(err)
		}
		var errOut bytes.Buffer
		l := &cli{errOut: &errOut, file: f, dialectName: c.dialect}
		l.warnDialectConfig()
		got := errOut.String()
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s %q: warned %q, want one naming %q", c.dialect, c.yaml, got, c.want)
		}
	}
}

// Catches: the CLI binary linking the cgo SQLite driver or sqlite-vec, which
// would make `go install` of momo need a C toolchain. With cgo off the import
// graph of cmd/momo must hold neither, and SQLite must come from the pure-Go
// driver.
func TestMomoBinaryNeedsNoCgo(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "../cmd/momo")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("go list is not available: %v\n%s", err, out)
	}
	pure := false
	for _, pkg := range strings.Fields(string(out)) {
		if strings.Contains(pkg, "mattn/go-sqlite3") || strings.Contains(pkg, "sqlite-vec") {
			t.Errorf("cmd/momo links %s, which needs cgo", pkg)
		}
		if strings.HasSuffix(pkg, "/driver/sqlitepure") {
			pure = true
		}
	}
	if !pure {
		t.Error("cmd/momo does not link driver/sqlitepure, so it has no SQLite driver")
	}
}

// Catches: a SQLite path being registered as a secret to redact, which turned
// every word of it that appeared in a message into "***" (a path with "data"
// made "database" read "***base").
func TestNoteURLSQLiteRegistersNoSecret(t *testing.T) {
	var errOut bytes.Buffer
	l := &cli{errOut: &errOut, env: noEnv, dialectName: "sqlite"}
	l.noteURL("/var/data/app.db")
	if len(l.secrets) != 0 {
		t.Errorf("a SQLite path was registered as a secret: %v", l.secrets)
	}
	if got := l.errf(3, "database /var/data/app.db is locked"); got != 3 || !strings.Contains(errOut.String(), "database /var/data/app.db is locked") {
		t.Errorf("a SQLite path was redacted from a message: %q", errOut.String())
	}
	errOut.Reset()
	l = &cli{errOut: &errOut, env: noEnv, dialectName: "mysql"}
	l.noteURL("mysql://u:secretpw@127.0.0.1/app")
	if len(l.secrets) == 0 {
		t.Error("a MySQL URL must still be registered as a secret")
	}
}
