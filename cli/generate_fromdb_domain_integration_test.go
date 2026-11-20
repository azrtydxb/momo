// SPDX-License-Identifier: Apache-2.0

//go:build integration

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	testdb "github.com/azrtydxb/momo/internal/testdb"
)

// domainDDL is a database with two domains, two composite types and a table
// that uses one of each kind of column: a domain over text with a default, NOT
// NULL and a check, a domain over varchar, and a composite type. The type spare
// has no column.
var domainDDL = []string{
	`CREATE DOMAIN email AS text NOT NULL DEFAULT '' CONSTRAINT email_shape CHECK (VALUE <> '')`,
	`CREATE DOMAIN short_code AS varchar(12) CONSTRAINT short_code_len CHECK (length(VALUE) >= 3)`,
	`CREATE TYPE address AS (street text, zip text, floor integer)`,
	`CREATE TYPE spare AS (a text)`,
	`CREATE TABLE customers (id bigint PRIMARY KEY, email email, code short_code, home address)`,
}

// domainSetup adopts domainDDL with generate --from-db and returns the module
// root, the database URL and the init version.
func domainSetup(t *testing.T) (root, dsn, version string) {
	t.Helper()
	dsn = testdb.PostgresURL(t)
	execAll(t, dsn, domainDDL)
	root = copyModule(t, t.TempDir())
	writeFile(t, filepath.Join(root, "momo.yaml"), `version: 1
packages:
  - ./models
dialects: [postgres]
migrate:
  dir:
    postgres: migrations/postgres
  url_env:
    postgres: MOMO_TEST_URL
  exclude: [vector, pgcrypto]
`)
	chdir(t, root)
	t.Setenv("MOMO_TEST_URL", dsn)
	if code, out, errOut := fromDB(t); code != 0 {
		t.Fatalf("generate --from-db: exit %d\n%s%s", code, out, errOut)
	}
	files, _ := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*_init.up.sql"))
	if len(files) != 1 {
		t.Fatalf("init migration: %v", files)
	}
	var o, e bytes.Buffer
	if c := Run([]string{"generate"}, &o, &e); c != 0 {
		t.Fatalf("generate: exit %d\n%s%s", c, o.String(), e.String())
	}
	return root, dsn, strings.SplitN(filepath.Base(files[0]), "_", 2)[0]
}

