// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// compositeInfo is a composite type of the database and the Go struct written
// for it.
type compositeInfo struct {
	c      schema.Composite
	goName string
}

// typeRef is the lower-case spelling a column uses to name a domain or a
// composite type.
func typeRef(typ string) string {
	return strings.ToLower(schema.NormalizeType(dialect.Postgres, typ))
}

// domain returns the domain a column type names.
func (g *generator) domain(typ string) (schema.Domain, bool) {
	d, ok := g.domains[typeRef(typ)]
	return d, ok
}

// composite returns the composite type a column type names.
func (g *generator) composite(typ string) (*compositeInfo, bool) {
	ci, ok := g.composites[typeRef(typ)]
	return ci, ok
}

// arrayOfComposite reports whether typ is an array of a composite type.
func (g *generator) arrayOfComposite(typ string) bool {
	base, ok := strings.CutSuffix(typeRef(typ), "[]")
	if !ok {
		return false
	}
	_, ok = g.composites[base]
	return ok
}

// typeDecls indexes the domains and composite types of a PostgreSQL schema, and
// names the Go struct of each composite type after the models have their names,
// so a struct never takes the name of a model. A composite type with an
// attribute that has no Go type, or is a composite type itself, is an error and
// is dropped from the index.
func (g *generator) typeDecls() {
	g.domains = map[string]schema.Domain{}
	g.composites = map[string]*compositeInfo{}
	if g.o.Dialect != dialect.Postgres {
		return
	}
	for _, d := range g.clean.Domains {
		g.domains[typeRef(d.Ref())] = d
	}
	var keep []schema.Composite
	for _, c := range g.clean.Composites {
		if name := schema.CompositeNesting(c, g.clean.Composites); name != "" {
			g.add(SevError, c.Ref(), "%v: attribute %q is a composite type or an array of one; nested composite types are not supported yet", momo.ErrUnsupported, name)
			continue
		}
		keep = append(keep, c)
		words := splitWords(c.Name)
		name := exported(pascal(words, "Composite"))
		g.composites[typeRef(c.Ref())] = &compositeInfo{c: c, goName: g.uniqueStruct(name)}
	}
	g.clean.Composites = keep
	for _, d := range g.clean.Domains {
		if schema.IsCompositeRef(d.Type, g.clean.Composites) {
			g.add(SevError, d.Ref(), "%v: the domain is over a composite type, which is not supported yet", momo.ErrUnsupported)
		}
	}
}

// domainBase resolves the Go type of a column of the domain d: the Go type of
// its base type, by an introspect.types rule or the built-in mapping. There is
// no Go type per domain.
func (g *generator) domainBase(d schema.Domain) (GoType, error) {
	if tr, ok := g.types[schema.NormalizeType(g.o.Dialect, d.Type)]; ok {
		return custom(tr.Go, tr.Import), nil
	}
	gt, err := MapType(g.o.Dialect, d.Type)
	if err != nil {
		return GoType{}, fmt.Errorf("%w (the base type of the domain %q)", err, d.Ref())
	}
	if g.o.Config.UUID == UUIDGoogle {
		gt = googleUUID(gt)
	}
	return gt, nil
}

// attrGoType resolves the Go type of an attribute of a composite type: an
// introspect.types rule, then the built-in mapping. An array has no record
// text form in a types.Composite, so it is not mapped.
func (g *generator) attrGoType(sqlType string) (GoType, error) {
	// An attribute of a domain has the Go type of the base type of the domain,
	// through a chain of domains.
	for range 8 {
		base, array := strings.CutSuffix(sqlType, "[]")
		d, ok := g.domain(base)
		if !ok {
			break
		}
		sqlType = d.Type
		if array {
			sqlType += "[]"
		}
	}
	if tr, ok := g.types[schema.NormalizeType(g.o.Dialect, sqlType)]; ok {
		return custom(tr.Go, tr.Import), nil
	}
	gt, err := MapType(g.o.Dialect, sqlType)
	if err != nil {
		return GoType{}, err
	}
	if strings.HasPrefix(gt.Expr, "[]") && gt.Expr != "[]byte" || strings.HasPrefix(gt.Expr, "map[") {
		return GoType{}, fmt.Errorf("%w: the attribute type %q maps to %s, which a types.Composite cannot read; add an introspect.types rule with a type that implements sql.Scanner and driver.Valuer", ErrUnmapped, sqlType, gt.Expr)
	}
	if g.o.Config.UUID == UUIDGoogle {
		gt = googleUUID(gt)
	}
	return gt, nil
}

