// SPDX-License-Identifier: Apache-2.0

//go:build integration

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo/driver/sqlitepure"
	"github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"
)

// sqliteLegacyDDL is a SQLite database built by hand: a rowid alias and an
// AUTOINCREMENT key, a composite key, a WITHOUT ROWID STRICT table, declared
// types that are not the ones momo writes (VARCHAR, BOOLEAN, DATETIME),
// constant and computed defaults, an enum CHECK, a named CHECK, STORED and
// VIRTUAL generated columns, unique constraints, a composite unique index, a
// descending index, a partial index, an expression index, foreign keys with
// actions, a view and a trigger.
var sqliteLegacyDDL = []string{
	`CREATE TABLE tenants (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		CONSTRAINT tenants_name_key UNIQUE (name))`,
	`CREATE TABLE accounts (
		id INTEGER PRIMARY KEY,
		tenant_id INTEGER NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
		email VARCHAR(200) NOT NULL,
		nickname TEXT,
		active BOOLEAN NOT NULL DEFAULT 1,
		score REAL NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'active', 'closed')),
		email_lower TEXT GENERATED ALWAYS AS (lower(email)) STORED,
		initial TEXT GENERATED ALWAYS AS (substr(email, 1, 1)) VIRTUAL,
		avatar BLOB,
		CONSTRAINT accounts_score_check CHECK (score >= 0))`,
	`CREATE UNIQUE INDEX accounts_email_key ON accounts (tenant_id, email)`,
	`CREATE INDEX accounts_nickname_idx ON accounts (nickname DESC)`,
	`CREATE INDEX accounts_active_idx ON accounts (tenant_id) WHERE active = 1`,
	`CREATE UNIQUE INDEX accounts_email_lower_key ON accounts (lower(email))`,
	`CREATE TABLE memberships (
		account_id INTEGER NOT NULL REFERENCES accounts (id),
		project_id INTEGER NOT NULL,
		role TEXT NOT NULL DEFAULT 'member',
		PRIMARY KEY (account_id, project_id))`,
	`CREATE TABLE profiles (
		id INTEGER PRIMARY KEY,
		account_id INTEGER NOT NULL UNIQUE REFERENCES accounts (id) ON DELETE CASCADE,
		bio TEXT)`,
	`CREATE TABLE settings (k TEXT NOT NULL PRIMARY KEY, v TEXT) WITHOUT ROWID, STRICT`,
	`CREATE VIEW active_accounts AS SELECT id, email FROM accounts WHERE active = 1`,
	`CREATE TRIGGER accounts_ai AFTER INSERT ON accounts BEGIN SELECT 1; END`,
}

