// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/internal/gosrc"
)

// keyKind is how the server formats and parses the value of an order key.
type keyKind int

const (
	kString keyKind = iota + 1
	kInt32
	kInt64
	kBool
	kFloat
	kTime
)

// bindKind returns the key kind of a Go field type the server can bind to
// scalar sc, and false for a type it cannot.
func bindKind(sc scalar, goType string) (keyKind, bool) {
	switch strings.TrimPrefix(goType, "*") {
	case "string":
		return kString, sc == scString
	case "int32":
		return kInt32, sc == scInt
	case "int64":
		return kInt64, sc == scInt64
	case "bool":
		return kBool, sc == scBool
	case "float64":
		return kFloat, sc == scFloat
	case "time.Time":
		return kTime, sc == scTime
	}
	return 0, false
}

// gcol is an exposed column with the model field that holds it.
type gcol struct {
	column
	f    *gen.Field
	kind keyKind
}

// gmodel is a model with a GraphQL type, ready for the Go glue.
type gmodel struct {
	keptModel
	modelAlias string
	descAlias  string
	cols       []gcol
	rels       []*gen.Relation
	pk         gcol
}

// commonInitialisms are the words gqlgen upper-cases in a Go name; a resolver
// method name containing one cannot be predicted from the field name here.
var commonInitialisms = map[string]bool{
	"ACL": true, "API": true, "ASCII": true, "CPU": true, "CSS": true, "DNS": true, "EOF": true,
	"GUID": true, "HTML": true, "HTTP": true, "HTTPS": true, "ID": true, "IP": true, "JSON": true,
	"LHS": true, "QPS": true, "RAM": true, "RHS": true, "RPC": true, "SLA": true, "SMTP": true,
	"SQL": true, "SSH": true, "TCP": true, "TLS": true, "TTL": true, "UDP": true, "UI": true,
	"UID": true, "UUID": true, "URI": true, "URL": true, "UTF8": true, "VM": true, "XML": true,
	"XMPP": true, "XSRF": true, "XSS": true,
}

