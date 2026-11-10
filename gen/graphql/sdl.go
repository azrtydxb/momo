// SPDX-License-Identifier: Apache-2.0

// Package graphql generates a GraphQL schema from momo models: Relay
// connections with keyset cursors, typed filter and order inputs and one
// query per model. It serves queries only; the spec lists no mutations.
package graphql

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/internal/gosrc"
	"github.com/azrtydxb/momo/gen/naming"
)

// nameRE is the GraphQL Name production; names starting with two
// underscores are reserved for introspection.
var nameRE = regexp.MustCompile(`^[_A-Za-z][_0-9A-Za-z]*$`)

func validName(s string) bool { return nameRE.MatchString(s) && !strings.HasPrefix(s, "__") }

// scalar describes the GraphQL scalar a Go type maps to and the filter input
// that goes with it.
type scalar struct {
	name   string // String, Int, Int64, Boolean, Float, Time
	filter string // StringFilter, ...
}

var (
	scString = scalar{"String", "StringFilter"}
	// scStringEq is a text column the server cannot search with contains: a
	// nullable one, or any string type but string. It has no contains operator.
	scStringEq = scalar{"String", "StringEqFilter"}
	scInt      = scalar{"Int", "IntFilter"}
	// scInt64 carries 64-bit and unsigned 32-bit integers, which do not fit
	// the 32-bit GraphQL Int. It is a custom scalar encoded as a string.
	scInt64 = scalar{"Int64", "Int64Filter"}
	scBool  = scalar{"Boolean", "BoolFilter"}
	scFloat = scalar{"Float", "FloatFilter"}
	scTime  = scalar{"Time", "TimeFilter"}
)

// filterOrder is the order in which shared filter inputs are emitted.
var filterOrder = []scalar{scString, scStringEq, scInt, scInt64, scBool, scFloat, scTime}

// reservedTypes are the names of the shared types and the built-in scalars;
// no model may derive one of them.
var reservedTypes = map[string]bool{
	"Query": true, "Mutation": true, "Subscription": true,
	"PageInfo": true, "OrderDirection": true,
	"Time": true, "Int64": true, "Int": true, "Float": true, "String": true, "Boolean": true, "ID": true,
	"StringFilter": true, "StringEqFilter": true, "IntFilter": true, "Int64Filter": true, "BoolFilter": true, "FloatFilter": true, "TimeFilter": true,
}

