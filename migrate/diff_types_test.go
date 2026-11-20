// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

var (
	idColumn    = schema.Column{Name: "id", Type: "bigint"}
	emailColumn = schema.Column{Name: "email", Type: "email"}
)

// domainSchema is a schema with the domain email, changed by mutate, and a
// users table of the given columns when there are any.
func domainSchema(mutate func(*schema.Domain), cols ...schema.Column) *schema.Schema {
	d := schema.Domain{Name: "email", Type: "text", Checks: []schema.DomainCheck{{Name: "email_shape", Expr: "VALUE <> ''"}}}
	if mutate != nil {
		mutate(&d)
	}
	s := &schema.Schema{Dialect: "postgres", Domains: []schema.Domain{d}, Tables: []schema.Table{}}
	if len(cols) > 0 {
		s.Tables = []schema.Table{{Name: "users", Columns: cols, PrimaryKey: []string{"id"}}}
	}
	return s
}

// Catches: a domain created, dropped or altered that Diff does not see, one
// that is altered when only the spelling of its expression changed, and an
// ALTER that is not marked risky when it adds a rule existing values can break.
func TestDiffDomains(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*schema.Domain)
		want   []ChangeKind
		risky  bool
	}{
		{"unchanged", nil, nil, false},
		{"spelling of a check", func(d *schema.Domain) { d.Checks[0].Expr = "value  <>  ''" }, nil, false},
		{"default added", func(d *schema.Domain) { d.Default = "''" }, []ChangeKind{AlterDomain}, false},
		{"not null added", func(d *schema.Domain) { d.NotNull = true }, []ChangeKind{AlterDomain}, true},
		{"check changed", func(d *schema.Domain) { d.Checks[0].Expr = "VALUE <> 'x'" }, []ChangeKind{AlterDomain}, true},
		{"check added", func(d *schema.Domain) {
			d.Checks = append(d.Checks, schema.DomainCheck{Name: "email_at", Expr: "VALUE ~ '@'"})
		}, []ChangeKind{AlterDomain}, true},
		{"check dropped", func(d *schema.Domain) { d.Checks = nil }, []ChangeKind{AlterDomain}, false},
		{"base type", func(d *schema.Domain) { d.Type = "character varying(255)" }, []ChangeKind{RecreateDomain}, false},
	}
	for _, c := range cases {
		changes, hints, err := Diff(domainSchema(nil), domainSchema(c.mutate), DiffOptions{})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := kinds(changes); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: changes = %v, want %v", c.name, got, c.want)
			continue
		}
		if len(changes) == 1 && changes[0].Risky != c.risky {
			t.Errorf("%s: Risky = %v, want %v", c.name, changes[0].Risky, c.risky)
		}
		if risky := strings.Contains(strings.Join(hints, "\n"), "risky: alter_domain"); risky != c.risky {
			t.Errorf("%s: risky hint = %v, want %v (%v)", c.name, risky, c.risky, hints)
		}
	}

	empty := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{}}
	created, _, err := Diff(empty, domainSchema(nil), DiffOptions{})
	if err != nil || !reflect.DeepEqual(kinds(created), []ChangeKind{CreateDomain}) {
		t.Errorf("create = %v, %v", kinds(created), err)
	}
	if _, _, err := Diff(domainSchema(nil), empty, DiffOptions{}); !errors.Is(err, ErrDestructive) {
		t.Errorf("dropping a domain without AllowDestructive: %v, want ErrDestructive", err)
	}
	dropped, _, err := Diff(domainSchema(nil), empty, DiffOptions{AllowDestructive: true})
	if err != nil || !reflect.DeepEqual(kinds(dropped), []ChangeKind{DropDomain}) || !dropped[0].Destructive {
		t.Errorf("drop = %v, %v", kinds(dropped), err)
	}
}

