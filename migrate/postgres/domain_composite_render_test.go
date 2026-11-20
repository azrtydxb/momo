// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

var (
	renderID    = schema.Column{Name: "id", Type: "bigint"}
	renderEmail = schema.Column{Name: "email", Type: "email"}
)

// renderDomainSchema is a schema with the domain email as mutate leaves it and,
// when cols is not empty, a users table of those columns.
func renderDomainSchema(mutate func(*schema.Domain), cols ...schema.Column) *schema.Schema {
	d := schema.Domain{Name: "email", Type: "text"}
	if mutate != nil {
		mutate(&d)
	}
	s := &schema.Schema{Dialect: "postgres", Domains: []schema.Domain{d}, Tables: []schema.Table{}}
	if len(cols) > 0 {
		s.Tables = []schema.Table{{Name: "users", Columns: cols, PrimaryKey: []string{"id"}}}
	}
	return s
}

// inOrder fails unless each of want is in sql, one after the other.
func inOrder(t *testing.T, sql string, want ...string) {
	t.Helper()
	at := 0
	for _, w := range want {
		i := strings.Index(sql[at:], w)
		if i < 0 {
			t.Errorf("%q is missing, or out of order, after offset %d in:\n%s", w, at, sql)
			return
		}
		at += i + len(w)
	}
}

// Catches: a domain written with its clauses in an order PostgreSQL rejects
// (the default comes before NOT NULL and the constraints), a check without its
// name, a domain created after the table that uses it, and a column that
// does not name the domain.
func TestRenderCreateDomain(t *testing.T) {
	to := renderDomainSchema(func(d *schema.Domain) {
		d.Default, d.NotNull = "''", true
		d.Checks = []schema.DomainCheck{{Name: "email_shape", Expr: "VALUE <> ''"}, {Name: "email_at", Expr: "VALUE ~ '@'"}}
	}, renderID, renderEmail)
	sql := renderDiff(t, &schema.Schema{Dialect: "postgres"}, to)
	inOrder(t, sql,
		`CREATE DOMAIN "email" AS text DEFAULT '' NOT NULL CONSTRAINT "email_shape" CHECK (VALUE <> '') CONSTRAINT "email_at" CHECK (VALUE ~ '@')`,
		`CREATE TABLE "users"`,
		`"email" email`,
	)

	qualified := renderDomainSchema(func(d *schema.Domain) { d.Schema = "app" })
	qualified.Schemas = []schema.Namespace{{Name: "app"}}
	if sql := renderDiff(t, &schema.Schema{Dialect: "postgres"}, qualified); !strings.Contains(sql, `CREATE DOMAIN "app"."email" AS text`) {
		t.Errorf("a domain in a schema is not qualified:\n%s", sql)
	}
}

// Catches: an ALTER DOMAIN that leaves a changed check in place or adds the
// new one before the old one is gone (a name clash), forgets to drop a default
// or NOT NULL, or is written for a domain whose base type changed.
func TestRenderAlterDomain(t *testing.T) {
	from := renderDomainSchema(func(d *schema.Domain) {
		d.Checks = []schema.DomainCheck{{Name: "email_shape", Expr: "VALUE <> ''"}, {Name: "email_old", Expr: "VALUE <> 'x'"}}
	})
	to := renderDomainSchema(func(d *schema.Domain) {
		d.Default, d.NotNull = "'a@b'", true
		d.Checks = []schema.DomainCheck{{Name: "email_shape", Expr: "VALUE <> 'y'"}, {Name: "email_new", Expr: "VALUE ~ '@'"}}
	})
	sql := renderDiff(t, from, to)
	inOrder(t, sql,
		`ALTER DOMAIN "email" DROP CONSTRAINT "email_shape"`,
		`ALTER DOMAIN "email" DROP CONSTRAINT "email_old"`,
		`ALTER DOMAIN "email" SET DEFAULT 'a@b'`,
		`ALTER DOMAIN "email" SET NOT NULL`,
		`ALTER DOMAIN "email" ADD CONSTRAINT "email_shape" CHECK (VALUE <> 'y')`,
		`ALTER DOMAIN "email" ADD CONSTRAINT "email_new" CHECK (VALUE ~ '@')`,
	)
	back := renderDiff(t, to, renderDomainSchema(nil))
	inOrder(t, back,
		`ALTER DOMAIN "email" DROP CONSTRAINT "email_new"`,
		`ALTER DOMAIN "email" DROP DEFAULT`,
		`ALTER DOMAIN "email" DROP NOT NULL`,
	)
	if strings.Contains(sql, "DROP DOMAIN") || strings.Contains(back, "DROP DOMAIN") {
		t.Errorf("an ALTER-able change recreated the domain:\n%s\n%s", sql, back)
	}
}

