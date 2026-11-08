// SPDX-License-Identifier: Apache-2.0

// Package graphql generates a GraphQL schema from momo models: Relay
// connections with keyset cursors, typed filter and order inputs and one
// query per model. It serves queries only; the spec lists no mutations.
package graphql

import (
	"sort"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/naming"
)

// scalar describes the GraphQL scalar a Go type maps to and the filter input
// that goes with it.
type scalar struct {
	name   string // String, Int, Boolean, Float, Time
	filter string // StringFilter, ...
}

var (
	scString = scalar{"String", "StringFilter"}
	scInt    = scalar{"Int", "IntFilter"}
	scBool   = scalar{"Boolean", "BoolFilter"}
	scFloat  = scalar{"Float", "FloatFilter"}
	scTime   = scalar{"Time", "TimeFilter"}
)

// filterOrder is the order in which shared filter inputs are emitted.
var filterOrder = []scalar{scString, scInt, scBool, scFloat, scTime}

// scalarFor maps a field's base Go type to a scalar. skip is set for types
// GraphQL does not expose (vectors); ok is false for a type with no mapping.
func scalarFor(baseType string) (sc scalar, skip, ok bool) {
	t := strings.TrimPrefix(baseType, "*")
	switch t {
	case "string", "sql.NullString":
		return scString, false, true
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64",
		"sql.NullInt16", "sql.NullInt32", "sql.NullInt64", "sql.NullByte":
		return scInt, false, true
	case "bool", "sql.NullBool":
		return scBool, false, true
	case "float32", "float64", "sql.NullFloat64":
		return scFloat, false, true
	case "time.Time", "sql.NullTime":
		return scTime, false, true
	}
	if strings.HasPrefix(t, "vector.") {
		return scalar{}, true, false
	}
	return scalar{}, false, false
}

func isInteger(sc scalar) bool { return sc == scInt }

// column is one allowlisted, exposed column of a model.
type column struct {
	name  string // database column
	field string // GraphQL field name
	sc    scalar
	null  bool
}

// camel turns snake_case or CamelCase into lowerCamelCase.
func camel(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if i == 0 || p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	out := strings.Join(parts, "")
	if out == "" {
		return out
	}
	return strings.ToLower(out[:1]) + out[1:]
}

// SDL returns the GraphQL schema for the models of g and the diagnostics for
// columns whose type has no GraphQL filter. Vector columns, sensitive columns
// and relations that are not in the allowlist are not exposed and raise no
// diagnostic. The output is deterministic.
func SDL(g *gen.Graph) ([]byte, []gen.Diagnostic) {
	var diags []gen.Diagnostic
	models := append([]*gen.Model(nil), g.Models...)
	sort.SliceStable(models, func(i, j int) bool { return models[i].GoName < models[j].GoName })

	seen := map[string]*gen.Model{}
	var kept []*gen.Model
	for _, m := range models {
		if prev, dup := seen[m.GoName]; dup {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: "GraphQL type " + m.GoName + " is produced by models in " + prev.PkgPath + " and " + m.PkgPath})
			continue
		}
		seen[m.GoName] = m
		kept = append(kept, m)
	}

	used := map[scalar]bool{}
	var body strings.Builder
	var query strings.Builder
	query.WriteString("type Query {\n")
	for _, m := range kept {
		cols, cd := columnsOf(m)
		diags = append(diags, cd...)
		for _, c := range cols {
			used[c.sc] = true
		}
		pk := singleIntPK(g, m, cols)
		writeModel(&body, m, cols, pk, &diags)
		single := lowerFirst(m.GoName)
		plural := lowerFirst(naming.Plural(m.GoName))
		if plural == single {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: "GraphQL query names for " + m.GoName + " collide: " + single})
		}
		query.WriteString("  " + plural + "(first: Int, after: String, filter: " + m.GoName + "Filter, order: " + m.GoName + "Order): " + m.GoName + "Connection!\n")
		if pk != "" {
			query.WriteString("  " + single + "(id: ID!): " + m.GoName + "\n")
		}
	}
	query.WriteString("}\n")

	var out strings.Builder
	out.WriteString("# " + strings.TrimPrefix(gen.Header(), "// ") + "\n")
	if used[scTime] {
		out.WriteString("\nscalar Time\n")
	}
	out.WriteString("\ntype PageInfo {\n  hasNextPage: Boolean!\n  endCursor: String\n}\n")
	out.WriteString("\nenum OrderDirection {\n  ASC\n  DESC\n}\n")
	for _, sc := range filterOrder {
		if used[sc] {
			out.WriteString("\n" + sharedFilter(sc))
		}
	}
	out.WriteString(body.String())
	out.WriteString("\n" + query.String())
	return []byte(out.String()), diags
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// columnsOf returns the exposed columns of m in field order, using the same
// allowlist as the generated ColumnByName: no sensitive fields and no
// repeated column.
func columnsOf(m *gen.Model) ([]column, []gen.Diagnostic) {
	var cols []column
	var diags []gen.Diagnostic
	seen := map[string]bool{}
	for _, f := range m.Fields {
		if f.Tags.Sensitive || seen[f.Column] {
			continue
		}
		seen[f.Column] = true
		sc, skip, ok := scalarFor(f.BaseType)
		if skip {
			continue
		}
		if !ok {
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: "no GraphQL filter for " + f.BaseType})
			continue
		}
		cols = append(cols, column{name: f.Column, field: camel(f.Column), sc: sc, null: f.Nullable})
	}
	return cols, diags
}

