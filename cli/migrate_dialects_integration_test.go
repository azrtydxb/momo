// SPDX-License-Identifier: Apache-2.0

//go:build integration

package cli

import (
	"bytes"
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitepure"
	testdb "github.com/azrtydxb/momo/internal/testdb"
)

// cliBackend is one database the CLI runs against. url opens (or creates) a
// database private to the test and returns the --url value; a MySQL server
// that the job does not provide skips the test.
type cliBackend struct {
	name, dialect string
	url           func(t *testing.T) string
	transactional bool
}

func cliBackends() []cliBackend {
	return []cliBackend{
		{"sqlite", "sqlite", func(t *testing.T) string { return "sqlite://" + filepath.ToSlash(testdb.SQLitePath(t)) }, true},
		{"mysql84", "mysql", func(t *testing.T) string { return testdb.MySQL84URL(t) }, false},
		{"mysql97", "mysql", func(t *testing.T) string { return testdb.MySQL97URL(t) }, false},
		{"mariadb", "mysql", func(t *testing.T) string { return testdb.MariaDBURL(t) }, false},
	}
}

// startDialectProject writes the models and momo.yaml of dialect d into a
// temporary module, makes it the working directory and returns its root.
func startDialectProject(t *testing.T, d string) string {
	t.Helper()
	root := dialectModule(t, d)
	writeFile(t, filepath.Join(root, "momo.yaml"), dialectConfig(d, "  url_env:\n    "+d+": MOMO_TEST_URL\n"))
	chdir(t, root)
	return root
}

// mysqlURLForm rewrites a go-sql-driver DSN as the equivalent mysql:// URL.
func mysqlURLForm(t *testing.T, dsn string) string {
	t.Helper()
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u := url.URL{Scheme: "mysql", User: url.UserPassword(cfg.User, cfg.Passwd), Host: cfg.Addr, Path: "/" + cfg.DBName}
	return u.String()
}