// Catches: a base type change rendered as ALTER DOMAIN, which has no such
// clause; a domain dropped while a column still uses it; and a column that is
// not changed back to the domain after it is created again.
func TestRenderRecreateDomain(t *testing.T) {
	from := renderDomainSchema(func(d *schema.Domain) { d.Checks = []schema.DomainCheck{{Name: "email_shape", Expr: "VALUE <> ''"}} }, renderID, renderEmail)
	to := renderDomainSchema(func(d *schema.Domain) {
		d.Type = "character varying(255)"
		d.Checks = []schema.DomainCheck{{Name: "email_shape", Expr: "VALUE <> ''"}}
	}, renderID, renderEmail)
	sql := renderDiff(t, from, to)
	inOrder(t, sql,
		`ALTER TABLE "users" ALTER COLUMN "email" TYPE text USING "email"::text`,
		`DROP DOMAIN "email"`,
		`CREATE DOMAIN "email" AS character varying(255) CONSTRAINT "email_shape" CHECK (VALUE <> '')`,
		`ALTER TABLE "users" ALTER COLUMN "email" TYPE "email" USING "email"::"email"`,
	)
}

// Catches: a dropped domain rendered as something other than DROP DOMAIN, and a
// domain that outlives the table that uses it.
func TestRenderDropDomain(t *testing.T) {
	sql := renderDiff(t, renderDomainSchema(nil, renderID, renderEmail), &schema.Schema{Dialect: "postgres", Tables: []schema.Table{}})
	inOrder(t, sql, `DROP TABLE "users"`, `DROP DOMAIN "email"`)
}

func renderCompositeSchema(mutate func(*schema.Composite)) *schema.Schema {
	c := schema.Composite{Name: "address", Attributes: []schema.CompositeAttr{{Name: "street", Type: "text"}, {Name: "zip", Type: "text"}}}
	if mutate != nil {
		mutate(&c)
	}
	return &schema.Schema{Dialect: "postgres", Composites: []schema.Composite{c}, Tables: []schema.Table{{
		Name:       "customers",
		Columns:    []schema.Column{renderID, {Name: "home", Type: "address", Nullable: true}},
		PrimaryKey: []string{"id"},
	}}}
}

// Catches: a composite type created after the table that uses it, attributes
// written in another order than declared, and an attribute type left out.
func TestRenderCreateComposite(t *testing.T) {
	sql := renderDiff(t, &schema.Schema{Dialect: "postgres"}, renderCompositeSchema(nil))
	inOrder(t, sql,
		`CREATE TYPE "address" AS ("street" text, "zip" text)`,
		`CREATE TABLE "customers"`,
		`"home" address`,
	)
}

// Catches: an ALTER TYPE that renders an attribute drop, retype or add in an
// order that clashes, adds an attribute that was already there, or renders no
// statement for a change.
func TestRenderAlterComposite(t *testing.T) {
	from := &schema.Schema{Dialect: "postgres", Composites: []schema.Composite{{Name: "address", Attributes: []schema.CompositeAttr{
		{Name: "street", Type: "text"}, {Name: "zip", Type: "text"}, {Name: "old", Type: "text"},
	}}}, Tables: []schema.Table{}}
	to := &schema.Schema{Dialect: "postgres", Composites: []schema.Composite{{Name: "address", Attributes: []schema.CompositeAttr{
		{Name: "street", Type: "text"}, {Name: "zip", Type: "character varying(10)"}, {Name: "city", Type: "text"}, {Name: "country", Type: "text"},
	}}}, Tables: []schema.Table{}}
	sql := renderDiff(t, from, to)
	inOrder(t, sql,
		`ALTER TYPE "address" DROP ATTRIBUTE "old"`,
		`ALTER TYPE "address" ALTER ATTRIBUTE "zip" TYPE character varying(10)`,
		`ALTER TYPE "address" ADD ATTRIBUTE "city" text`,
		`ALTER TYPE "address" ADD ATTRIBUTE "country" text`,
	)
	if n := strings.Count(sql, "ALTER TYPE"); n != 4 {
		t.Errorf("%d ALTER TYPE statements, want 4:\n%s", n, sql)
	}
	drop := renderDiff(t, to, &schema.Schema{Dialect: "postgres", Tables: []schema.Table{}})
	if !strings.Contains(drop, `DROP TYPE "address"`) {
		t.Errorf("a dropped composite type is not DROP TYPE:\n%s", drop)
	}
}

// Catches: a domain or composite type with a name that PostgreSQL would fold
// to another, or without what it needs, rendered as SQL that creates a
// different object or fails at apply.
func TestRenderRefusesInvalidDomainsAndComposites(t *testing.T) {
	d := mpg.New()
	for name, c := range map[string]migrate.Change{
		"domain name with a capital":   {Kind: migrate.CreateDomain, Name: "Email", After: &schema.Domain{Name: "Email", Type: "text"}},
		"domain without a base type":   {Kind: migrate.CreateDomain, Name: "email", After: &schema.Domain{Name: "email"}},
		"domain check without a name":  {Kind: migrate.CreateDomain, Name: "email", After: &schema.Domain{Name: "email", Type: "text", Checks: []schema.DomainCheck{{Expr: "VALUE <> ''"}}}},
		"composite without attributes": {Kind: migrate.CreateComposite, Name: "address", After: &schema.Composite{Name: "address"}},
		"composite attribute no type":  {Kind: migrate.CreateComposite, Name: "address", After: &schema.Composite{Name: "address", Attributes: []schema.CompositeAttr{{Name: "street"}}}},
		"payload of the wrong type":    {Kind: migrate.CreateDomain, Name: "email", After: &schema.Composite{Name: "email"}},
	} {
		if stmts, err := d.Render([]migrate.Change{c}); err == nil {
			t.Errorf("%s: rendered %+v", name, stmts)
		}
	}
}
