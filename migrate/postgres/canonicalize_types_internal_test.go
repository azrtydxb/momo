// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a column, a composite attribute or a domain that names a type of
// another schema as "schema.name" keeping that name in the scratch schema, where
// the type exists as "name" only, so the scratch copy cannot be created and the
// domain and the table are never canonicalized.
func TestScratchModelUnqualifiesTypeReferences(t *testing.T) {
	s := &schema.Schema{
		Dialect: "postgres",
		Domains: []schema.Domain{
			{Schema: "billing", Name: "email", Type: "text"},
			{Name: "work_email", Type: "billing.email"},
		},
		Composites: []schema.Composite{{
			Schema: "billing", Name: "address",
			Attributes: []schema.CompositeAttr{{Name: "street", Type: "text"}, {Name: "mail", Type: "Billing.Email"}, {Name: "mails", Type: "billing.email[]"}},
		}},
		Tables: []schema.Table{{
			Name: "customers",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint"},
				{Name: "email", Type: "billing.email"},
				{Name: "emails", Type: "billing.email[]"},
				{Name: "home", Type: "billing.address"},
				{Name: "other", Type: "other.email"},
			},
		}},
	}
	m, err := scratchModel(s)
	if err != nil {
		t.Fatal(err)
	}
	var cols []string
	for _, c := range m.Tables[0].Columns {
		cols = append(cols, c.Type)
	}
	if want := []string{"bigint", "email", "email[]", "address", "other.email"}; !slices.Equal(cols, want) {
		t.Errorf("column types = %v, want %v", cols, want)
	}
	if m.Domains[0].Schema != "" || m.Domains[1].Type != "email" {
		t.Errorf("domains = %+v", m.Domains)
	}
	attrs := m.Composites[0].Attributes
	if attrs[0].Type != "text" || attrs[1].Type != "email" || attrs[2].Type != "email[]" {
		t.Errorf("attributes = %+v", attrs)
	}
	if s.Tables[0].Columns[1].Type != "billing.email" {
		t.Error("scratchModel changed the schema it was given")
	}
}

// Catches: a domain whose rewrite failed in the best-effort mode being dropped
// from the report, so drift compares its declared text without saying so, and a
// failed rewrite that changes the domains it was given.
func TestDomainRewriteFailureIsReported(t *testing.T) {
	s := &schema.Schema{Domains: []schema.Domain{
		{Name: "email", Type: "text", Default: "''"},
		{Schema: "billing", Name: "code", Type: "text", Checks: []schema.DomainCheck{{Name: "c", Expr: "VALUE <> ''"}}},
		{Name: "plain", Type: "text"},
	}}
	want := []migrate.ObjectRef{{Kind: "domain", Name: "email"}, {Kind: "domain", Schema: "billing", Name: "code"}}
	if got := domainRefs(s); !slices.Equal(got, want) {
		t.Errorf("domainRefs = %v, want %v", got, want)
	}
	if got := textTables(s); !slices.Equal(got, want) {
		t.Errorf("textTables = %v, want the domains that carry text %v", got, want)
	}
	read := &schema.Schema{Domains: []schema.Domain{{Name: "email", Type: "text", Default: "''::text"}}}
	before := s.Domains[0].Default
	if err := rewriteDomains(s, read); err == nil {
		t.Fatal("a domain missing from the scratch schema was rewritten")
	}
	if s.Domains[0].Default != before {
		t.Errorf("a failed rewrite changed the declared default to %q", s.Domains[0].Default)
	}
}