// hasInitialism reports whether a camelCase or snake_case name has a word that
// is a common initialism.
func hasInitialism(name string) bool {
	var words []string
	cur := []rune{}
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range name {
		switch {
		case r == '_':
			flush()
		case unicode.IsUpper(r):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	for _, w := range words {
		if commonInitialisms[strings.ToUpper(w)] {
			return true
		}
	}
	return false
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// exposedRelations returns the relations of m that SDL exposes as fields: the
// target has a type and the field name is valid and not taken by a column.
func exposedRelations(m *gen.Model, cols []gcol, byModel map[*gen.Model]bool) []*gen.Relation {
	taken := map[string]bool{}
	for _, c := range cols {
		taken[c.field] = true
	}
	var out []*gen.Relation
	for _, r := range m.Relations {
		if r.Target == nil || !byModel[r.Target] {
			continue
		}
		field := lowerFirst(r.GoName)
		if !validName(field) || taken[field] {
			continue
		}
		taken[field] = true
		out = append(out, r)
	}
	return out
}

// analyzeGo decides what the Go glue covers and returns a diagnostic for what
// the server cannot bind.
func analyzeGo(g *gen.Graph, modelsPackage string) ([]gmodel, []gen.Diagnostic) {
	keep, byModel, diags := analyze(g)
	var out []gmodel
	aliasOf := map[string]string{}
	taken := map[string]bool{"context": true, "errors": true, "fmt": true, "io": true, "strconv": true, "time": true, "json": true, "momo": true, "apiquery": true, "graphql": true}
	alias := func(path, want string) string {
		if a, ok := aliasOf[path]; ok {
			return a
		}
		a := want
		for n := 2; taken[a]; n++ {
			a = want + strconv.Itoa(n)
		}
		taken[a] = true
		aliasOf[path] = a
		return a
	}
	for _, k := range keep {
		m := k.m
		fail := func(f *gen.Field, msg string) {
			d := gen.Diagnostic{Pos: m.Pos, Msg: msg}
			if f != nil {
				d.Pos = f.Pos
			}
			diags = append(diags, d)
		}
		gm := gmodel{keptModel: k}
		ok := true
		if hasInitialism(k.single) || hasInitialism(k.plural) {
			fail(nil, "the GraphQL server cannot name the resolvers of "+m.GoName+": a query name has an initialism, which gqlgen upper-cases")
			ok = false
		}
		byCol := map[string]*gen.Field{}
		for _, f := range m.Fields {
			if _, dup := byCol[f.Column]; !dup {
				byCol[f.Column] = f
			}
		}
		for _, c := range k.cols {
			f := byCol[c.name]
			kind, bound := bindKind(c.sc, f.GoType)
			switch {
			case len(f.Path) != 1:
				fail(f, "the GraphQL server does not support the embedded field "+f.GoName+" of "+m.GoName)
				ok = false
			case !bound:
				fail(f, "the GraphQL server cannot bind the type "+f.GoType+" of "+m.GoName+"."+f.GoName+" to "+c.sc.name)
				ok = false
			default:
				gc := gcol{column: c, f: f, kind: kind}
				gm.cols = append(gm.cols, gc)
				if c.field == k.pk {
					gm.pk = gc
				}
			}
		}
		if k.pk == "" || gm.pk.f == nil || gm.pk.f.GoType != "int64" {
			fail(nil, "the GraphQL server needs a single int64 primary key on "+m.GoName+" to page it")
			ok = false
		}
		mp, err := gosrc.ModelImport(m)
		if err != nil {
			fail(nil, err.Error())
			continue
		}
		if modelsPackage != "" && mp != modelsPackage {
			fail(nil, "model "+m.GoName+" is in "+mp+", not in the configured models package "+modelsPackage)
			ok = false
		}
		dp, err := gosrc.DescImport(m)
		if err != nil {
			fail(nil, err.Error())
			continue
		}
		if !ok {
			continue
		}
		gm.modelAlias = alias(mp, m.PkgName)
		gm.descAlias = alias(dp, m.SubPkg+"desc")
		gm.rels = exposedRelations(m, gm.cols, byModel)
		out = append(out, gm)
	}
	return out, diags
}

// importsOf lists the import aliases the models of ms use.
func importsOf(ms []gmodel) map[string]string {
	imp := map[string]string{
		"context": "context", "json": "encoding/json", "errors": "errors", "fmt": "fmt", "io": "io",
		"strconv": "strconv", "time": "time",
		"momo":     "github.com/azrtydxb/momo",
		"apiquery": "github.com/azrtydxb/momo/gen/apiquery",
		"graphql":  "github.com/99designs/gqlgen/graphql",
	}
	for _, m := range ms {
		mp, _ := gosrc.ModelImport(m.m)
		dp, _ := gosrc.DescImport(m.m)
		imp[m.modelAlias] = mp
		imp[m.descAlias] = dp
	}
	return imp
}

// GoSource returns the Go glue of the GraphQL server, in package pkg: the Go
// types the schema binds to, the list and get functions and the scalar
// marshalers. It reads the same models and columns as SDL.
func GoSource(g *gen.Graph, pkg, modelsPackage string) ([]byte, []gen.Diagnostic) {
	if !pkgNameRE.MatchString(pkg) {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid Go package name %q for the GraphQL glue", pkg)}}
	}
	ms, diags := analyzeGo(g, modelsPackage)
	if len(diags) > 0 {
		return nil, diags
	}
	used := map[scalar]bool{}
	for _, m := range ms {
		for _, c := range m.cols {
			used[c.sc] = true
		}
	}

	var b strings.Builder
	b.WriteString(glueHelpers)
	b.WriteString(glueScalars)
	for _, sc := range filterOrder {
		if used[sc] {
			b.WriteString(filterSource(sc))
		}
	}
	for _, m := range ms {
		writeGoModel(&b, m)
	}
	body := b.String()

	var out strings.Builder
	out.WriteString(gen.Header() + "\n\n")
	out.WriteString("// Package " + pkg + " holds the Go types, list functions and scalars the gqlgen\n// server of the GraphQL schema binds to.\npackage " + pkg + "\n\n")
	out.WriteString(gosrc.Imports(body, importsOf(ms)))
	out.WriteString(body)
	return []byte(out.String()), nil
}

var pkgNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

const glueHelpers = `// PageInfo describes the page a connection holds.
type PageInfo struct {
	HasNextPage bool
	EndCursor   *string
}

type lookupFunc func(name string) (momo.ColumnRef, error)

func invalid(msg string) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "graphql", Err: errors.New(msg)}
}

func cmpPred(lookup lookupFunc, col, op, val string) (momo.Predicate, error) {
	return apiquery.ParseFilter(lookup, apiquery.Filter{Column: col, Op: op, Value: val})
}

// all ANDs the predicates; no predicate is no restriction.
func all(ps []momo.Predicate) momo.Predicate {
	switch len(ps) {
	case 0:
		return nil
	case 1:
		return ps[0]
	}
	return momo.And(ps...)
}

// combine joins the predicates of the children of a filter with join.
func combine[F any](children []*F, pred func(*F) (momo.Predicate, error), join func(...momo.Predicate) momo.Predicate) (momo.Predicate, error) {
	var ps []momo.Predicate
	for _, c := range children {
		p, err := pred(c)
		if err != nil {
			return nil, err
		}
		if p != nil {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		return nil, nil
	}
	return join(ps...), nil
}

// cmpOps holds the operators of a comparison filter.
type cmpOps[T any] struct {
	eq, ne, lt, lte, gt, gte *T
	in                       []T
}

func (o cmpOps[T]) predicates(lookup lookupFunc, col string, format func(T) string) ([]momo.Predicate, error) {
	var ps []momo.Predicate
	for _, c := range []struct {
		op string
		v  *T
	}{{"eq", o.eq}, {"ne", o.ne}, {"lt", o.lt}, {"le", o.lte}, {"gt", o.gt}, {"ge", o.gte}} {
		if c.v == nil {
			continue
		}
		p, err := cmpPred(lookup, col, c.op, format(*c.v))
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	if o.in != nil {
		if len(o.in) == 0 {
			return nil, invalid("in needs at least one value")
		}
		alts := make([]momo.Predicate, 0, len(o.in))
		for _, v := range o.in {
			p, err := cmpPred(lookup, col, "eq", format(v))
			if err != nil {
				return nil, err
			}
			alts = append(alts, p)
		}
		ps = append(ps, momo.Or(alts...))
	}
	return ps, nil
}

func parseOrder(columns map[string]string, field, direction string) (col string, desc bool, err error) {
	col, ok := columns[field]
	if !ok {
		return "", false, invalid("unknown order field")
	}
	switch direction {
	case "ASC":
		return col, false, nil
	case "DESC":
		return col, true, nil
	}
	return "", false, invalid("unknown order direction")
}

// wanted reports whether the request selects the field at path below the field
// being resolved. Outside a gqlgen request it reports true, so everything is
// loaded.
func wanted(ctx context.Context, path ...string) bool {
	fc := graphql.GetFieldContext(ctx)
	if fc == nil {
		return true
	}
	oc := graphql.GetOperationContext(ctx)
	fields := graphql.CollectFields(oc, fc.Field.Selections, nil)
	for i, name := range path {
		var next *graphql.CollectedField
		for j := range fields {
			if fields[j].Name == name {
				next = &fields[j]
				break
			}
		}
		if next == nil {
			return false
		}
		if i == len(path)-1 {
			return true
		}
		fields = graphql.CollectFields(oc, next.Selections, nil)
	}
	return false
}
`

const glueScalars = `
// MarshalInt64 writes a 64-bit integer as a JSON string, the encoding of the
// Int64 scalar.
func MarshalInt64(i int64) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		_, _ = io.WriteString(w, strconv.Quote(strconv.FormatInt(i, 10)))
	})
}

// UnmarshalInt64 reads an Int64 value: a decimal string or a JSON number.
func UnmarshalInt64(v any) (int64, error) {
	switch v := v.(type) {
	case string:
		return strconv.ParseInt(v, 10, 64)
	case json.Number:
		return v.Int64()
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	}
	return 0, fmt.Errorf("%T is not an Int64", v)
}

// MarshalID64 writes a 64-bit integer identifier as a JSON string.
func MarshalID64(i int64) graphql.Marshaler { return MarshalInt64(i) }

// UnmarshalID64 reads a 64-bit integer identifier.
func UnmarshalID64(v any) (int64, error) { return UnmarshalInt64(v) }
`

// filterSource returns the Go type and predicate of one shared filter input.
func filterSource(sc scalar) string {
	name := sc.filter
	r := strings.NewReplacer("{{N}}", name)
	switch sc {
	case scString:
		return r.Replace(`
func formatString(s string) string { return s }

// StringFilter is the filter input of a text column.
type StringFilter struct {
	Eq       *string
	Ne       *string
	In       []string
	Contains *string
}

func (f *StringFilter) predicate(lookup lookupFunc, col string, contains func(string) momo.Predicate) (momo.Predicate, error) {
	if f == nil {
		return nil, nil
	}
	ps, err := cmpOps[string]{eq: f.Eq, ne: f.Ne, in: f.In}.predicates(lookup, col, formatString)
	if err != nil {
		return nil, err
	}
	if f.Contains != nil {
		if contains == nil {
			return nil, invalid("contains is only supported on text columns")
		}
		ps = append(ps, contains(*f.Contains))
	}
	return all(ps), nil
}
`)
	case scBool:
		return r.Replace(`
func formatBool(v bool) string { return strconv.FormatBool(v) }

// BoolFilter is the filter input of a boolean column.
type BoolFilter struct {
	Eq *bool
	Ne *bool
}

func (f *BoolFilter) predicate(lookup lookupFunc, col string) (momo.Predicate, error) {
	if f == nil {
		return nil, nil
	}
	ps, err := cmpOps[bool]{eq: f.Eq, ne: f.Ne}.predicates(lookup, col, formatBool)
	if err != nil {
		return nil, err
	}
	return all(ps), nil
}
`)
	}
	goType, format := "", ""
	switch sc {
	case scInt:
		goType, format = "int", "func(v int) string { return strconv.Itoa(v) }"
	case scInt64:
		goType, format = "int64", "func(v int64) string { return strconv.FormatInt(v, 10) }"
	case scFloat:
		goType, format = "float64", "func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }"
	case scTime:
		goType, format = "time.Time", "func(v time.Time) string { return v.Format(time.RFC3339Nano) }"
	}
	r = strings.NewReplacer("{{N}}", name, "{{T}}", goType, "{{format}}", format)
	return r.Replace(`
// {{N}} is the filter input of a column of the same scalar.
type {{N}} struct {
	Eq  *{{T}}
	Ne  *{{T}}
	In  []{{T}}
	Lt  *{{T}}
	Lte *{{T}}
	Gt  *{{T}}
	Gte *{{T}}
}

func (f *{{N}}) predicate(lookup lookupFunc, col string) (momo.Predicate, error) {
	if f == nil {
		return nil, nil
	}
	ps, err := cmpOps[{{T}}]{f.Eq, f.Ne, f.Lt, f.Lte, f.Gt, f.Gte, f.In}.predicates(lookup, col, {{format}})
	if err != nil {
		return nil, err
	}
	return all(ps), nil
}
`)
}

// keyFormat is the Go expression that formats the order key of column c from
// the row variable d.
func keyFormat(c gcol) string {
	v := "d." + c.f.GoName
	switch c.kind {
	case kString:
		return v
	case kInt32:
		return "strconv.FormatInt(int64(" + v + "), 10)"
	case kInt64:
		return "strconv.FormatInt(" + v + ", 10)"
	case kBool:
		return "strconv.FormatBool(" + v + ")"
	case kFloat:
		return "strconv.FormatFloat(" + v + ", 'g', -1, 64)"
	}
	return v + ".Format(time.RFC3339Nano)"
}

// keyParse is the Go statement that parses an order key s of column c.
func keyParse(c gcol) string {
	switch c.kind {
	case kString:
		return "return s, nil"
	case kInt32, kInt64:
		return "return strconv.ParseInt(s, 10, 64)"
	case kBool:
		return "return strconv.ParseBool(s)"
	case kFloat:
		return "return strconv.ParseFloat(s, 64)"
	}
	return "return time.Parse(time.RFC3339Nano, s)"
}

func writeGoModel(b *strings.Builder, gm gmodel) {
	m := gm.m
	pkCol := gm.pk.name
	r := strings.NewReplacer(
		"{{M}}", m.GoName,
		"{{lm}}", lowerFirst(m.GoName),
		"{{models}}", gm.modelAlias,
		"{{desc}}", gm.descAlias,
		"{{PK}}", gm.pk.f.GoName,
		"{{pkcol}}", pkCol,
		"{{Plural}}", upperFirst(gm.plural),
	)
	b.WriteString(r.Replace(`
// {{M}}Edge is one {{M}} of a page with its cursor.
type {{M}}Edge struct {
	Node   *{{models}}.{{M}}
	Cursor string
}

// {{M}}Connection is a page of {{M}} rows.
type {{M}}Connection struct {
	Edges    []*{{M}}Edge
	PageInfo *PageInfo
}

// {{M}}Order orders a page of {{M}} rows. Field is an order field value of the
// schema and Direction is ASC or DESC.
type {{M}}Order struct {
	Field     string
	Direction string
}

// {{M}}Filter is the filter input of {{M}} rows.
type {{M}}Filter struct {
	And []*{{M}}Filter
	Or  []*{{M}}Filter
	Not *{{M}}Filter
`))
	for _, c := range gm.cols {
		b.WriteString("\t" + c.f.GoName + " *" + c.sc.filter + "\n")
	}
	b.WriteString("}\n")

	b.WriteString(r.Replace(`
func (f *{{M}}Filter) predicate() (momo.Predicate, error) {
	if f == nil {
		return nil, nil
	}
	steps := []func() (momo.Predicate, error){
		func() (momo.Predicate, error) { return combine(f.And, (*{{M}}Filter).predicate, momo.And) },
		func() (momo.Predicate, error) { return combine(f.Or, (*{{M}}Filter).predicate, momo.Or) },
		func() (momo.Predicate, error) {
			p, err := f.Not.predicate()
			if err != nil || p == nil {
				return nil, err
			}
			return momo.Not(p), nil
		},
`))
	for _, c := range gm.cols {
		call := "f." + c.f.GoName + ".predicate({{desc}}.ColumnByName, " + strconv.Quote(c.name)
		switch {
		case c.sc == scString && c.f.GoType == "string":
			call += ", func(s string) momo.Predicate { return momo.ContainsLiteral({{desc}}." + c.f.GoName + ", s) })"
		case c.sc == scString:
			call += ", nil)"
		default:
			call += ")"
		}
		b.WriteString(r.Replace("\t\tfunc() (momo.Predicate, error) { return " + call + " },\n"))
	}
	b.WriteString(r.Replace(`	}
	var ps []momo.Predicate
	for _, step := range steps {
		p, err := step()
		if err != nil {
			return nil, err
		}
		if p != nil {
			ps = append(ps, p)
		}
	}
	return all(ps), nil
}

// {{lm}}OrderColumns maps an order field value to its column.
var {{lm}}OrderColumns = map[string]string{
`))
	for _, c := range gm.cols {
		b.WriteString("\t" + strconv.Quote(c.enum) + ": " + strconv.Quote(c.name) + ",\n")
	}
	b.WriteString(r.Replace(`}

// {{lm}}Key formats the order key of column col of d; false means col cannot be
// an order key.
func {{lm}}Key(col string, d *{{models}}.{{M}}) (string, bool) {
	switch col {
`))
	for _, c := range gm.cols {
		if c.null {
			continue
		}
		b.WriteString("\tcase " + strconv.Quote(c.name) + ":\n\t\treturn " + keyFormat(c) + ", true\n")
	}
	b.WriteString(r.Replace(`	}
	return "", false
}

// {{lm}}KeyValue parses the order key s of column col.
func {{lm}}KeyValue(col, s string) (any, error) {
	switch col {
`))
	for _, c := range gm.cols {
		if c.null {
			continue
		}
		b.WriteString("\tcase " + strconv.Quote(c.name) + ":\n\t\t" + keyParse(c) + "\n")
	}
	b.WriteString(r.Replace(`	}
	return nil, invalid("unknown order column")
}

func {{lm}}Cursor(orderCol string, d *{{models}}.{{M}}) (string, error) {
	pk := strconv.FormatInt(d.{{PK}}, 10)
	if orderCol == "{{pkcol}}" {
		return apiquery.EncodeCursor(pk), nil
	}
	k, ok := {{lm}}Key(orderCol, d)
	if !ok {
		return "", invalid("cannot order by " + orderCol)
	}
	return apiquery.EncodeCursor(k, pk), nil
}

// List{{Plural}} returns one page of {{M}} rows. It selects the relations the
// request asks for with one extra statement each, however many rows the page
// has.
func List{{Plural}}(ctx context.Context, q momo.Querier, first *int, after *string, filter *{{M}}Filter, order *{{M}}Order) (*{{M}}Connection, error) {
	limit := 0
	if first != nil {
		limit = *first
	}
	limit = apiquery.ClampLimit(limit)
	orderCol, desc := "{{pkcol}}", false
	if order != nil {
		var err error
		if orderCol, desc, err = parseOrder({{lm}}OrderColumns, order.Field, order.Direction); err != nil {
			return nil, err
		}
	}
	if _, ok := {{lm}}Key(orderCol, &{{models}}.{{M}}{}); !ok && orderCol != "{{pkcol}}" {
		return nil, invalid("cannot order by " + orderCol)
	}
	ref, err := {{desc}}.ColumnByName(orderCol)
	if err != nil {
		return nil, err
	}
	items := []momo.Orderable{momo.Asc(ref)}
	if desc {
		items = []momo.Orderable{momo.Desc(ref)}
	}
	if orderCol != "{{pkcol}}" {
		items = append(items, {{desc}}.{{PK}}.Asc())
	}
	sel := momo.Select[{{models}}.{{M}}](q).OrderBy(items...).Limit(int64(limit + 1))
	if filter != nil {
		p, err := filter.predicate()
		if err != nil {
			return nil, err
		}
		if p != nil {
			sel = sel.Where(p)
		}
	}
	if after != nil && *after != "" {
		c, err := apiquery.DecodeCursor(*after)
		if err != nil {
			return nil, err
		}
		cols := []string{orderCol, "{{pkcol}}"}
		if orderCol == "{{pkcol}}" {
			cols = cols[:1]
		}
		if len(c.Keys) != len(cols) {
			return nil, invalid("invalid page cursor")
		}
		vals := make([]any, len(cols))
		for i, k := range c.Keys {
			if vals[i], err = {{lm}}KeyValue(cols[i], k); err != nil {
				return nil, invalid("invalid page cursor")
			}
		}
		sel = sel.After(vals...)
	}
`))
	if len(gm.rels) > 0 {
		b.WriteString(r.Replace("\tvar loaders []momo.Loader[{{models}}.{{M}}]\n"))
		for _, rel := range gm.rels {
			b.WriteString(r.Replace("\tif wanted(ctx, \"edges\", \"node\", " + strconv.Quote(lowerFirst(rel.GoName)) + ") {\n\t\tloaders = append(loaders, {{desc}}." + rel.GoName + ".Scope())\n\t}\n"))
		}
		b.WriteString("\tif len(loaders) > 0 {\n\t\tsel = sel.Preload(loaders...)\n\t}\n")
	}
	b.WriteString(r.Replace(`	rows, err := sel.All(ctx)
	if err != nil {
		return nil, err
	}
	conn := &{{M}}Connection{Edges: []*{{M}}Edge{}, PageInfo: &PageInfo{}}
	if len(rows) > limit {
		rows = rows[:limit]
		conn.PageInfo.HasNextPage = true
	}
	for i := range rows {
		cursor, err := {{lm}}Cursor(orderCol, &rows[i])
		if err != nil {
			return nil, err
		}
		conn.Edges = append(conn.Edges, &{{M}}Edge{Node: &rows[i], Cursor: cursor})
		conn.PageInfo.EndCursor = &cursor
	}
	return conn, nil
}

// Get{{M}} returns the {{M}} with the given id, or nil when there is none. It
// selects the relations the request asks for.
func Get{{M}}(ctx context.Context, q momo.Querier, id int64) (*{{models}}.{{M}}, error) {
	sel := momo.Select[{{models}}.{{M}}](q).Where({{desc}}.{{PK}}.Eq(id))
`))
	if len(gm.rels) > 0 {
		b.WriteString(r.Replace("\tvar loaders []momo.Loader[{{models}}.{{M}}]\n"))
		for _, rel := range gm.rels {
			b.WriteString(r.Replace("\tif wanted(ctx, " + strconv.Quote(lowerFirst(rel.GoName)) + ") {\n\t\tloaders = append(loaders, {{desc}}." + rel.GoName + ".Scope())\n\t}\n"))
		}
		b.WriteString("\tif len(loaders) > 0 {\n\t\tsel = sel.Preload(loaders...)\n\t}\n")
	}
	b.WriteString(r.Replace(`	row, err := sel.One(ctx)
	if errors.Is(err, momo.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
`))
}

// gqlgenConfig returns the gqlgen.yml for the models. All paths are relative to
// the directory of the file. importPath is the import path of that directory.
func gqlgenConfig(ms []gmodel, importPath string) []byte {
	models := map[string][]string{
		"ID":             {"github.com/99designs/gqlgen/graphql.ID", importPath + "/model.ID64"},
		"PageInfo":       {importPath + "/model.PageInfo"},
		"OrderDirection": {"github.com/99designs/gqlgen/graphql.String"},
	}
	used := map[scalar]bool{}
	for _, m := range ms {
		mp, _ := gosrc.ModelImport(m.m)
		n := m.m.GoName
		models[n] = []string{mp + "." + n}
		models[n+"Connection"] = []string{importPath + "/model." + n + "Connection"}
		models[n+"Edge"] = []string{importPath + "/model." + n + "Edge"}
		models[n+"Filter"] = []string{importPath + "/model." + n + "Filter"}
		models[n+"Order"] = []string{importPath + "/model." + n + "Order"}
		models[n+"OrderField"] = []string{"github.com/99designs/gqlgen/graphql.String"}
		for _, c := range m.cols {
			used[c.sc] = true
		}
	}
	for sc := range used {
		models[sc.filter] = []string{importPath + "/model." + sc.filter}
	}
	if used[scTime] {
		models["Time"] = []string{"github.com/99designs/gqlgen/graphql.Time"}
	}
	if used[scInt64] {
		models["Int64"] = []string{importPath + "/model.Int64"}
	}
	names := make([]string, 0, len(models))
	for n := range models {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# " + strings.TrimPrefix(gen.Header(), "// ") + "\n")
	b.WriteString("# Run gqlgen generate in this directory after a change of schema.graphqls.\n")
	b.WriteString("schema:\n  - schema.graphqls\n\n")
	b.WriteString("exec:\n  filename: generated/exec.go\n  package: generated\n\n")
	b.WriteString("model:\n  filename: model/models_gen.go\n  package: model\n\n")
	b.WriteString("resolver:\n  layout: single-file\n  filename: resolver.go\n  package: graph\n\n")
	b.WriteString("models:\n")
	for _, n := range names {
		b.WriteString("  " + n + ":\n    model:\n")
		for _, mod := range models[n] {
			b.WriteString("      - " + mod + "\n")
		}
	}
	return []byte(b.String())
}

// resolverScaffold returns the starting point of resolver.go: the resolvers of
// the generated queries, delegating to the list and get functions.
func resolverScaffold(ms []gmodel, pkg, importPath string) []byte {
	var b strings.Builder
	for _, m := range ms {
		n := m.m.GoName
		r := strings.NewReplacer("{{M}}", n, "{{models}}", m.modelAlias, "{{Plural}}", upperFirst(m.plural), "{{Single}}", upperFirst(m.single), "{{plural}}", m.plural, "{{single}}", m.single)
		b.WriteString(r.Replace(`
// {{Plural}} is the resolver for the {{plural}} field.
func (r *queryResolver) {{Plural}}(ctx context.Context, first *int, after *string, filter *model.{{M}}Filter, order *model.{{M}}Order) (*model.{{M}}Connection, error) {
	return model.List{{Plural}}(ctx, r.DB, first, after, filter, order)
}

// {{Single}} is the resolver for the {{single}} field.
func (r *queryResolver) {{Single}}(ctx context.Context, id int64) (*{{models}}.{{M}}, error) {
	return model.Get{{M}}(ctx, r.DB, id)
}
`))
	}
	body := `
// Resolver is the root resolver. It is yours to edit: momo generate never
// overwrites this file.
type Resolver struct {
	DB *momo.DB
}

// Query returns the generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
` + b.String()
	imp := map[string]string{
		"context":   "context",
		"momo":      "github.com/azrtydxb/momo",
		"generated": importPath + "/generated",
		"model":     importPath + "/model",
	}
	for _, m := range ms {
		mp, _ := gosrc.ModelImport(m.m)
		imp[m.modelAlias] = mp
	}
	return []byte("package " + pkg + "\n\n" + gosrc.Imports(body, imp) + strings.TrimPrefix(body, "\n"))
}
