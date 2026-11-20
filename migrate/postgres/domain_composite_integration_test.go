// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// dcmpSchema is a customers table with a column of a domain over text, one of a
// domain over varchar and one of a composite type, and the composite type spare
// that no column uses. mutate changes the schema before it is returned. The test
// database has pgvector installed, so the inspected schema lists it and the
// declared one must too.
func dcmpSchema(mutate ...func(*schema.Schema)) *schema.Schema {
	s := &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Domains: []schema.Domain{
			{
				Name: "email", Type: "text", NotNull: true, Default: "'nobody@example.com'",
				Checks: []schema.DomainCheck{{Name: "email_nonempty", Expr: "VALUE <> ''"}, {Name: "email_shape", Expr: "VALUE ~ '@'"}},
			},
			{Name: "short_code", Type: "character varying(12)"},
		},
		Composites: []schema.Composite{
			{Name: "address", Attributes: []schema.CompositeAttr{{Name: "street", Type: "text"}, {Name: "zip", Type: "text"}, {Name: "floor", Type: "integer"}}},
			{Name: "spare", Attributes: []schema.CompositeAttr{{Name: "a", Type: "text"}}},
		},
		Tables: []schema.Table{{
			Name: "customers",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint"},
				{Name: "email", Type: "email"},
				{Name: "code", Type: "short_code", Nullable: true},
				{Name: "home", Type: "address", Nullable: true},
			},
			PrimaryKey: []string{"id"},
		}},
	}
	for _, m := range mutate {
		m(s)
	}
	return s
}

// Catches: a domain or a composite type lost in creation or in inspection, so a
// database built from the model has a column whose type does not exist, or reads
// back as a different model; a default, NOT NULL or check that PostgreSQL
// rewrites and that then never compares equal; and a column that reads back
// with a type other than the one the model names.
func TestPostgresDomainsAndCompositesRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	got := roundTrip(t, db, dcmpSchema())
	if len(got.Domains) != 2 || len(got.Composites) != 2 {
		t.Fatalf("inspection found %d domains and %d composite types, want 2 and 2 (the row type of a table is not a composite type)", len(got.Domains), len(got.Composites))
	}
	home, _ := got.Tables[0].Column("home")
	email, _ := got.Tables[0].Column("email")
	if home.Type != "address" || email.Type != "email" {
		t.Errorf("column types read back as %q and %q", home.Type, email.Type)
	}
}

// Catches: a diff that does not alter a domain or a composite type in place
// (a drop and create that fails while a column uses it), does not drop the
// default, NOT NULL or a check that the model no longer has, and a base type
// change that loses the data of the columns that use the domain.
func TestPostgresDomainsAndCompositesDiffApplies(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	roundTrip(t, db, dcmpSchema())
	if _, err := db.Driver().Exec(ctx, `INSERT INTO customers (id, email, code) VALUES (1, 'a@b.c', 'abc')`, nil); err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		name    string
		mutate  func(*schema.Schema)
		destroy bool
	}{
		{"alter the domain", func(s *schema.Schema) {
			d := &s.Domains[0]
			d.NotNull, d.Default = false, "''"
			d.Checks = []schema.DomainCheck{{Name: "email_shape", Expr: "VALUE ~ '@'"}, {Name: "email_x", Expr: "VALUE <> 'x'"}}
		}, false},
		{"add an attribute", func(s *schema.Schema) {
			s.Composites[1].Attributes = append(s.Composites[1].Attributes, schema.CompositeAttr{Name: "b", Type: "integer"})
		}, false},
		{"retype an attribute", func(s *schema.Schema) {
			s.Composites[1].Attributes[0].Type = "character varying(10)"
		}, false},
		{"drop an attribute", func(s *schema.Schema) {
			s.Composites[1].Attributes = s.Composites[1].Attributes[:1]
		}, true},
		{"change the base type", func(s *schema.Schema) { s.Domains[1].Type = "character varying(20)" }, true},
	}
	var applied []func(*schema.Schema)
	for _, st := range steps {
		prev := dcmpSchema(applied...)
		applied = append(applied, st.mutate)
		next := dcmpSchema(applied...)
		opts := migrate.DiffOptions{}
		if st.destroy {
			if _, _, err := migrate.Diff(prev, next, opts); !errors.Is(err, migrate.ErrDestructive) {
				t.Fatalf("%s without AllowDestructive: %v, want ErrDestructive", st.name, err)
			}
			opts.AllowDestructive = true
		}
		applyStatements(t, db, planStatements(t, prev, next, opts))
		got := inspectNow(t, db)
		if !schema.Equal(got, next) {
			t.Fatalf("%s: the database differs from the model:\n%s", st.name, jsonDiff(got, next))
		}
		if again, _, err := migrate.Diff(got, next, migrate.DiffOptions{}); err != nil || len(again) != 0 {
			t.Fatalf("%s: the diff after applying is not empty: %v %+v", st.name, err, again)
		}
	}
	codes, err := momo.QueryScalars[string](ctx, db, "SELECT code FROM customers WHERE id = 1")
	if err != nil || len(codes) != 1 || codes[0] != "abc" {
		t.Fatalf("the data of a column of a domain whose base type changed: %v, %v", codes, err)
	}
}

