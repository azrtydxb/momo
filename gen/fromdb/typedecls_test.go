// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func domainSample() *schema.Schema {
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Domains: []schema.Domain{
			{Name: "email", Type: "text", NotNull: true, Default: "''::text", Checks: []schema.DomainCheck{{Name: "email_shape", Expr: "VALUE <> ''::text"}}},
			{Name: "short_code", Type: "character varying(12)"},
			{Name: "big_count", Type: "bigint"},
		},
		Composites: []schema.Composite{
			{Name: "address", Attributes: []schema.CompositeAttr{{Name: "street", Type: "text"}, {Name: "zip", Type: "text"}, {Name: "floor", Type: "integer"}, {Name: "seen_at", Type: "timestamp with time zone"}}},
		},
		Tables: []schema.Table{{
			Name: "customers",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint"},
				{Name: "email", Type: "email"},
				{Name: "code", Type: "short_code", Nullable: true},
				{Name: "visits", Type: "big_count"},
				{Name: "home", Type: "address", Nullable: true},
				{Name: "work", Type: "address"},
			},
			PrimaryKey: []string{"id"},
		}},
	}
}

// Catches: a domain column written as a Go type of its own, or as the domain's
// name, instead of the type of its base type with a type tag; a domain or a
// composite type dropped on the way to the models, so the init migration
// builds a table whose column type does not exist; a composite struct whose
// fields are not pointers, which would fail to scan a NULL attribute; and a
// declaration the model reader would not accept.
func TestGenerateFromDBDomainsAndComposites(t *testing.T) {
	res, files := generate(t, domainSample(), Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	src := files["customers"]
	mustMatch(t, "customers model", src,
		`Email\s+string\s+`+"`"+`momo:"type:email"`+"`",
		`Code\s+\*string\s+`+"`"+`momo:"type:short_code"`+"`",
		`Visits\s+int64\s+`+"`"+`momo:"type:big_count"`+"`",
		`Home\s+\*types\.Composite\[Address\]\s+`+"`"+`momo:"type:address"`+"`",
		`Work\s+types\.Composite\[Address\]\s+`+"`"+`momo:"type:address"`+"`",
		`type Address struct`,
		`Street\s+\*string`,
		`Zip\s+\*string`,
		`Floor\s+\*int32`,
		`SeenAt\s+\*time\.Time`,
	)
	mustContain(t, "customers model", src,
		`c.Domain("email").As("text").NotNull().Default("''::text").Check("email_shape", "VALUE <> ''::text")`,
		`c.Domain("short_code").As("character varying(12)")`,
		`c.Domain("big_count").As("bigint")`,
		`c.Composite("address").Attr("street", "text").Attr("zip", "text").Attr("floor", "integer").Attr("seen_at", "timestamp with time zone")`,
		`"github.com/azrtydxb/momo/types"`,
	)
	if strings.Contains(src, "size:12") {
		t.Errorf("a domain column was given the size of its base type:\n%s", src)
	}
	if len(res.Snapshot.Domains) != 3 || len(res.Snapshot.Composites) != 1 {
		t.Errorf("the snapshot has %d domains and %d composites, want 3 and 1", len(res.Snapshot.Domains), len(res.Snapshot.Composites))
	}
}

// Catches: a composite struct named like a model, which would not compile in
// one package, and one with a name that is not exported.
func TestGenerateFromDBCompositeStructName(t *testing.T) {
	s := domainSample()
	s.Composites[0].Name = "customer"
	s.Tables[0].Columns[4].Type, s.Tables[0].Columns[5].Type = "customer", "customer"
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	src := files["customers"]
	mustMatch(t, "customers model", src, `type Customer struct`, `type Customer2 struct`, `types\.Composite\[Customer2\]`)
}

// Catches: a composite type or an array of one being written as a model field,
// which scans nothing, instead of the generation failing with a finding that
// names it.
func TestGenerateFromDBRefusesNestedComposites(t *testing.T) {
	nested := domainSample()
	nested.Composites = append(nested.Composites, schema.Composite{Name: "outer", Attributes: []schema.CompositeAttr{{Name: "addr", Type: "address"}}})
	res, _ := generate(t, nested, Config{})
	if !res.Failed() || !findingMentions(res, "outer", "nested composite types are not supported") {
		t.Errorf("a composite type inside a composite type was not refused: %v", res.Findings)
	}

	array := domainSample()
	array.Tables[0].Columns = append(array.Tables[0].Columns, schema.Column{Name: "history", Type: "address[]", Nullable: true})
	res, _ = generate(t, array, Config{})
	if !res.Failed() || !findingMentions(res, "customers.history", "array of a composite type") {
		t.Errorf("a column of an array of composite types was not refused: %v", res.Findings)
	}

	over := domainSample()
	over.Domains = append(over.Domains, schema.Domain{Name: "addr_dom", Type: "address"})
	res, _ = generate(t, over, Config{})
	if !res.Failed() || !findingMentions(res, "addr_dom", "composite type") {
		t.Errorf("a domain over a composite type was not refused: %v", res.Findings)
	}
}

// Catches: an attribute whose Go type a types.Composite cannot read (an array)
// written as a field anyway, which fails at the first scan.
func TestGenerateFromDBRefusesUnreadableAttribute(t *testing.T) {
	s := domainSample()
	s.Composites[0].Attributes = append(s.Composites[0].Attributes, schema.CompositeAttr{Name: "tags", Type: "text[]"})
	res, _ := generate(t, s, Config{})
	if !res.Failed() || !findingMentions(res, "address.tags", "cannot read") {
		t.Errorf("an array attribute was not refused: %v", res.Findings)
	}
}

// Catches: a composite attribute of a domain type reported as unmapped, or
// mapped to something other than the Go type of the base of the domain, even
// through a domain based on another domain.
func TestGenerateFromDBCompositeAttributeOfDomain(t *testing.T) {
	s := domainSample()
	s.Domains = append(s.Domains, schema.Domain{Name: "work_email", Type: "email"})
	s.Composites[0].Attributes = append(s.Composites[0].Attributes,
		schema.CompositeAttr{Name: "contact", Type: "email"},
		schema.CompositeAttr{Name: "backup", Type: "work_email"},
		schema.CompositeAttr{Name: "code", Type: "short_code"},
		schema.CompositeAttr{Name: "visits", Type: "big_count"},
	)
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustMatch(t, "customers model", files["customers"],
		`Contact\s+\*string`, `Backup\s+\*string`, `Code\s+\*string`, `Visits\s+\*int64`,
	)
}

// Catches: domains and composite types written for a dialect that has none: a
// SQLite or MySQL schema never carries them, and a PostgreSQL-only mapping
// must not run there.
func TestGenerateFromDBIgnoresDomainsOutsidePostgres(t *testing.T) {
	s := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: dialect.SQLite, Tables: []schema.Table{{
		Name:       "t",
		Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "email", Type: "email"}},
		PrimaryKey: []string{"id"},
	}}}
	res, _ := generateSQLite(t, s, Config{})
	if !res.Failed() {
		t.Errorf("a column of the unknown type email on SQLite must be unmapped, not a domain: %v", res.Findings)
	}
}

func findingMentions(res *Result, object, text string) bool {
	for _, f := range res.Findings {
		if f.Severity == SevError && strings.Contains(f.Object, object) && strings.Contains(f.Msg, text) {
			return true
		}
	}
	return false
}