// scalarFor maps a field's base Go type to a scalar. skip is set for types
// GraphQL does not expose (vectors); ok is false for a type with no mapping.
func scalarFor(baseType string) (sc scalar, skip, ok bool) {
	t := strings.TrimPrefix(baseType, "*")
	switch t {
	case "string", "sql.NullString":
		return scString, false, true
	case "int8", "int16", "int32", "uint8", "uint16", "sql.NullInt16", "sql.NullInt32", "sql.NullByte":
		return scInt, false, true
	case "int", "int64", "uint", "uint32", "uint64", "sql.NullInt64":
		return scInt64, false, true
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

// orderable reports whether a page can be ordered by c: keyset pagination needs
// a key that is never null.
func orderable(c column) bool { return !c.null }

func isInteger(sc scalar) bool { return sc == scInt || sc == scInt64 }

// column is one allowlisted, exposed column of a model.
type column struct {
	name  string // database column
	field string // GraphQL field name
	enum  string // GraphQL order enum value
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

// derivedTypes lists the type names the model name derives.
func derivedTypes(name string) []string {
	return []string{name, name + "Connection", name + "Edge", name + "Filter", name + "OrderField", name + "Order"}
}

// SDL returns the GraphQL schema for the models of g and the diagnostics for
// columns and models that cannot be exposed. Vector columns, sensitive
// columns and relations that are not in the allowlist are not exposed and
// raise no diagnostic. Names are validated against the GraphQL Name rule,
// and a column, model or derived type name that collides with another is a
// diagnostic and is left out. The output is deterministic.
func SDL(g *gen.Graph) ([]byte, []gen.Diagnostic) {
	keep, byModel, diags := analyze(g)
	diag := func(d gen.Diagnostic) { diags = append(diags, d) }
	used := map[scalar]bool{}
	var body strings.Builder
	var query strings.Builder
	for _, k := range keep {
		for _, c := range k.cols {
			used[c.sc] = true
		}
		writeModel(&body, k.m, k.cols, k.pk, byModel, &diags)
		query.WriteString("  " + k.plural + "(first: Int, after: String, filter: " + k.m.GoName + "Filter, order: " + k.m.GoName + "Order): " + k.m.GoName + "Connection!\n")
		if k.pk != "" {
			query.WriteString("  " + k.single + "(id: ID!): " + k.m.GoName + "\n")
		}
	}
	if len(keep) == 0 {
		diag(gen.Diagnostic{Msg: "no model can be exposed, so the schema has no Query type"})
	}

	var out strings.Builder
	out.WriteString("# " + strings.TrimPrefix(gen.Header(), "// ") + "\n")
	if used[scTime] {
		out.WriteString("\nscalar Time\n")
	}
	if used[scInt64] {
		out.WriteString("\n# Int64 is a 64-bit integer encoded as a string.\nscalar Int64\n")
	}
	out.WriteString("\ntype PageInfo {\n  hasNextPage: Boolean!\n  endCursor: String\n}\n")
	out.WriteString("\nenum OrderDirection {\n  ASC\n  DESC\n}\n")
	for _, sc := range filterOrder {
		if used[sc] {
			out.WriteString("\n" + sharedFilter(sc))
		}
	}
	out.WriteString(body.String())
	if query.Len() > 0 {
		out.WriteString("\ntype Query {\n" + query.String() + "}\n")
	}
	return []byte(out.String()), diags
}

// keptModel is a model that has a GraphQL type, with its exposed columns.
type keptModel struct {
	m      *gen.Model
	cols   []column
	pk     string // GraphQL field name of a single integer primary key
	single string // query name for one row
	plural string // query name for a page
}

// analyze decides which models of g get GraphQL types and which of their
// columns are exposed, with the diagnostics for what cannot be exposed. SDL and
// the Go glue of the server read the same result, so they never disagree.
func analyze(g *gen.Graph) (keep []keptModel, byModel map[*gen.Model]bool, diags []gen.Diagnostic) {
	diag := func(d gen.Diagnostic) { diags = append(diags, d) }
	models := append([]*gen.Model(nil), g.Models...)
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].GoName != models[j].GoName {
			return models[i].GoName < models[j].GoName
		}
		return models[i].PkgPath < models[j].PkgPath
	})

	taken := map[string]string{}   // derived type name -> model
	queries := map[string]string{} // query field name -> model
	byModel = map[*gen.Model]bool{}
	for _, m := range models {
		fail := func(msg string) { diag(gen.Diagnostic{Pos: m.Pos, Msg: msg}) }
		if !validName(m.GoName) {
			fail("model name " + m.GoName + " is not a valid GraphQL name")
			continue
		}
		var clash string
		for _, n := range derivedTypes(m.GoName) {
			if reservedTypes[n] {
				clash = n + " is a reserved GraphQL type name"
			} else if owner, dup := taken[n]; dup {
				clash = "type " + n + " is also derived from model " + owner
			}
		}
		if clash != "" {
			fail("GraphQL types of model " + m.GoName + " collide: " + clash)
			continue
		}
		cols, cd := columnsOf(m)
		diags = append(diags, cd...)
		if len(cols) == 0 {
			fail("model " + m.GoName + " has no exposed columns, so it has no GraphQL type")
			continue
		}
		if !slices.ContainsFunc(cols, orderable) {
			fail("model " + m.GoName + " has no column a page can be ordered by (all are nullable), so it has no GraphQL type")
			continue
		}
		single := gosrc.LowerFirst(m.GoName)
		plural := gosrc.LowerFirst(naming.Plural(m.GoName))
		pk := singleIntPK(g, m, cols)
		qnames := []string{plural}
		if pk != "" {
			qnames = append(qnames, single)
		}
		if plural == single {
			fail("GraphQL query names for " + m.GoName + " collide: " + single)
			continue
		}
		qclash := ""
		for _, q := range qnames {
			if owner, dup := queries[q]; dup {
				qclash = "query " + q + " is also derived from model " + owner
			}
		}
		if qclash != "" {
			fail("GraphQL queries of model " + m.GoName + " collide: " + qclash)
			continue
		}
		for _, n := range derivedTypes(m.GoName) {
			taken[n] = m.GoName
		}
		for _, q := range qnames {
			queries[q] = m.GoName
		}
		keep = append(keep, keptModel{m: m, cols: cols, pk: pk, single: single, plural: plural})
		byModel[m] = true
	}
	return keep, byModel, diags
}

