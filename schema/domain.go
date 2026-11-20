// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Domain is a PostgreSQL domain: a base type with an optional NOT NULL, a
// default and named CHECK constraints, as in
//
//	CREATE DOMAIN email AS text NOT NULL DEFAULT '' CONSTRAINT email_shape CHECK (VALUE LIKE '%@%')
//
// A column uses it by naming it as its type; Ref is that spelling. A domain
// exists on PostgreSQL only. The name of a domain, like that of a composite
// type, is a lower-case unquoted identifier (see ValidateDomain).
type Domain struct {
	// Schema is the PostgreSQL namespace. Empty means the connection's
	// current_schema().
	Schema string `json:"schema,omitempty"`
	Name   string `json:"name"`
	// Type is the base type, such as "text", "character varying(255)" or
	// "numeric(10,2)". Compare with NormalizeType.
	Type string `json:"type"`
	// NotNull marks the domain NOT NULL: no column of it may hold NULL.
	NotNull bool `json:"not_null,omitempty"`
	// Default is a SQL expression, empty when the domain has no default.
	Default string `json:"default,omitempty"`
	// Checks are the named CHECK constraints. VALUE stands for the value that
	// is checked.
	Checks []DomainCheck `json:"checks,omitempty"`
}

// DomainCheck is a named CHECK constraint of a Domain.
type DomainCheck struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

// Ref is how a column names the domain as its type: the bare name in the
// current schema, otherwise "schema.name".
func (d Domain) Ref() string { return typeRef(d.Schema, d.Name) }

// Composite is a PostgreSQL composite type, as in
//
//	CREATE TYPE address AS (street text, zip text)
//
// A column uses it by naming it as its type; Ref is that spelling. The
// attributes are positional: the Go struct of a types.Composite maps its
// exported fields to them in order, and PostgreSQL cannot reorder them. A
// composite exists on PostgreSQL only.
type Composite struct {
	// Schema is the PostgreSQL namespace. Empty means the connection's
	// current_schema().
	Schema     string          `json:"schema,omitempty"`
	Name       string          `json:"name"`
	Attributes []CompositeAttr `json:"attributes"`
}

// CompositeAttr is one attribute of a Composite.
type CompositeAttr struct {
	Name string `json:"name"`
	// Type is the SQL type of the attribute. It is not a composite type and
	// not an array of one: those are refused.
	Type string `json:"type"`
}

// Ref is how a column names the composite as its type: the bare name in the
// current schema, otherwise "schema.name".
func (c Composite) Ref() string { return typeRef(c.Schema, c.Name) }

func typeRef(schemaName, name string) string {
	if schemaName == "" {
		return name
	}
	return schemaName + "." + name
}

// typeIdent matches the names a domain and a composite type, and their schema,
// may have: lower-case unquoted identifiers. The type name is also how a
// column spells its type, and the schema package compares types in lower case,
// so a name that needs quoting could not be told apart from another. The names
// of an attribute and of a domain constraint are quoted when rendered and have
// no such limit.
var typeIdent = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// ValidateDomain reports why d cannot be created: a name that is not a
// lower-case unquoted identifier, an empty base type, a check without a name
// or an expression, or two checks of one name.
func ValidateDomain(d Domain) error {
	if d.Name == "" {
		return errors.New("a domain needs a name")
	}
	if !typeIdent.MatchString(d.Name) || (d.Schema != "" && !typeIdent.MatchString(d.Schema)) {
		return fmt.Errorf("domain %q: the name and the schema must be lower-case unquoted identifiers (letters, digits and underscores, not starting with a digit)", d.Ref())
	}
	if strings.TrimSpace(d.Type) == "" {
		return fmt.Errorf("domain %q: a domain needs a base type", d.Ref())
	}
	seen := map[string]bool{}
	for _, c := range d.Checks {
		switch {
		case c.Name == "":
			return fmt.Errorf("domain %q: a check needs a name", d.Ref())
		case strings.TrimSpace(c.Expr) == "":
			return fmt.Errorf("domain %q: check %q needs an expression", d.Ref(), c.Name)
		case seen[c.Name]:
			return fmt.Errorf("domain %q: two checks are named %q", d.Ref(), c.Name)
		}
		seen[c.Name] = true
	}
	return nil
}

// ValidateComposite reports why c cannot be created: a name that is not a
// lower-case unquoted identifier, no attribute, an attribute without a name or
// a type, or two attributes of one name. A composite attribute is not checked
// against the other composites here: see CompositeNesting.
func ValidateComposite(c Composite) error {
	if c.Name == "" {
		return errors.New("a composite type needs a name")
	}
	if !typeIdent.MatchString(c.Name) || (c.Schema != "" && !typeIdent.MatchString(c.Schema)) {
		return fmt.Errorf("composite type %q: the name and the schema must be lower-case unquoted identifiers (letters, digits and underscores, not starting with a digit)", c.Ref())
	}
	if len(c.Attributes) == 0 {
		return fmt.Errorf("composite type %q: a composite type needs at least one attribute", c.Ref())
	}
	seen := map[string]bool{}
	for _, a := range c.Attributes {
		switch {
		case a.Name == "":
			return fmt.Errorf("composite type %q: an attribute needs a name", c.Ref())
		case strings.TrimSpace(a.Type) == "":
			return fmt.Errorf("composite type %q: attribute %q needs a type", c.Ref(), a.Name)
		case seen[a.Name]:
			return fmt.Errorf("composite type %q: two attributes are named %q", c.Ref(), a.Name)
		}
		seen[a.Name] = true
	}
	return nil
}

// CompositeNesting reports the attribute of c whose type is another composite
// type of cs, or an array of one. Momo refuses both for now: the record text
// of a nested composite is not modelled. It returns "" when there is none.
func CompositeNesting(c Composite, cs []Composite) string {
	for _, a := range c.Attributes {
		if IsCompositeRef(a.Type, cs) {
			return a.Name
		}
	}
	return ""
}

// IsCompositeRef reports whether the SQL type t names one of the composite
// types cs, or is an array of one.
func IsCompositeRef(t string, cs []Composite) bool {
	t = strings.TrimSuffix(lowerType(t), "[]")
	for _, c := range cs {
		if t == strings.ToLower(c.Ref()) {
			return true
		}
	}
	return false
}

// lowerType lower-cases a type name and collapses its white space,
// without reading it as a PostgreSQL type: a user-defined type has no other
// spelling.
func lowerType(t string) string {
	return strings.Join(strings.Fields(strings.ToLower(t)), " ")
}