// TestGenerateFromDBDomainsAndComposites adopts a database with domains and
// composite types end to end: the models declare them and the init migration
// creates them before the table, the init migration applies on an empty
// database, and the migrated database compares equal to the source one (no
// diff, no drift, a clean baseline).
//
// Catches: a domain or composite type lost between inspection and the init
// migration, so the init migration fails on an empty database with `type
// "email" does not exist` while drift and baseline on the source database look
// clean; a column of a domain written as the domain's own type instead of the
// type of its base; and a composite column written without the struct that
// reads it.
func TestGenerateFromDBDomainsAndComposites(t *testing.T) {
	root, dsn, version := domainSetup(t)

	t.Run("empty database", func(t *testing.T) {
		fresh := testdb.PostgresURL(t)
		if c, o, e := cliRun(t, fresh, "apply"); c != 0 {
			t.Fatalf("apply on an empty database: exit %d\n%s%s", c, o, e)
		}
		if c, o, e := cliRun(t, fresh, "diff", "after"); c != 0 || !strings.Contains(o, "no changes") {
			t.Fatalf("diff on the migrated database must be empty: exit %d\n%s%s", c, o, e)
		}
		if c, o, e := cliRun(t, fresh, "drift"); c != 0 {
			t.Fatalf("drift on the migrated database: exit %d\n%s%s", c, o, e)
		}
	})

	models := readText(t, filepath.Join(root, "models", "customers_model.go"))
	for _, want := range []string{
		`c.Domain("email").As("text").NotNull().Default("''::text").Check("email_shape", `,
		`c.Domain("short_code").As("character varying(12)").Check("short_code_len", `,
		`c.Composite("address").Attr("street", "text").Attr("zip", "text").Attr("floor", "integer")`,
		`c.Composite("spare").Attr("a", "text")`,
		"type Address struct",
		"types.Composite[Address]",
		`momo:"type:email"`,
		`momo:"type:short_code"`,
	} {
		if !strings.Contains(models, want) {
			t.Fatalf("the models lack %s:\n%s", want, models)
		}
	}
	initSQL := readText(t, filepath.Join(root, "migrations", "postgres", version+"_init.up.sql"))
	tableAt := strings.Index(initSQL, `CREATE TABLE "customers"`)
	for _, create := range []string{`CREATE DOMAIN "email"`, `CREATE DOMAIN "short_code"`, `CREATE TYPE "address"`} {
		if at := strings.Index(initSQL, create); at < 0 || tableAt < 0 || at > tableAt {
			t.Fatalf("the init migration must have %s before the table that uses it:\n%s", create, initSQL)
		}
	}

	// The source database adopts the migration without a difference.
	if c, o, e := cliRun(t, dsn, "baseline", "--version", version); c != 0 {
		t.Fatalf("baseline: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, dsn, "diff", "after"); c != 0 || !strings.Contains(o, "no changes") {
		t.Fatalf("diff after baseline must be empty: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, dsn, "drift"); c != 0 {
		t.Fatalf("drift: exit %d\n%s%s", c, o, e)
	}
}

// TestDomainAndCompositeAlterAndDrift changes a domain and a composite type in
// the model, which is one migration with an ALTER DOMAIN and an ALTER TYPE and
// no drop, and changes a domain in the database, which drift reports.
//
// Catches: a change of a domain rendered as a drop and a create that fails
// while a column uses it, a composite attribute that is not added, and drift
// that cannot see a domain.
func TestDomainAndCompositeAlterAndDrift(t *testing.T) {
	root, dsn, version := domainSetup(t)
	if c, o, e := cliRun(t, dsn, "baseline", "--version", version); c != 0 {
		t.Fatalf("baseline: exit %d\n%s%s", c, o, e)
	}

	// A change made outside momo is drift.
	execAll(t, dsn, []string{`ALTER DOMAIN email DROP CONSTRAINT email_shape`})
	c, out, _ := cliRun(t, dsn, "drift")
	if c != 1 || !strings.Contains(out, "changed: domain email") {
		t.Fatalf("drift must report the altered domain: exit %d\n%s", c, out)
	}
	execAll(t, dsn, []string{`ALTER DOMAIN email ADD CONSTRAINT email_shape CHECK (VALUE <> '')`})
	if c, o, e := cliRun(t, dsn, "drift"); c != 0 {
		t.Fatalf("drift after restoring the domain: exit %d\n%s%s", c, o, e)
	}

	// A change made in the model is one ALTER DOMAIN and one ALTER TYPE.
	path := filepath.Join(root, "models", "customers_model.go")
	src := readText(t, path)
	edited := strings.Replace(src, `c.Domain("email")`, `c.Domain("email").Check("email_at", "VALUE ~ '@'")`, 1)
	edited = strings.Replace(edited, `c.Composite("spare").Attr("a", "text")`, `c.Composite("spare").Attr("a", "text").Attr("b", "integer")`, 1)
	if edited == src || !strings.Contains(edited, `Attr("b", "integer")`) {
		t.Fatalf("the model has no declaration to edit:\n%s", src)
	}
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, o, e := cliRun(t, dsn, "diff", "edit_types"); c != 0 {
		t.Fatalf("diff: exit %d\n%s%s", c, o, e)
	}
	ups, _ := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*_edit_types.up.sql"))
	if len(ups) != 1 {
		t.Fatalf("want one migration for the change, got %v", ups)
	}
	body := readText(t, ups[0])
	for _, want := range []string{`ALTER DOMAIN "email" ADD CONSTRAINT "email_at" CHECK (VALUE ~ '@')`, `ALTER TYPE "spare" ADD ATTRIBUTE "b" integer`} {
		if !strings.Contains(body, want) {
			t.Fatalf("the migration lacks %s:\n%s", want, body)
		}
	}
	for _, bad := range []string{"CREATE DOMAIN", "DROP DOMAIN", "CREATE TYPE", "DROP TYPE"} {
		if strings.Contains(body, bad) {
			t.Fatalf("the migration has %s, want ALTER statements only:\n%s", bad, body)
		}
	}
	if c, o, e := cliRun(t, dsn, "apply"); c != 0 {
		t.Fatalf("apply: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, dsn, "drift"); c != 0 {
		t.Fatalf("drift after apply: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, dsn, "diff", "again"); c != 0 || !strings.Contains(o, "no changes") {
		t.Fatalf("diff after apply must be empty: exit %d\n%s%s", c, o, e)
	}
}
