// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// --------------------------------------------- domains and composite types

// domainDef is the part of CREATE DOMAIN after the name: the base type, the
// default, NOT NULL and the named checks.
func domainDef(d schema.Domain) (string, error) {
	if err := schema.ValidateDomain(d); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(" AS " + d.Type)
	if d.Default != "" {
		b.WriteString(" DEFAULT " + d.Default)
	}
	if d.NotNull {
		b.WriteString(" NOT NULL")
	}
	for _, c := range d.Checks {
		b.WriteString(" CONSTRAINT " + ident(c.Name) + " CHECK (" + c.Expr + ")")
	}
	return b.String(), nil
}

func renderCreateDomain(c migrate.Change) ([]migrate.Statement, error) {
	d, err := get[schema.Domain](c.After)
	if err != nil {
		return nil, err
	}
	def, err := domainDef(d)
	if err != nil {
		return nil, err
	}
	return one("CREATE DOMAIN " + tableRef(d.Schema, d.Name) + def), nil
}

// renderAlterDomain renders the ALTER DOMAIN statements that turn the domain
// before into after: its constraints that go or change are dropped first, then
// the default and NOT NULL are set, then the new constraints are added. The
// base type is not altered here: a change of it is a recreate_domain.
func renderAlterDomain(c migrate.Change) ([]migrate.Statement, error) {
	before, err := get[schema.Domain](c.Before)
	if err != nil {
		return nil, err
	}
	after, err := get[schema.Domain](c.After)
	if err != nil {
		return nil, err
	}
	if err := schema.ValidateDomain(after); err != nil {
		return nil, err
	}
	ne := func(x string) string { return schema.NormalizeExpr(dialect.Postgres, x) }
	if schema.NormalizeType(dialect.Postgres, before.Type) != schema.NormalizeType(dialect.Postgres, after.Type) {
		return nil, errors.New("the base type of a domain cannot be altered in place")
	}
	pre := "ALTER DOMAIN " + tableRef(after.Schema, after.Name) + " "
	var out []migrate.Statement
	add := func(s string) { out = append(out, migrate.Statement{SQL: s}) }
	newExpr := map[string]string{}
	for _, ck := range after.Checks {
		newExpr[ck.Name] = ne(ck.Expr)
	}
	oldExpr := map[string]string{}
	for _, ck := range before.Checks {
		oldExpr[ck.Name] = ne(ck.Expr)
		if e, ok := newExpr[ck.Name]; !ok || e != oldExpr[ck.Name] {
			add(pre + "DROP CONSTRAINT " + ident(ck.Name))
		}
	}
	if ne(before.Default) != ne(after.Default) {
		if after.Default == "" {
			add(pre + "DROP DEFAULT")
		} else {
			add(pre + "SET DEFAULT " + after.Default)
		}
	}
	if before.NotNull != after.NotNull {
		if after.NotNull {
			add(pre + "SET NOT NULL")
		} else {
			add(pre + "DROP NOT NULL")
		}
	}
	for _, ck := range after.Checks {
		if e, ok := oldExpr[ck.Name]; !ok || e != newExpr[ck.Name] {
			add(pre + "ADD CONSTRAINT " + ident(ck.Name) + " CHECK (" + ck.Expr + ")")
		}
	}
	return out, nil
}

// renderRecreateDomain renders a change of the base type of a domain: each
// column that uses the domain is changed to the old base type, the domain is
// dropped and created again, and each column that stays on the domain is
// changed back to it. The last step rewrites the column and fails on a value
// the new type or the new constraints reject.
func renderRecreateDomain(c migrate.Change) ([]migrate.Statement, error) {
	before, err := get[schema.Domain](c.Before)
	if err != nil {
		return nil, err
	}
	after, err := get[schema.Domain](c.After)
	if err != nil {
		return nil, err
	}
	def, err := domainDef(after)
	if err != nil {
		return nil, err
	}
	var out []migrate.Statement
	dims := func(col migrate.DomainColumn) string {
		if col.Array {
			return "[]"
		}
		return ""
	}
	for _, col := range c.DomainColumns {
		old := before.Type + dims(col)
		out = append(out, migrate.Statement{SQL: "ALTER TABLE " + tableRef(col.Schema, col.Table) + " ALTER COLUMN " + ident(col.Column) + " TYPE " + old + " USING " + ident(col.Column) + "::" + old})
	}
	out = append(out,
		migrate.Statement{SQL: "DROP DOMAIN " + tableRef(before.Schema, before.Name)},
		migrate.Statement{SQL: "CREATE DOMAIN " + tableRef(after.Schema, after.Name) + def},
	)
	ref := tableRef(after.Schema, after.Name)
	for _, col := range c.DomainColumns {
		if col.Reattach {
			typ := ref + dims(col)
			out = append(out, migrate.Statement{SQL: "ALTER TABLE " + tableRef(col.Schema, col.Table) + " ALTER COLUMN " + ident(col.Column) + " TYPE " + typ + " USING " + ident(col.Column) + "::" + typ})
		}
	}
	return out, nil
}

func renderCreateComposite(c migrate.Change) ([]migrate.Statement, error) {
	x, err := get[schema.Composite](c.After)
	if err != nil {
		return nil, err
	}
	if err := schema.ValidateComposite(x); err != nil {
		return nil, err
	}
	attrs := make([]string, len(x.Attributes))
	for i, a := range x.Attributes {
		attrs[i] = ident(a.Name) + " " + a.Type
	}
	return one("CREATE TYPE " + tableRef(x.Schema, x.Name) + " AS (" + strings.Join(attrs, ", ") + ")"), nil
}

// renderAlterComposite renders the ALTER TYPE statements that turn the
// composite type before into after: dropped attributes first, then the ones
// whose type changes, then the new ones in their order. PostgreSQL appends a
// new attribute and cannot move one; the differ refuses a plan that needs it.
func renderAlterComposite(c migrate.Change) ([]migrate.Statement, error) {
	before, err := get[schema.Composite](c.Before)
	if err != nil {
		return nil, err
	}
	after, err := get[schema.Composite](c.After)
	if err != nil {
		return nil, err
	}
	if err := schema.ValidateComposite(after); err != nil {
		return nil, err
	}
	nt := func(x string) string { return schema.NormalizeType(dialect.Postgres, x) }
	pre := "ALTER TYPE " + tableRef(after.Schema, after.Name) + " "
	oldType := map[string]string{}
	for _, a := range before.Attributes {
		oldType[a.Name] = a.Type
	}
	newName := map[string]bool{}
	for _, a := range after.Attributes {
		newName[a.Name] = true
	}
	var out []migrate.Statement
	add := func(s string) { out = append(out, migrate.Statement{SQL: s}) }
	for _, a := range before.Attributes {
		if !newName[a.Name] {
			add(pre + "DROP ATTRIBUTE " + ident(a.Name))
		}
	}
	for _, a := range after.Attributes {
		if t, ok := oldType[a.Name]; ok && nt(t) != nt(a.Type) {
			add(pre + "ALTER ATTRIBUTE " + ident(a.Name) + " TYPE " + a.Type)
		}
	}
	for _, a := range after.Attributes {
		if _, ok := oldType[a.Name]; !ok {
			add(pre + "ADD ATTRIBUTE " + ident(a.Name) + " " + a.Type)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("composite type %q has no attribute change to render", after.Name)
	}
	return out, nil
}