// singleIntPK returns the GraphQL field name of m's primary key when it is a
// single integer column, else "".
func singleIntPK(g *gen.Graph, m *gen.Model, cols []column) string {
	if g.Schema == nil {
		return ""
	}
	t, ok := g.Schema.Table(m.Table)
	if !ok || len(t.PrimaryKey) != 1 {
		return ""
	}
	for _, c := range cols {
		if c.name == t.PrimaryKey[0] && isInteger(c.sc) {
			return c.field
		}
	}
	return ""
}

func writeModel(b *strings.Builder, m *gen.Model, cols []column, pk string, diags *[]gen.Diagnostic) {
	name := m.GoName
	taken := map[string]bool{}
	b.WriteString("\ntype " + name + " {\n")
	for _, c := range cols {
		typ := c.sc.name
		if c.field == pk {
			typ = "ID"
		}
		if !c.null || c.field == pk {
			typ += "!"
		}
		taken[c.field] = true
		b.WriteString("  " + c.field + ": " + typ + "\n")
	}
	for _, r := range m.Relations {
		if r.Target == nil {
			continue
		}
		field := lowerFirst(r.GoName)
		if taken[field] {
			*diags = append(*diags, gen.Diagnostic{Pos: r.Pos, Msg: "GraphQL field " + field + " on " + name + " is both a column and a relation"})
			continue
		}
		taken[field] = true
		switch r.Kind {
		case gen.HasMany, gen.ManyToMany:
			b.WriteString("  " + field + ": [" + r.Target.GoName + "!]!\n")
		default:
			b.WriteString("  " + field + ": " + r.Target.GoName + "\n")
		}
	}
	b.WriteString("}\n")

	b.WriteString("\ntype " + name + "Connection {\n  edges: [" + name + "Edge!]!\n  pageInfo: PageInfo!\n}\n")
	b.WriteString("\ntype " + name + "Edge {\n  node: " + name + "!\n  cursor: String!\n}\n")

	b.WriteString("\ninput " + name + "Filter {\n")
	b.WriteString("  and: [" + name + "Filter!]\n  or: [" + name + "Filter!]\n  not: " + name + "Filter\n")
	for _, c := range cols {
		b.WriteString("  " + c.field + ": " + c.sc.filter + "\n")
	}
	b.WriteString("}\n")

	b.WriteString("\nenum " + name + "OrderField {\n")
	for _, c := range cols {
		b.WriteString("  " + strings.ToUpper(c.name) + "\n")
	}
	b.WriteString("}\n")
	b.WriteString("\ninput " + name + "Order {\n  field: " + name + "OrderField!\n  direction: OrderDirection!\n}\n")
}

func sharedFilter(sc scalar) string {
	n := sc.name
	switch sc {
	case scString:
		return "input StringFilter {\n  eq: String\n  ne: String\n  in: [String!]\n  contains: String\n}\n"
	case scBool:
		return "input BoolFilter {\n  eq: Boolean\n  ne: Boolean\n}\n"
	}
	return "input " + sc.filter + " {\n  eq: " + n + "\n  ne: " + n + "\n  in: [" + n + "!]\n  lt: " + n + "\n  lte: " + n + "\n  gt: " + n + "\n  gte: " + n + "\n}\n"
}