// TestMigrateCLILifecycleAllDialects drives diff, validate, plan, apply, status,
// drift, lint, rollback and apply again through the CLI on a real SQLite file
// and on every MySQL-family server the job provides; a server that is not
// provided skips its subtest. Catches: a dialect whose CLI path fails against a
// real database (wrong driver, URL, lock, history table or column types), and a
// schema the generator renders that the server then reports as drift.
func TestMigrateCLILifecycleAllDialects(t *testing.T) {
	for _, b := range cliBackends() {
		t.Run(b.name, func(t *testing.T) {
			dsn := b.url(t)
			root := startDialectProject(t, b.dialect)

			code, out, errOut := cliRun(t, dsn, "diff", "init")
			if code != 0 || !strings.Contains(out, "migrations/"+b.dialect+"/") || !strings.Contains(out, "_init.up.sql") {
				t.Fatalf("diff: exit %d, out %q, err %q", code, out, errOut)
			}
			if code, out, _ = cliRun(t, dsn, "diff", "again"); code != 0 || !strings.Contains(out, "no changes") {
				t.Fatalf("second diff: exit %d, out %q", code, out)
			}
			if code, _, errOut = cliRun(t, dsn, "validate"); code != 0 {
				t.Fatalf("validate: exit %d: %s", code, errOut)
			}
			if code, out, errOut = cliRun(t, dsn, "plan"); code != 0 || !strings.Contains(out, "pending") {
				t.Fatalf("plan: exit %d, out %q, err %q", code, out, errOut)
			}
			if code, out, errOut = cliRun(t, dsn, "apply"); code != 0 || !strings.Contains(out, "applied") {
				t.Fatalf("apply: exit %d, out %q, err %q", code, out, errOut)
			}
			if code, out, errOut = cliRun(t, dsn, "status"); code != 0 || !strings.Contains(out, "applied") {
				t.Fatalf("status: exit %d, out %q, err %q", code, out, errOut)
			}
			if code, out, errOut = cliRun(t, dsn, "drift"); code != 0 || !strings.Contains(out, "no drift") {
				t.Fatalf("drift after apply: exit %d, out %q, err %q", code, out, errOut)
			}
			if code, out, _ = cliRun(t, dsn, "apply", "--lock-wait", "1s"); code != 0 || !strings.Contains(out, "no pending") {
				t.Fatalf("second apply: exit %d, out %q", code, out)
			}
			if code, _, errOut = cliRun(t, dsn, "validate", "--db"); code != 0 {
				t.Fatalf("validate --db: exit %d: %s", code, errOut)
			}
			if code, _, errOut = cliRun(t, dsn, "lint"); code != 0 {
				t.Fatalf("lint: exit %d: %s", code, errOut)
			}
			// "--url -" reads the same URL from standard input.
			if code, out, errOut = cliRunStdin(t, dsn, "status"); code != 0 || !strings.Contains(out, "applied") {
				t.Fatalf("status with --url -: exit %d, out %q, err %q", code, out, errOut)
			}
			if b.dialect == "mysql" {
				// The mysql:// URL reaches the same database as the DSN.
				if code, out, errOut = cliRun(t, mysqlURLForm(t, dsn), "status"); code != 0 || !strings.Contains(out, "applied") {
					t.Fatalf("status through a mysql:// URL: exit %d, out %q, err %q", code, out, errOut)
				}
			} else {
				// A bare path reaches the same file as the sqlite:// URL.
				path := strings.TrimPrefix(dsn, "sqlite://")
				if code, out, errOut = cliRun(t, path, "status"); code != 0 || !strings.Contains(out, "applied") {
					t.Fatalf("status through a file path: exit %d, out %q, err %q", code, out, errOut)
				}
			}

			// Rollback needs a down file; without one it refuses with exit 1.
			if code, _, errOut = cliRun(t, dsn, "rollback"); code != 1 || !strings.Contains(errOut, "down") {
				t.Fatalf("rollback without a down file: exit %d, want 1: %s", code, errOut)
			}
			ups, err := filepath.Glob(filepath.Join(root, "migrations", b.dialect, "*.up.sql"))
			if err != nil || len(ups) != 1 {
				t.Fatalf("want one up file, got %v %v", ups, err)
			}
			writeFile(t, strings.TrimSuffix(ups[0], ".up.sql")+".down.sql", "DROP TABLE tenants;\n")
			if code, _, errOut = cliRun(t, dsn, "hash"); code != 0 {
				t.Fatalf("hash: exit %d: %s", code, errOut)
			}
			if code, out, errOut = cliRun(t, dsn, "rollback", "--group"); code != 0 || !strings.Contains(out, "rolled back") {
				t.Fatalf("rollback: exit %d, out %q, err %q", code, out, errOut)
			}
			if code, out, _ = cliRun(t, dsn, "plan"); code != 0 || strings.Contains(out, "no pending") {
				t.Fatalf("the rolled back migration must be pending, plan said %q", out)
			}
			if code, out, errOut = cliRun(t, dsn, "apply"); code != 0 || !strings.Contains(out, "applied") {
				t.Fatalf("apply after rollback: exit %d, out %q, err %q", code, out, errOut)
			}
		})
	}
}

// TestMigrateCLIDriftAllDialects checks that drift reports a table the models
// do not declare, exit 1, on SQLite and every MySQL-family server provided.
// Catches: the drift path inspecting nothing, or the wrong schema, on a dialect.
func TestMigrateCLIDriftAllDialects(t *testing.T) {
	for _, b := range cliBackends() {
		t.Run(b.name, func(t *testing.T) {
			dsn := b.url(t)
			root := startDialectProject(t, b.dialect)
			for _, args := range [][]string{{"diff", "init"}, {"apply"}} {
				if code, _, errOut := cliRun(t, dsn, args...); code != 0 {
					t.Fatalf("%v: exit %d: %s", args, code, errOut)
				}
			}
			// A custom migration that adds a table the snapshot does not know.
			if code, _, errOut := cliRun(t, dsn, "new", "--custom", "stray"); code != 0 {
				t.Fatalf("new --custom: exit %d: %s", code, errOut)
			}
			ups, err := filepath.Glob(filepath.Join(root, "migrations", b.dialect, "*_stray.up.sql"))
			if err != nil || len(ups) != 1 {
				t.Fatalf("want one custom up file, got %v %v", ups, err)
			}
			writeFile(t, ups[0], "CREATE TABLE stray (id integer PRIMARY KEY);\n")
			if code, _, errOut := cliRun(t, dsn, "hash"); code != 0 {
				t.Fatalf("hash: exit %d: %s", code, errOut)
			}
			if code, _, errOut := cliRun(t, dsn, "apply"); code != 0 {
				t.Fatalf("apply custom: exit %d: %s", code, errOut)
			}
			// The snapshot of a custom migration is a copy of the previous one,
			// so the table the file created is drift.
			code, out, errOut := cliRun(t, dsn, "drift")
			if code != 1 || !strings.Contains(out, "stray") {
				t.Fatalf("drift: exit %d, want 1 naming stray; out %q, err %q", code, out, errOut)
			}
		})
	}
}