// Catches: a composite type that a column uses being altered with a statement
// PostgreSQL refuses halfway through a migration, instead of the diff refusing
// it up front.
func TestPostgresDiffRefusesAlteringUsedComposite(t *testing.T) {
	from := dcmpSchema()
	to := dcmpSchema(func(s *schema.Schema) {
		s.Composites[0].Attributes = append(s.Composites[0].Attributes, schema.CompositeAttr{Name: "city", Type: "text"})
	})
	_, _, err := migrate.Diff(from, to, migrate.DiffOptions{AllowDestructive: true})
	if err == nil || !strings.Contains(err.Error(), "uses it") {
		t.Fatalf("err = %v, want one that says a column uses the type", err)
	}
}

// Catches: a nested composite type, which momo does not model, being skipped
// or half read, so Baseline and drift report a database that does not match its
// model; it must be refused with an error that wraps ErrUnsupported and names
// the type, and migrate.exclude must be the way out.
func TestPostgresInspectRefusesNestedComposite(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE TYPE inner_t AS (a integer)`)
	mustExec(t, db.Driver(), `CREATE TYPE outer_t AS (i inner_t)`)
	opts := migrate.InspectOptions{Schemas: []string{testdb.Schema(t)}}
	_, err := inspectOnce(ctx, t, db, opts)
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "outer_t") {
		t.Fatalf("err = %v, want ErrUnsupported naming outer_t", err)
	}
	opts.Exclude = []string{"outer_t"}
	got, err := inspectOnce(ctx, t, db, opts)
	if err != nil {
		t.Fatalf("an excluded composite type must not fail the inspection: %v", err)
	}
	if len(got.Composites) != 1 || got.Composites[0].Name != "inner_t" {
		t.Errorf("composites = %+v, want inner_t alone", got.Composites)
	}
}

// Catches: drift that cannot see a domain or a composite type: a rule dropped
// outside momo, a type dropped, and a type or domain nobody declared must each
// be reported, and a database built from the snapshot must be clean (which
// needs the check text PostgreSQL rewrites to compare equal).
func TestPostgresDomainsAndCompositesDrift(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := dcmpSchema(func(s *schema.Schema) {
		// A check whose stored text differs from the declared one.
		s.Domains[1].Checks = []schema.DomainCheck{{Name: "short_code_len", Expr: "length(VALUE) >= 3"}}
	})
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{}))
	e := canonEngine(t, db.Driver(), canonFS(t, desired))
	rep, err := e.Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("drift on the database built from the snapshot: %v %+v", err, rep)
	}

	mustExec(t, db.Driver(), `ALTER DOMAIN email DROP CONSTRAINT email_shape`)
	mustExec(t, db.Driver(), `DROP TYPE spare`)
	mustExec(t, db.Driver(), `CREATE TYPE stray AS (x integer)`)
	mustExec(t, db.Driver(), `CREATE DOMAIN stray_dom AS integer`)
	rep, err = e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	has := func(refs []migrate.ObjectRef, kind, name string) bool {
		return slices.ContainsFunc(refs, func(r migrate.ObjectRef) bool { return r.Kind == kind && r.Name == name })
	}
	changed := false
	for _, c := range rep.Changed {
		changed = changed || (c.Object.Kind == "domain" && c.Object.Name == "email")
	}
	if !changed {
		t.Errorf("the dropped check is not reported as a changed domain: %+v", rep.Changed)
	}
	if !has(rep.Missing, "composite", "spare") {
		t.Errorf("the dropped composite type is not missing: %+v", rep.Missing)
	}
	if !has(rep.Unmanaged, "composite", "stray") || !has(rep.Unmanaged, "domain", "stray_dom") {
		t.Errorf("an undeclared composite type and domain are not unmanaged: %+v", rep.Unmanaged)
	}
}

// arrayDomainSchema is a table with a column of an array of the domain email,
// whose base type is as given.
func arrayDomainSchema(base string) *schema.Schema {
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Domains:    []schema.Domain{{Name: "email", Type: base}},
		Tables: []schema.Table{{
			Name:       "lists",
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "tags", Type: "email[]", Nullable: true}},
			PrimaryKey: []string{"id"},
		}},
	}
}

// Catches: a base type change of a domain that turns a column of an array of the
// domain into a scalar: the column is taken off the domain with the base type
// alone and put back with the domain alone, so an array of text becomes one text
// value and the elements are lost.
func TestPostgresDomainBaseTypeChangeKeepsArrayColumns(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	from, to := arrayDomainSchema("text"), arrayDomainSchema("character varying(50)")
	roundTrip(t, db, from)
	if _, err := db.Driver().Exec(ctx, `INSERT INTO lists (id, tags) VALUES (1, ARRAY['a@b.c', 'x@y.z']::email[])`, nil); err != nil {
		t.Fatal(err)
	}
	applyStatements(t, db, planStatements(t, from, to, migrate.DiffOptions{AllowDestructive: true}))
	if got := inspectNow(t, db); !schema.Equal(got, to) {
		t.Fatalf("the column is not an array of the domain any more:\n%s", jsonDiff(got, to))
	}
	elems, err := momo.QueryScalars[string](ctx, db, "SELECT array_to_string(tags, ',') FROM lists WHERE id = 1")
	if err != nil || len(elems) != 1 || elems[0] != "a@b.c,x@y.z" {
		t.Fatalf("the elements of the array column: %v, %v", elems, err)
	}
}