// sqliteExec runs statements on the SQLite database file at path.
func sqliteExec(t *testing.T, path string, stmts []string) {
	t.Helper()
	db, _, err := sqlitepure.Open(path, sqlitepure.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, s := range stmts {
		if _, err := db.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
}

// inspectSQLite reads the schema of the SQLite database file at path the way
// the CLI does.
func inspectSQLite(t *testing.T, path string) *schema.Schema {
	t.Helper()
	db, drv, err := sqlitepure.Open(path, sqlitepure.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	e, err := migrate.NewEngine(drv, fstest.MapFS{}, migrate.WithDialect(msqlite.New()))
	if err != nil {
		t.Fatal(err)
	}
	s, err := e.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// schemaLines renders a schema as one JSON line per field, for a readable diff.
func schemaLines(t *testing.T, s *schema.Schema) []string {
	t.Helper()
	b, err := json.MarshalIndent(schema.Normalized(s), "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(b), "\n")
}

// lineDiff lists the lines only one side has.
func lineDiff(a, b []string) string {
	count := map[string]int{}
	for _, l := range a {
		count[l]++
	}
	for _, l := range b {
		count[l]--
	}
	var out []string
	for l, n := range count {
		switch {
		case n > 0:
			out = append(out, "A only: "+strings.TrimSpace(l))
		case n < 0:
			out = append(out, "B only: "+strings.TrimSpace(l))
		}
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

func sqliteMigrate(t *testing.T, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = runMigrate(&o, &e, args, func(string) string { return "" })
	return code, o.String(), e.String()
}

// TestFromDBSQLiteDatabaseRoundTrip adopts a hand-built SQLite database: it
// writes the models from database A, then checks the three ways the models
// can be wrong. The init snapshot and a baseline against A itself must show no
// diff and no drift; a migration the models produce, applied to an empty
// database B, must build the schema of A; and --check must pass on the
// models it wrote and fail once A changes.
//
// Catches: --from-db on SQLite writing models that do not rebuild the database
// they came from (a lost table option, default, index, generated column or
// constraint), a CLI that cannot open a SQLite file, or a --check that does not
// notice a changed database.
func TestFromDBSQLiteDatabaseRoundTrip(t *testing.T) {
	dbDir := t.TempDir()
	a := filepath.Join(dbDir, "a.db")
	sqliteExec(t, a, sqliteLegacyDDL)
	root := copyModule(t, t.TempDir())
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\npackages:\n  - ./models\ndialects: [sqlite]\nintrospect:\n  inverse: true\n")
	chdir(t, root)

	var o, e bytes.Buffer
	if code := Run([]string{"generate", "--from-db", a}, &o, &e); code != 0 {
		t.Fatalf("generate --from-db: exit %d\n%s%s", code, o.String(), e.String())
	}
	out := o.String()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "warn:") || strings.HasPrefix(line, "error:") {
			t.Errorf("the models of this database need no warning: %s", line)
		}
	}
	for _, want := range []string{"accounts_model.go", "memberships_model.go", "profiles_model.go", "settings_model.go", "tenants_model.go"} {
		if _, err := os.Stat(filepath.Join(root, "models", want)); err != nil {
			t.Fatalf("missing %s: %v\n%s", want, err, out)
		}
	}
	inits, _ := filepath.Glob(filepath.Join(root, "migrations", "sqlite", "*_init.up.sql"))
	if len(inits) != 1 {
		t.Fatalf("init migration: %v\n%s", inits, out)
	}
	version := strings.SplitN(filepath.Base(inits[0]), "_", 2)[0]
	o.Reset()
	e.Reset()
	if code := Run([]string{"generate"}, &o, &e); code != 0 {
		t.Fatalf("generate: exit %d\n%s%s", code, o.String(), e.String())
	}

	// The models against the snapshot of A, and A against the snapshot.
	if c, o, e := sqliteMigrate(t, "diff", "after"); c != 0 || !strings.Contains(o, "no changes") {
		t.Fatalf("diff against the init snapshot must be empty: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := sqliteMigrate(t, "--url", a, "baseline", "--version", version); c != 0 {
		t.Fatalf("baseline: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := sqliteMigrate(t, "--url", a, "drift"); c != 0 {
		t.Fatalf("drift on A: exit %d\n%s%s", c, o, e)
	}

	// The models against an empty database: diff, apply, compare with A.
	if c, o, e := sqliteMigrate(t, "--dir", "fresh", "diff", "init"); c != 0 {
		t.Fatalf("diff init: exit %d\n%s%s", c, o, e)
	}
	b := filepath.Join(dbDir, "b.db")
	if c, o, e := sqliteMigrate(t, "--dir", "fresh", "--url", b, "apply"); c != 0 {
		t.Fatalf("apply on an empty database: exit %d\n%s%s", c, o, e)
	}
	if d := lineDiff(schemaLines(t, inspectSQLite(t, a)), schemaLines(t, inspectSQLite(t, b))); d != "" {
		t.Fatalf("the database built from the models differs from the one they came from:\n%s", d)
	}
	if c, o, e := sqliteMigrate(t, "--dir", "fresh", "--url", b, "drift"); c != 0 {
		t.Fatalf("drift on B: exit %d\n%s%s", c, o, e)
	}

	// --check passes on what was written and fails once A changes.
	o.Reset()
	e.Reset()
	if code := Run([]string{"generate", "--from-db", "sqlite://" + a, "--check"}, &o, &e); code != 0 {
		t.Fatalf("--check on a current tree: exit %d\n%s%s", code, o.String(), e.String())
	}
	sqliteExec(t, a, []string{`ALTER TABLE tenants ADD COLUMN note TEXT`})
	o.Reset()
	e.Reset()
	if code := Run([]string{"generate", "--from-db", "file:" + a, "--check"}, &o, &e); code != 1 || !strings.Contains(o.String(), "stale: models/tenants_model.go") {
		t.Fatalf("--check after a change: exit %d, want 1 naming tenants_model.go\n%s%s", code, o.String(), e.String())
	}
}

// Catches: introspect.dbdefault: all stopping at PostgreSQL, or the pointer
// rule being skipped, so a SQLite column with DEFAULT 1 cannot be given false.
func TestFromDBSQLiteDBDefaultAll(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a.db")
	sqliteExec(t, a, sqliteLegacyDDL)
	root := copyModule(t, t.TempDir())
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\npackages:\n  - ./models\ndialects: [sqlite]\nintrospect:\n  dbdefault: all\n")
	chdir(t, root)
	var o, e bytes.Buffer
	if code := Run([]string{"generate", "--from-db", a, "--no-snapshot"}, &o, &e); code != 0 {
		t.Fatalf("generate --from-db: exit %d\n%s%s", code, o.String(), e.String())
	}
	b, err := os.ReadFile(filepath.Join(root, "models", "accounts_model.go")) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"*bool", "type:BOOLEAN,default:1,dbdefault", "type:DATETIME,default:CURRENT_TIMESTAMP,dbdefault"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("accounts_model.go lacks %q:\n%s", want, b)
		}
	}
}