// Catches: a base type change that PostgreSQL cannot do in place planned as a
// quiet ALTER, one that does not name the columns it has to take off the domain
// and put back, one that is not refused as destructive when a column uses the
// domain, and a column the plan drops that is still listed.
func TestDiffDomainBaseTypeChange(t *testing.T) {
	widen := func(d *schema.Domain) { d.Type = "character varying(255)" }
	from := domainSchema(nil, idColumn, emailColumn)
	to := domainSchema(widen, idColumn, emailColumn)
	if _, _, err := Diff(from, to, DiffOptions{}); !errors.Is(err, ErrDestructive) {
		t.Fatalf("a base type change under a column without AllowDestructive: %v, want ErrDestructive", err)
	}
	changes, _, err := Diff(from, to, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != RecreateDomain || !changes[0].Destructive {
		t.Fatalf("changes = %+v", changes)
	}
	want := []DomainColumn{{Table: "users", Column: "email", Reattach: true}}
	if !reflect.DeepEqual(changes[0].DomainColumns, want) {
		t.Errorf("DomainColumns = %+v, want %+v", changes[0].DomainColumns, want)
	}
	if before, ok := changes[0].Before.(*schema.Domain); !ok || before.Type != "text" {
		t.Errorf("Before = %+v, want the old domain", changes[0].Before)
	}
	if after, ok := changes[0].After.(*schema.Domain); !ok || after.Type != "character varying(255)" {
		t.Errorf("After = %+v, want the new domain", changes[0].After)
	}

	// The column goes in the same plan: it is dropped first, so it is not taken
	// off the domain. A column that moves to another type stays on the old base
	// type and is not put back.
	dropCol := domainSchema(widen, idColumn)
	changes, _, err = Diff(from, dropCol, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Kind == RecreateDomain && (len(c.DomainColumns) != 0 || c.Destructive) {
			t.Errorf("a dropped column is listed: %+v", c)
		}
	}
	retyped := domainSchema(widen, idColumn, schema.Column{Name: "email", Type: "text"})
	changes, _, err = Diff(from, retyped, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Kind == RecreateDomain && !reflect.DeepEqual(c.DomainColumns, []DomainColumn{{Table: "users", Column: "email"}}) {
			t.Errorf("a column that leaves the domain: %+v", c.DomainColumns)
		}
	}
}

// Catches: a domain created after the table that uses it, or dropped before the
// table that still uses it, which PostgreSQL refuses either way.
func TestOrderDomainsAroundTables(t *testing.T) {
	empty := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{}}
	with := domainSchema(nil, idColumn, emailColumn)
	create, _, err := Diff(empty, with, DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(Order(create)); !reflect.DeepEqual(got, []ChangeKind{CreateDomain, CreateTable}) {
		t.Errorf("create order = %v", got)
	}
	drop, _, err := Diff(with, empty, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(Order(drop)); !reflect.DeepEqual(got, []ChangeKind{DropTable, DropDomain}) {
		t.Errorf("drop order = %v", got)
	}
}

// compositeSchema is a schema with the composite type address, changed by
// mutate, and a customers table with a column of it when withColumn is set.
func compositeSchema(mutate func(*schema.Composite), withColumn bool) *schema.Schema {
	c := schema.Composite{Name: "address", Attributes: []schema.CompositeAttr{{Name: "street", Type: "text"}, {Name: "zip", Type: "text"}}}
	if mutate != nil {
		mutate(&c)
	}
	s := &schema.Schema{Dialect: "postgres", Composites: []schema.Composite{c}, Tables: []schema.Table{}}
	if withColumn {
		s.Tables = []schema.Table{{Name: "customers", Columns: []schema.Column{idColumn, {Name: "home", Type: "address", Nullable: true}}, PrimaryKey: []string{"id"}}}
	}
	return s
}

// Catches: an attribute added, dropped or retyped that Diff does not see, a
// dropped attribute that is not destructive, and a plan PostgreSQL would reject
// halfway (an attribute inserted before another, a reorder, a type that a
// column uses) that is not refused up front with a message that says why.
func TestDiffComposites(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(*schema.Composite)
		want        []ChangeKind
		destructive bool
	}{
		{"unchanged", nil, nil, false},
		{"attribute added last", func(c *schema.Composite) {
			c.Attributes = append(c.Attributes, schema.CompositeAttr{Name: "city", Type: "text"})
		}, []ChangeKind{AlterComposite}, false},
		{"attribute dropped", func(c *schema.Composite) { c.Attributes = c.Attributes[:1] }, []ChangeKind{AlterComposite}, true},
		{"attribute retyped", func(c *schema.Composite) { c.Attributes[1].Type = "character varying(10)" }, []ChangeKind{AlterComposite}, false},
		{"attribute swapped", func(c *schema.Composite) {
			c.Attributes = []schema.CompositeAttr{{Name: "street", Type: "text"}, {Name: "city", Type: "text"}}
		}, []ChangeKind{AlterComposite}, true},
	}
	for _, c := range cases {
		changes, _, err := Diff(compositeSchema(nil, false), compositeSchema(c.mutate, false), DiffOptions{AllowDestructive: true})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := kinds(changes); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: changes = %v, want %v", c.name, got, c.want)
			continue
		}
		if len(changes) == 1 && changes[0].Destructive != c.destructive {
			t.Errorf("%s: Destructive = %v, want %v", c.name, changes[0].Destructive, c.destructive)
		}
	}
	if _, _, err := Diff(compositeSchema(nil, false), compositeSchema(func(c *schema.Composite) { c.Attributes = c.Attributes[:1] }, false), DiffOptions{}); !errors.Is(err, ErrDestructive) {
		t.Errorf("dropping an attribute without AllowDestructive: %v, want ErrDestructive", err)
	}

	refused := []struct {
		name   string
		mutate func(*schema.Composite)
		cols   bool
		want   string
	}{
		{"inserted before", func(c *schema.Composite) {
			c.Attributes = []schema.CompositeAttr{{Name: "city", Type: "text"}, {Name: "street", Type: "text"}, {Name: "zip", Type: "text"}}
		}, false, "after a new attribute"},
		{"reordered", func(c *schema.Composite) { c.Attributes[0], c.Attributes[1] = c.Attributes[1], c.Attributes[0] }, false, "order"},
		{"used by a column", func(c *schema.Composite) { c.Attributes[1].Type = "integer" }, true, "uses it"},
	}
	for _, c := range refused {
		_, _, err := Diff(compositeSchema(nil, c.cols), compositeSchema(c.mutate, c.cols), DiffOptions{AllowDestructive: true})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one that says %q", c.name, err, c.want)
		}
	}
	// A column that uses the type is no obstacle while the type does not change.
	if changes, _, err := Diff(compositeSchema(nil, true), compositeSchema(nil, true), DiffOptions{}); err != nil || len(changes) != 0 {
		t.Errorf("unchanged type under a column: %v, %v", changes, err)
	}

	empty := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{}}
	created, _, err := Diff(empty, compositeSchema(nil, true), DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(Order(created)); !reflect.DeepEqual(got, []ChangeKind{CreateComposite, CreateTable}) {
		t.Errorf("create order = %v", got)
	}
	dropped, _, err := Diff(compositeSchema(nil, true), empty, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(Order(dropped)); !reflect.DeepEqual(got, []ChangeKind{DropTable, DropComposite}) {
		t.Errorf("drop order = %v", got)
	}
}

// Catches: a column that is an array of the domain recorded as a column of the
// domain itself, so the recreation turns the array into a scalar.
func TestDiffDomainRecreateRecordsArrayColumns(t *testing.T) {
	widen := func(d *schema.Domain) { d.Type = "character varying(255)" }
	tags := schema.Column{Name: "tags", Type: "email[]", Nullable: true}
	changes, _, err := Diff(domainSchema(nil, idColumn, tags), domainSchema(widen, idColumn, tags), DiffOptions{AllowDestructive: true})
	if err != nil || len(changes) != 1 {
		t.Fatalf("changes = %+v, %v", changes, err)
	}
	want := []DomainColumn{{Table: "users", Column: "tags", Reattach: true, Array: true}}
	if !reflect.DeepEqual(changes[0].DomainColumns, want) {
		t.Errorf("DomainColumns = %+v, want %+v", changes[0].DomainColumns, want)
	}
	// A column that stops being an array of the domain is not put back on it.
	scalar := schema.Column{Name: "tags", Type: "email", Nullable: true}
	changes, _, err = Diff(domainSchema(nil, idColumn, tags), domainSchema(widen, idColumn, scalar), DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Kind == RecreateDomain && (len(c.DomainColumns) != 1 || c.DomainColumns[0].Reattach) {
			t.Errorf("a column that changes shape was reattached: %+v", c.DomainColumns)
		}
	}
}

// Catches: a domain recreated while a composite type attribute, another domain
// or a view that the plan leaves alone still depends on it, which PostgreSQL
// rejects halfway through the migration; the differ must refuse up front and
// name what depends on it.
func TestDiffDomainRecreateRefusedUnderOtherObjects(t *testing.T) {
	widen := func(d *schema.Domain) { d.Type = "character varying(255)" }
	view := schema.View{Name: "user_emails", Query: "SELECT email FROM users", Depends: []string{"users"}}
	for _, c := range []struct {
		name  string
		build func(*schema.Schema)
		want  string
	}{
		{"composite attribute", func(s *schema.Schema) {
			s.Composites = []schema.Composite{{Name: "contact", Attributes: []schema.CompositeAttr{{Name: "mail", Type: "email"}}}}
		}, `attribute "mail" of the composite type contact`},
		{"array attribute", func(s *schema.Schema) {
			s.Composites = []schema.Composite{{Name: "contact", Attributes: []schema.CompositeAttr{{Name: "mails", Type: "email[]"}}}}
		}, `attribute "mails" of the composite type contact`},
		{"domain over it", func(s *schema.Schema) {
			s.Domains = append(s.Domains, schema.Domain{Name: "work_email", Type: "email"})
		}, "the domain work_email is based on it"},
		{"untouched view", func(s *schema.Schema) { s.Views = []schema.View{view} }, "the view user_emails reads the table users"},
	} {
		from := domainSchema(nil, idColumn, emailColumn)
		to := domainSchema(widen, idColumn, emailColumn)
		c.build(from)
		c.build(to)
		_, _, err := Diff(from, to, DiffOptions{AllowDestructive: true})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one that says %q", c.name, err, c.want)
		}
	}
	// A view the plan changes is dropped before the columns change and created
	// again after, so it is no obstacle.
	from := domainSchema(nil, idColumn, emailColumn)
	to := domainSchema(widen, idColumn, emailColumn)
	from.Views = []schema.View{{Name: "user_emails", Query: "SELECT email FROM users", Depends: []string{"users"}}}
	to.Views = []schema.View{{Name: "user_emails", Query: "SELECT id, email FROM users", Depends: []string{"users"}}}
	if _, _, err := Diff(from, to, DiffOptions{AllowDestructive: true}); err != nil {
		t.Errorf("a changed view must not block the recreation: %v", err)
	}
}