// TestMigrateCLIFailedMigration applies a migration whose last statement fails.
// On SQLite the migration is one transaction, so nothing persists and the
// version is pending again with its error; on MySQL and MariaDB DDL is not
// transactional, so the earlier statements stay, status shows the migration as
// dirty (exit 1), apply prints the hint, and resolve settles it. Catches: the
// CLI treating MySQL as transactional (reporting a clean rollback while tables
// were created), or SQLite leaving a dirty row.
func TestMigrateCLIFailedMigration(t *testing.T) {
	for _, b := range cliBackends() {
		t.Run(b.name, func(t *testing.T) {
			dsn := b.url(t)
			root := startDialectProject(t, b.dialect)
			for _, args := range [][]string{{"diff", "init"}, {"apply"}, {"new", "--custom", "bad"}} {
				if code, _, errOut := cliRun(t, dsn, args...); code != 0 {
					t.Fatalf("%v: exit %d: %s", args, code, errOut)
				}
			}
			ups, err := filepath.Glob(filepath.Join(root, "migrations", b.dialect, "*_bad.up.sql"))
			if err != nil || len(ups) != 1 {
				t.Fatalf("want one custom up file, got %v %v", ups, err)
			}
			version := strings.SplitN(filepath.Base(ups[0]), "_", 2)[0]
			writeFile(t, ups[0], "CREATE TABLE part_a (id integer PRIMARY KEY);\nCREATE TABLE part_b (id integer PRIMARY KEY);\nINSERT INTO no_such_table VALUES (1);\n")
			if code, _, errOut := cliRun(t, dsn, "hash"); code != 0 {
				t.Fatalf("hash: exit %d: %s", code, errOut)
			}
			code, _, errOut := cliRun(t, dsn, "apply")
			if code != 3 {
				t.Fatalf("apply of a failing migration: exit %d, want 3: %s", code, errOut)
			}
			code, out, _ := cliRun(t, dsn, "status")
			if b.transactional {
				if code != 0 || strings.Contains(out, "dirty") || !strings.Contains(out, "pending "+version) {
					t.Fatalf("SQLite status after a rolled back migration: exit %d, out %q; want pending, not dirty", code, out)
				}
				if strings.Contains(errOut, "do not roll back DDL") {
					t.Fatalf("SQLite printed the MySQL non-transactional hint: %s", errOut)
				}
				return
			}
			if code != 1 || !strings.Contains(out, "dirty "+version) {
				t.Fatalf("MySQL status after a half-applied migration: exit %d, out %q; want dirty", code, out)
			}
			if !strings.Contains(errOut, "do not roll back DDL") {
				t.Fatalf("apply did not print the non-transactional hint: %s", errOut)
			}
			if code, _, errOut = cliRun(t, dsn, "resolve", "--version", version, "--rolled-back"); code != 0 {
				t.Fatalf("resolve: exit %d: %s", code, errOut)
			}
			if code, out, _ = cliRun(t, dsn, "status"); code != 0 || strings.Contains(out, "dirty") {
				t.Fatalf("status after resolve: exit %d, out %q", code, out)
			}
		})
	}
}

