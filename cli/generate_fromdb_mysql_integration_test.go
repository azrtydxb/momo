// SPDX-License-Identifier: Apache-2.0

//go:build integration

package cli

import (
	"bytes"
	"context"
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/mysqldriver"
	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/config"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
	gomysql "github.com/go-sql-driver/mysql"
)

// mysqlLegacyDDL is a MySQL schema built by hand, valid on MySQL 8.4, MySQL 9.7
// and MariaDB 11.8: unsigned auto-increment keys, integer widths, decimal,
// float and double, varchar, text of every size, blob and binary types, bit,
// enum (one value with a comma and a quote) and set, json, date, time, year,
// datetime with and without precision, timestamp, tinyint(1), NOT NULL and
// constant defaults (a string with a comma, an empty string, an expression),
// ON UPDATE, unique keys, plain, composite, DESC and prefix indexes, named and
// unnamed checks, foreign keys with and without actions (one to a unique column
// that is not the primary key), and STORED and VIRTUAL generated columns.
var mysqlLegacyDDL = []string{
	`CREATE TABLE accounts (
		id bigint unsigned NOT NULL AUTO_INCREMENT,
		email varchar(200) NOT NULL,
		name varchar(100) NULL,
		bio text NULL,
		notes mediumtext NULL,
		avatar blob NULL,
		icon tinyblob NULL,
		code bit(8) NULL,
		raw binary(4) NULL,
		token varbinary(16) NULL,
		active tinyint(1) NOT NULL DEFAULT 1,
		level tinyint unsigned NOT NULL DEFAULT 0,
		rank_no int NOT NULL DEFAULT 5,
		score smallint NULL,
		big bigint NOT NULL DEFAULT 0,
		balance decimal(12,2) NOT NULL DEFAULT 0.00,
		ratio double NULL,
		weight float NULL,
		status enum('new','active','closed') NOT NULL DEFAULT 'new',
		pick enum('a,b','it''s','x') NOT NULL DEFAULT 'a,b',
		perms set('r','w','x') NULL,
		settings json NULL,
		born date NULL,
		at_time time NULL,
		since year NULL,
		checked datetime NULL,
		created_at datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		touched datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		seen_at timestamp NULL DEFAULT NULL,
		label varchar(20) NOT NULL DEFAULT '',
		uid varchar(36) NOT NULL DEFAULT (uuid()),
		email_lower varchar(200) GENERATED ALWAYS AS (lower(email)) STORED,
		name_len int GENERATED ALWAYS AS (char_length(name)) VIRTUAL,
		PRIMARY KEY (id),
		UNIQUE KEY accounts_email_key (email),
		KEY accounts_name_idx (name),
		KEY accounts_born_level (born, level DESC),
		KEY accounts_bio_prefix (bio(20)),
		CONSTRAINT accounts_level_chk CHECK (level < 100),
		CONSTRAINT accounts_balance_chk CHECK (balance >= 0))`,
	`CREATE TABLE projects (
		id int NOT NULL AUTO_INCREMENT,
		account_id bigint unsigned NOT NULL,
		owner_id bigint unsigned NULL,
		owner_email varchar(200) NULL,
		title varchar(150) NOT NULL,
		body longtext NULL,
		PRIMARY KEY (id),
		KEY projects_title_idx (title),
		UNIQUE KEY projects_account_title (account_id, title),
		CONSTRAINT projects_account_fk FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE,
		CONSTRAINT projects_owner_fk FOREIGN KEY (owner_id) REFERENCES accounts (id) ON DELETE SET NULL ON UPDATE CASCADE,
		CONSTRAINT projects_owner_email_fk FOREIGN KEY (owner_email) REFERENCES accounts (email))`,
	`CREATE TABLE tasks (
		id bigint NOT NULL AUTO_INCREMENT,
		project_id int NOT NULL,
		done tinyint(1) NOT NULL DEFAULT 0,
		PRIMARY KEY (id),
		CHECK (done IN (0, 1)),
		FOREIGN KEY (project_id) REFERENCES projects (id))`,
	`CREATE TABLE rowtypes (
		id bigint unsigned NOT NULL AUTO_INCREMENT,
		big bigint unsigned NOT NULL,
		tm time NOT NULL,
		yr year NOT NULL,
		flag tinyint(1) NOT NULL,
		bits bit(8) NOT NULL,
		bin binary(4) NOT NULL,
		txt mediumtext NOT NULL,
		small smallint NOT NULL,
		PRIMARY KEY (id))`,
}