// columnsOf returns the exposed columns of m in field order, using the same
// allowlist as the generated ColumnByName: no sensitive fields and no
// repeated column. A column whose derived names are invalid or collide with an
// earlier column, or with the reserved filter fields and, or and not, is a
// diagnostic and is left out.
func columnsOf(m *gen.Model) ([]column, []gen.Diagnostic) {
	var cols []column
	var diags []gen.Diagnostic
	seen := map[string]bool{}
	fields := map[string]string{"and": "", "or": "", "not": ""}
	enums := map[string]string{}
	for _, f := range m.Fields {
		if f.Tags.Sensitive || seen[f.Column] {
			continue
		}
		seen[f.Column] = true
		sc, skip, ok := scalarFor(f.BaseType)
		if skip {
			continue
		}
		if sc == scString && (f.BaseType != "string" || f.Nullable) {
			sc = scStringEq
		}
		if !ok {
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: "no GraphQL filter for " + f.BaseType})
			continue
		}
		c := column{name: f.Column, field: camel(f.Column), enum: strings.ToUpper(f.Column), sc: sc, null: f.Nullable}
		bad := ""
		owner, fieldTaken := fields[c.field]
		switch {
		case !validName(c.field) || !validName(c.enum):
			bad = "is not a valid GraphQL name"
		case fieldTaken && owner == "":
			bad = "collides with the reserved filter field " + c.field
		case fieldTaken:
			bad = "collides with column " + owner + " (both become " + c.field + ")"
		case enums[c.enum] != "":
			bad = "collides with column " + enums[c.enum] + " (both become " + c.enum + ")"
		}
		if bad != "" {
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: "column " + f.Column + " of " + m.GoName + " " + bad})
			continue
		}
		fields[c.field] = c.name
		enums[c.enum] = c.name
		cols = append(cols, c)
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

func writeModel(b *strings.Builder, m *gen.Model, cols []column, pk string, exposed map[*gen.Model]bool, diags *[]gen.Diagnostic) {
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
		if r.Target == nil || !exposed[r.Target] {
			continue
		}
		field := gosrc.LowerFirst(r.GoName)
		switch {
		case !validName(field):
			*diags = append(*diags, gen.Diagnostic{Pos: r.Pos, Msg: "relation " + r.GoName + " of " + name + " is not a valid GraphQL name"})
			continue
		case taken[field]:
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
		if orderable(c) {
			b.WriteString("  " + c.enum + "\n")
		}
	}
	b.WriteString("}\n")
	b.WriteString("\ninput " + name + "Order {\n  field: " + name + "OrderField!\n  direction: OrderDirection!\n}\n")
}

func sharedFilter(sc scalar) string {
	n := sc.name
	switch sc {
	case scString:
		return "input StringFilter {\n  eq: String\n  ne: String\n  in: [String!]\n  contains: String\n}\n"
	case scStringEq:
		return "input StringEqFilter {\n  eq: String\n  ne: String\n  in: [String!]\n}\n"
	case scBool:
		return "input BoolFilter {\n  eq: Boolean\n  ne: Boolean\n}\n"
	}
	return "input " + sc.filter + " {\n  eq: " + n + "\n  ne: " + n + "\n  in: [" + n + "!]\n  lt: " + n + "\n  lte: " + n + "\n  gt: " + n + "\n  gte: " + n + "\n}\n"
}
