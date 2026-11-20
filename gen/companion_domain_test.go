// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Catches: Config.Domain or Config.Composite not reaching the schema (a
// migration that builds a table whose column type does not exist), a check, the
// default, NOT NULL or an attribute lost or reordered, and a type tag that does
// not become the column type.
func TestCompanionDomainAndComposite(t *testing.T) {
	g, diags, err := Load([]string{"./companiondomain"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, diags := BuildSchema(g.Models, pg())
	if len(diags) != 0 {
		t.Fatalf("schema diagnostics: %v", diags)
	}
	wantDomains := []schema.Domain{{
		Name: "email", Type: "text", NotNull: true, Default: "''",
		// The checks are in name order, as every named collection is.
		Checks: []schema.DomainCheck{{Name: "email_at", Expr: "VALUE ~ '@'"}, {Name: "email_shape", Expr: "VALUE <> ''"}},
	}}
	if !reflect.DeepEqual(s.Domains, wantDomains) {
		t.Errorf("domains = %+v, want %+v", s.Domains, wantDomains)
	}
	wantComposites := []schema.Composite{{
		Name:       "address",
		Attributes: []schema.CompositeAttr{{Name: "street", Type: "text"}, {Name: "zip", Type: "text"}},
	}}
	if !reflect.DeepEqual(s.Composites, wantComposites) {
		t.Errorf("composites = %+v, want %+v", s.Composites, wantComposites)
	}
	for col, want := range map[string]struct {
		typ      string
		nullable bool
	}{
		"email":  {"email", false},
		"backup": {"email", true},
		"home":   {"address", false},
		"work":   {"address", true},
	} {
		c := mustCol(t, s, "customers", col)
		if c.Type != want.typ || c.Nullable != want.nullable {
			t.Errorf("customers.%s = type %q nullable %v, want %q and %v", col, c.Type, c.Nullable, want.typ, want.nullable)
		}
	}
}

// Catches: a domain or a composite type reaching a dialect that has neither:
// the failure would come late, from the renderer, instead of the generator
// naming the declaration.
func TestCompanionDomainAndCompositeRefusedOutsidePostgres(t *testing.T) {
	g, _, err := Load([]string{"./companiondomain"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		_, diags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{d}})
		joined := diagStrings(diags)
		for _, want := range []string{
			`domain "email" is not supported on ` + string(d),
			`composite type "address" is not supported on ` + string(d),
			"momo: unsupported by dialect or driver",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: missing diagnostic %q in:\n%s", d, want, joined)
			}
		}
	}
}

// Catches: a malformed Config.Domain or Config.Composite accepted, which would
// write a name that PostgreSQL folds to another into the DDL or build a type
// with half an attribute.
func TestCompanionDomainAndCompositeRefusals(t *testing.T) {
	_, diags, err := Load([]string{"./companiondomainbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	for _, want := range []string{
		`type name "address" is already declared at models.go:`,
		`domain "no_base": a domain needs a base type`,
		`duplicate As on domain "twice"`,
		`domain "Upper": the name and the schema must be lower-case unquoted identifiers`,
		`domain "no_expr": check "c" needs an expression`,
		`composite type "empty": a composite type needs at least one attribute`,
		`composite type "pair": two attributes are named "a"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing diagnostic %q in:\n%s", want, joined)
		}
	}
}

// Catches: a field that cannot work being accepted: a types.Composite that names
// no composite type, a composite type given to a field that is not a
// types.Composite, an array of composite types, a composite type inside a
// composite type and a domain over one. Each fails at apply or at the first
// scan otherwise, far from the model that caused it.
func TestCompositeUseRefusals(t *testing.T) {
	// Load builds the schema, so the diagnostics come with it.
	_, diags, err := Load([]string{"./companiondomainuse"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	for _, want := range []string{
		`a types.Composite field needs a type tag that names a composite type declared with Config.Composite`,
		`the type tag names the composite type "address", so the field must be a types.Composite of a struct, not string`,
		`"address[]" is an array of a composite type, which is not supported yet`,
		`composite type "outer": attribute "home" is a composite type or an array of one`,
		`domain "over" is over a composite type`,
		"momo: unsupported by dialect or driver",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing diagnostic %q in:\n%s", want, joined)
		}
	}
}