// compositeStruct writes the Go struct of a composite type. Every attribute may
// be NULL in the database, so a field is a pointer, except a []byte, which holds
// NULL as nil. Its fields map to the attributes by position.
func (g *generator) compositeStruct(ci *compositeInfo, imports map[string]bool) (string, bool) {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s is the PostgreSQL composite type %s. Its fields map to the attributes by\n// position, in order, and a pointer field is NULL when nil.\n", ci.goName, strconv.Quote(ci.c.Ref()))
	fmt.Fprintf(&b, "type %s struct {\n", ci.goName)
	used := map[string]bool{}
	ok := true
	for _, a := range ci.c.Attributes {
		gt, err := g.attrGoType(a.Type)
		if err != nil {
			g.add(SevError, ci.c.Ref()+"."+a.Name, "%v", err)
			ok = false
			continue
		}
		expr := gt.Expr
		if !gt.Nilable {
			expr = "*" + expr
		}
		if gt.Import != "" {
			imports[gt.Import] = true
		}
		for _, p := range exprImports(gt.Expr) {
			imports[p] = true
		}
		fmt.Fprintf(&b, "\t%s %s\n", uniqueName(used, exported(pascal(splitWords(a.Name), "Attr"))), expr)
	}
	b.WriteString("}\n")
	return b.String(), ok
}

// declareTypes writes the struct of each composite type and the Domain and
// Composite calls on the first model, and drops from the snapshot what the
// models cannot declare. With introspect.only a domain or composite type is
// kept only when a column of a kept table, or a kept composite type, uses it.
func (g *generator) declareTypes(host *model) {
	used := g.usedTypes()
	var doms []schema.Domain
	for _, d := range g.clean.Domains {
		if len(g.o.Config.Only) > 0 && !used[typeRef(d.Ref())] && !schema.MatchObject(g.o.Config.Only, d.Schema, d.Name) {
			g.add(SevNote, d.Ref(), "domain is used by nothing that introspect.only keeps and is not named in it: not declared and not in the snapshot; add it to migrate.exclude so the baseline and drift ignore the live domain")
			continue
		}
		if !tagSafe(d.Ref()) {
			g.add(SevError, d.Ref(), "the domain name cannot be written in a tag")
			continue
		}
		doms = append(doms, d)
		host.stmts = append(host.stmts, domainStmt(d))
	}
	g.clean.Domains = doms
	var comps []schema.Composite
	for _, c := range g.clean.Composites {
		ci := g.composites[typeRef(c.Ref())]
		if ci == nil {
			continue
		}
		if len(g.o.Config.Only) > 0 && !used[typeRef(c.Ref())] && !schema.MatchObject(g.o.Config.Only, c.Schema, c.Name) {
			g.add(SevNote, c.Ref(), "composite type is used by nothing that introspect.only keeps and is not named in it: not declared and not in the snapshot; add it to migrate.exclude so the baseline and drift ignore the live type")
			continue
		}
		if !tagSafe(c.Ref()) {
			g.add(SevError, c.Ref(), "the composite type name cannot be written in a tag")
			continue
		}
		decl, ok := g.compositeStruct(ci, host.imports)
		if !ok {
			continue
		}
		comps = append(comps, c)
		host.decls = append(host.decls, decl)
		host.stmts = append(host.stmts, compositeStmt(c))
	}
	g.clean.Composites = comps
}

// usedTypes returns the domains and composite types, by typeRef, that a column
// of a kept table names, directly or as an array.
func (g *generator) usedTypes() map[string]bool {
	out := map[string]bool{}
	for _, t := range g.clean.Tables {
		for _, c := range t.Columns {
			out[strings.TrimSuffix(typeRef(c.Type), "[]")] = true
		}
	}
	return out
}

// domainStmt renders c.Domain(name) with its options.
func domainStmt(d schema.Domain) string {
	var b strings.Builder
	b.WriteString("c.Domain(" + strconv.Quote(d.Name) + ")")
	if d.Schema != "" {
		b.WriteString(".Schema(" + strconv.Quote(d.Schema) + ")")
	}
	b.WriteString(".As(" + strconv.Quote(d.Type) + ")")
	if d.NotNull {
		b.WriteString(".NotNull()")
	}
	if d.Default != "" {
		b.WriteString(".Default(" + strconv.Quote(d.Default) + ")")
	}
	for _, ck := range d.Checks {
		b.WriteString(".Check(" + strconv.Quote(ck.Name) + ", " + strconv.Quote(ck.Expr) + ")")
	}
	return b.String()
}

// compositeStmt renders c.Composite(name) with its attributes.
func compositeStmt(x schema.Composite) string {
	var b strings.Builder
	b.WriteString("c.Composite(" + strconv.Quote(x.Name) + ")")
	if x.Schema != "" {
		b.WriteString(".Schema(" + strconv.Quote(x.Schema) + ")")
	}
	for _, a := range x.Attributes {
		b.WriteString(".Attr(" + strconv.Quote(a.Name) + ", " + strconv.Quote(a.Type) + ")")
	}
	return b.String()
}