type mysqlTarget struct {
	name    string
	flavor  dialect.Flavor
	mariadb bool
	url     func(testing.TB) string
}

var mysqlTargets = []mysqlTarget{
	{"mysql-8.4", dialect.FlavorMySQL, false, testdb.MySQL84URL},
	{"mysql-9.7", dialect.FlavorMySQL, false, testdb.MySQL97URL},
	{"mariadb-11.8", dialect.FlavorMariaDB, true, testdb.MariaDBURL},
}

// emptyMySQLDatabase creates a second database next to the one dsn names, and
// returns its DSN. It is dropped when t ends.
func emptyMySQLDatabase(t *testing.T, dsn string) string {
	t.Helper()
	cfg, err := gomysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := cfg.DBName + "_b"
	cfg.DBName = ""
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+name+"`")
		_ = admin.Close()
	})
	if _, err := admin.ExecContext(context.Background(), "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	return cfg.FormatDSN()
}

// mysqlEngine opens dsn and returns an engine on it, whose migration directory
// is dir.
func mysqlEngine(t *testing.T, dsn string, flavor dialect.Flavor, dir string) *migrate.Engine {
	t.Helper()
	sdb, drv, err := mysqldriver.Open(dsn, flavor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	return migratetest.NewEngine(t, mmysql.New(), drv, os.DirFS(dir))
}

// mysqlProject builds a database on the target from ddl and a project that
// reads it with --from-db. It returns the project root, the DSN and a handle on
// the database.
func mysqlProject(t *testing.T, tg mysqlTarget, ddl []string) (root, dsn string, sdb *sql.DB) {
	t.Helper()
	dsn = tg.url(t)
	sdb, _, err := mysqldriver.Open(dsn, tg.flavor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	for _, s := range ddl {
		if _, err := sdb.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	root = copyModule(t, t.TempDir())
	writeFile(t, filepath.Join(root, "momo.yaml"), `version: 1
packages:
  - ./models
dialects: [mysql]
migrate:
  dir:
    mysql: migrations/mysql
introspect:
  inverse: true
`)
	chdir(t, root)
	t.Setenv("MOMO_TEST_URL", dsn)
	return root, dsn, sdb
}

func readModel(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "models", name)) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// dropIndex removes the index from the inspected schema and reports whether it
// was there.
func dropIndex(s *schema.Schema, table, name string) bool {
	for i := range s.Tables {
		if s.Tables[i].Name != table {
			continue
		}
		var kept []schema.Index
		found := false
		for _, ix := range s.Tables[i].Indexes {
			if ix.Name == name {
				found = true
				continue
			}
			kept = append(kept, ix)
		}
		s.Tables[i].Indexes = kept
		return found
	}
	return false
}

// goTypes are the Go types the row-level check can scan into.
var goTypes = map[string]reflect.Type{
	"uint64": reflect.TypeOf(uint64(0)), "int16": reflect.TypeOf(int16(0)), "string": reflect.TypeOf(""),
	"bool": reflect.TypeOf(false), "[]byte": reflect.TypeOf([]byte(nil)),
}

// TestGenerateFromDBMySQLRoundTrip is the adoption path on MySQL 8.4, MySQL 9.7
// and MariaDB 11.8: inspect a database built with raw DDL, write the models,
// load them, diff them against an empty database, apply, and compare the two
// inspections.
//
// Catches, as far as schema.Equal sees: models that do not declare the schema
// they were read from, so a database rebuilt from them differs in a column
// type, unsigned, decimal scale, enum value, default, generated column kind,
// index order, check or foreign key (also one to a non-primary unique column).
// It also asserts what is reported and left out: the ON UPDATE warning, the
// prefix index warning, the dbdefault rule per flavor and the tinyint(1) note.
// It cannot see what schema.Equal ignores (see docs/mysql.md).
func TestGenerateFromDBMySQLRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, tg := range mysqlTargets {
		t.Run(tg.name, func(t *testing.T) {
			root, dsnA, _ := mysqlProject(t, tg, mysqlLegacyDDL)

			code, out, errOut := fromDB(t)
			if code != 0 {
				t.Fatalf("generate --from-db: exit %d\n%s%s", code, out, errOut)
			}
			if code, out2, errOut2 := fromDB(t, "--check"); code != 0 {
				t.Fatalf("--check on a current tree: exit %d\n%s%s", code, out2, errOut2)
			}
			for _, want := range []string{"accounts_model.go", "projects_model.go", "tasks_model.go", "rowtypes_model.go"} {
				if _, err := os.Stat(filepath.Join(root, "models", want)); err != nil {
					t.Fatalf("missing %s: %v\n%s", want, err, out)
				}
			}
			for _, want := range []string{
				"accounts.touched: ON UPDATE is not modeled",
				"accounts index accounts_bio_prefix",
				"accounts.active: tinyint(1) is a Go bool",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("the report lacks %q:\n%s", want, out)
				}
			}
			projects := readModel(t, root, "projects_model.go")
			if !strings.Contains(projects, "belongs_to") || !strings.Contains(projects, `c.ForeignKey("projects_owner_email_fk"`) {
				t.Errorf("projects lacks the belongs_to relation or the foreign key to the unique column:\n%s", projects)
			}
			accounts := readModel(t, root, "accounts_model.go")
			for _, want := range []string{"has_many", "virtual", "type:enum('a,b','it''s','x')", "type:bit(8)", "type:year", `c.Default(&m.Pick, "'a,b'")`, "default:''", "default:uuid()"} {
				if !strings.Contains(accounts, want) {
					t.Errorf("accounts lacks %q:\n%s", want, accounts)
				}
			}
			if tg.mariadb {
				if !strings.Contains(accounts, "default:1,dbdefault") || strings.Contains(out, "a zero value is written instead of the column default") {
					t.Errorf("MariaDB has RETURNING: the non-zero constant default must get dbdefault and no note\n%s\n%s", accounts, out)
				}
			} else {
				if strings.Contains(accounts, "dbdefault") || !strings.Contains(out, "accounts.active: the default 1 is not tagged dbdefault") {
					t.Errorf("MySQL has no RETURNING: no dbdefault, and a note instead\n%s\n%s", accounts, out)
				}
			}
			if files, _ := filepath.Glob(filepath.Join(root, "migrations", "mysql", "*_init.up.sql")); len(files) != 1 {
				t.Fatalf("init migration: %v\n%s", files, out)
			}

			file, _, err := loadConfigFile("", root)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.ToConfig(file, root)
			if err != nil {
				t.Fatal(err)
			}
			g, diags, err := gen.Load([]string{"./models"}, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) > 0 {
				t.Fatalf("the generated models do not load: %v", diags)
			}

			engA := mysqlEngine(t, dsnA, tg.flavor, t.TempDir())
			liveA, err := engA.Inspect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// A prefix index has no declaration: it is reported and left out of
			// the models, so it is also left out of the comparison.
			if !dropIndex(liveA, "accounts", "accounts_bio_prefix") {
				t.Fatal("the inspector did not read the prefix index")
			}
			if changes, _, err := migrate.Diff(liveA, g.Schema, migrate.DiffOptions{}); err != nil || len(changes) != 0 {
				t.Fatalf("the models differ from the database they were read from (err %v): %+v", err, changes)
			}

			dsnB := emptyMySQLDatabase(t, dsnA)
			dirB := t.TempDir()
			engB := mysqlEngine(t, dsnB, tg.flavor, dirB)
			m, err := engB.Diff(ctx, g.Schema, "roundtrip", migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if m == nil {
				t.Fatal("the models against an empty database produced no migration")
			}
			for name, body := range m.Files {
				if err := os.WriteFile(filepath.Join(dirB, name), body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := engB.Apply(ctx, migrate.ApplyOptions{}); err != nil {
				t.Fatalf("apply: %v", err)
			}
			liveB, err := engB.Inspect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !schema.Equal(liveA, liveB) {
				t.Fatalf("the rebuilt database differs from the original:\n%s", migratetest.Diff(liveB, liveA))
			}
			if changes, _, err := migrate.Diff(liveB, liveA, migrate.DiffOptions{}); err != nil || len(changes) != 0 {
				t.Fatalf("drift between the rebuilt and the original database (err %v): %+v", err, changes)
			}
			if again, err := engB.Diff(ctx, g.Schema, "noop", migrate.DiffOptions{}); err != nil || again != nil {
				t.Fatalf("a second diff of the models produced a migration (err %v): %+v", err, again)
			}
		})
	}
}

// TestGenerateFromDBMySQLRowTypes writes a row with the extreme values of the
// types the generator chose and reads it back, on each server.
//
// Catches: a Go type chosen for a column that cannot hold what the column
// stores: bigint unsigned above math.MaxInt64, TIME beyond 24 hours, year, bit,
// binary padding, tinyint(1). It scans into the Go types the generated model
// declares for the rowtypes table, not through momo's insert and select
// builders, which need generated code that MySQL models do not have yet.
func TestGenerateFromDBMySQLRowTypes(t *testing.T) {
	ctx := context.Background()
	for _, tg := range mysqlTargets {
		t.Run(tg.name, func(t *testing.T) {
			root, _, sdb := mysqlProject(t, tg, mysqlLegacyDDL[3:])
			if code, out, errOut := fromDB(t); code != 0 {
				t.Fatalf("generate --from-db: exit %d\n%s%s", code, out, errOut)
			}
			file, _, err := loadConfigFile("", root)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.ToConfig(file, root)
			if err != nil {
				t.Fatal(err)
			}
			g, diags, err := gen.Load([]string{"./models"}, cfg)
			if err != nil || len(diags) > 0 {
				t.Fatalf("the generated models do not load: %v %v", err, diags)
			}
			var cols []string
			var dests []any
			for _, m := range g.Models {
				if m.Table != "rowtypes" {
					continue
				}
				for _, f := range m.Fields {
					typ, ok := goTypes[f.GoType]
					if !ok {
						t.Fatalf("column %s has the Go type %s, which the check cannot scan", f.Column, f.GoType)
					}
					cols = append(cols, "`"+f.Column+"`")
					dests = append(dests, reflect.New(typ).Interface())
				}
			}
			if len(cols) != 9 {
				t.Fatalf("columns of rowtypes = %v, want 9", cols)
			}
			if _, err := sdb.ExecContext(ctx, "INSERT INTO rowtypes (big, tm, yr, flag, bits, bin, txt, small) VALUES (18446744073709551615, '838:59:59', 2155, 1, b'10101010', 'ab', 'text', -5)"); err != nil {
				t.Fatal(err)
			}
			if err := sdb.QueryRowContext(ctx, "SELECT "+strings.Join(cols, ", ")+" FROM rowtypes").Scan(dests...); err != nil {
				t.Fatalf("scan into the generated Go types: %v", err)
			}
			got := map[string]any{}
			for i, c := range cols {
				got[strings.Trim(c, "`")] = reflect.ValueOf(dests[i]).Elem().Interface()
			}
			if got["big"] != uint64(math.MaxUint64) || got["tm"] != "838:59:59" || got["yr"] != int16(2155) || got["flag"] != true || got["small"] != int16(-5) || got["txt"] != "text" {
				t.Errorf("row read back as %+v", got)
			}
			if !bytes.Equal(got["bits"].([]byte), []byte{0xAA}) || !bytes.Equal(got["bin"].([]byte), []byte{'a', 'b', 0, 0}) {
				t.Errorf("bit and binary read back as %v and %v", got["bits"], got["bin"])
			}
		})
	}
}

// TestGenerateFromDBMySQLPrefixPrimaryKey: a primary key on a column prefix is
// refused with a message, and nothing is written.
//
// Catches: the inspector reading PRIMARY KEY (name(10)) as the whole column, so
// the models declare a key the database does not have.
func TestGenerateFromDBMySQLPrefixPrimaryKey(t *testing.T) {
	for _, tg := range mysqlTargets {
		t.Run(tg.name, func(t *testing.T) {
			root, _, _ := mysqlProject(t, tg, []string{`CREATE TABLE pfx (name varchar(100) NOT NULL, PRIMARY KEY (name(10)))`})
			code, out, errOut := fromDB(t)
			if code == 0 || !strings.Contains(out+errOut, "pfx.name: the primary key uses a column prefix") {
				t.Fatalf("exit %d, want a refusal naming the prefix primary key\n%s%s", code, out, errOut)
			}
			if entries, _ := filepath.Glob(filepath.Join(root, "models", "*")); len(entries) != 0 {
				t.Fatalf("model files were left behind: %v", entries)
			}
		})
	}
}