// TestMigrateCLIBaselineAllDialects checks that baseline exits 1 on an empty
// database that does not match the snapshot. Catches: baseline comparing the
// wrong database on a dialect, or accepting a mismatch.
func TestMigrateCLIBaselineAllDialects(t *testing.T) {
	for _, b := range cliBackends() {
		t.Run(b.name, func(t *testing.T) {
			dsn := b.url(t)
			root := startDialectProject(t, b.dialect)
			if code, _, errOut := cliRun(t, dsn, "diff", "init"); code != 0 {
				t.Fatalf("diff: exit %d: %s", code, errOut)
			}
			snaps, _ := filepath.Glob(filepath.Join(root, "migrations", b.dialect, "*_init.snapshot.json"))
			if len(snaps) != 1 {
				t.Fatalf("snapshots: %v", snaps)
			}
			version := strings.SplitN(filepath.Base(snaps[0]), "_", 2)[0]
			if code, _, errOut := cliRun(t, dsn, "baseline", "--version", version); code != 1 {
				t.Fatalf("baseline on an empty database: exit %d, want 1: %s", code, errOut)
			}
		})
	}
}

// cliRunStdin runs `momo migrate` with the URL on standard input ("--url -").
func cliRunStdin(t *testing.T, dsn string, args ...string) (code int, out, errOut string) {
	t.Helper()
	old := migrateStdin
	migrateStdin = strings.NewReader(dsn)
	t.Cleanup(func() { migrateStdin = old })
	var o, e bytes.Buffer
	code = runMigrate(&o, &e, append(args, "--url", "-"), func(string) string { return "" })
	return code, o.String(), e.String()
}

// holdSQLiteWriteLock opens the database file and starts a write transaction
// on its own connection, as a long migration of another process would, and
// returns the func that commits it.
func holdSQLiteWriteLock(t *testing.T, path string) (release func()) {
	t.Helper()
	ctx := context.Background()
	db, drv, err := sqlitepure.Open(path, sqlitepure.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pc, err := drv.(momo.Pinner).Pin(ctx)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := pc.Exec(ctx, "BEGIN IMMEDIATE", nil); err != nil {
		_ = pc.Release()
		_ = db.Close()
		t.Fatal(err)
	}
	var once sync.Once
	release = func() {
		once.Do(func() {
			_, _ = pc.Exec(ctx, "COMMIT", nil)
			_ = pc.Release()
			_ = db.Close()
		})
	}
	t.Cleanup(release)
	return release
}

// TestMigrateCLISQLiteLockWait holds the write lock of a SQLite file from a
// second connection and runs apply against it. With a short --lock-wait apply
// gives up with exit 3 naming the busy database; with a long one it keeps retrying past the
// driver's busy timeout until the holder commits, and then applies. Catches: a busy database failing as
// "acquire the migration lock" instead of migrate.ErrLocked, so the engine
// never retried and --lock-wait had no effect on SQLite.
func TestMigrateCLISQLiteLockWait(t *testing.T) {
	path := testdb.SQLitePath(t)
	dsn := "sqlite://" + filepath.ToSlash(path)
	startDialectProject(t, "sqlite")
	if code, _, errOut := cliRun(t, dsn, "diff", "init"); code != 0 {
		t.Fatalf("diff: exit %d: %s", code, errOut)
	}

	release := holdSQLiteWriteLock(t, path)
	code, _, errOut := cliRun(t, dsn, "apply", "--lock-wait", "1s")
	if code != 3 || !strings.Contains(errOut, "busy") || !strings.Contains(errOut, "held by another process") {
		t.Fatalf("apply against a held write lock: exit %d, want 3 naming the busy database: %s", code, errOut)
	}
	release()

	release = holdSQLiteWriteLock(t, path)
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Longer than the driver's 5 s busy timeout: only a retry gets past it.
		time.Sleep(8 * time.Second)
		release()
	}()
	code, out, errOut := cliRun(t, dsn, "apply", "--lock-wait", "30s")
	<-done
	if code != 0 || !strings.Contains(out, "applied") {
		t.Fatalf("apply waiting for the holder: exit %d, out %q, err %q", code, out, errOut)
	}
	if code, out, errOut = cliRun(t, dsn, "status"); code != 0 || !strings.Contains(out, "applied") {
		t.Fatalf("status: exit %d, out %q, err %q", code, out, errOut)
	}
}
