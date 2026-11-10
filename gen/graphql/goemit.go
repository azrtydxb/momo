// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"fmt"
	"maps"
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
		return kString, sc == scString || sc == scStringEq
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
	f     *gen.Field
	kind  keyKind
	ident string // the name of the column's descriptor in the descriptor package
}

// grel is an exposed relation with the name of its descriptor.
type grel struct {
	r      *gen.Relation
	ident  string
	target *gen.Model
}

// gmodel is a model with a GraphQL type, ready for the Go glue.
type gmodel struct {
	keptModel
	modelAlias string
	descAlias  string
	cols       []gcol
	rels       []grel
	pk         gcol
	pkIdent    string
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
func exposedRelations(g *gen.Graph, m *gen.Model, cols []gcol, byModel map[*gen.Model]bool) []grel {
	taken := map[string]bool{}
	for _, c := range cols {
		taken[c.field] = true
	}
	_, idents := gen.DescriptorIdents(m, g)
	var out []grel
	for i, r := range m.Relations {
		if r.Target == nil || !byModel[r.Target] {
			continue
		}
		field := gosrc.LowerFirst(r.GoName)
		if !validName(field) || taken[field] {
			continue
		}
		taken[field] = true
		out = append(out, grel{r: r, ident: idents[i], target: r.Target})
	}
	return out
}

// analyzeGo decides what the Go glue covers and returns a diagnostic for what
// the server cannot bind.
func analyzeGo(g *gen.Graph, modelsPackage string) ([]gmodel, []gen.Diagnostic) {
	keep, byModel, diags := analyze(g)
	var out []gmodel
	aliases := gosrc.NewAliases(glueImports)
	alias := aliases.Alias
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
		identOf := map[*gen.Field]string{}
		fieldIdents, _ := gen.DescriptorIdents(m, g)
		for i, f := range m.Fields {
			identOf[f] = fieldIdents[i]
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
				gc := gcol{column: c, f: f, kind: kind, ident: identOf[f]}
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
		gm.rels = exposedRelations(g, m, gm.cols, byModel)
		gm.pkIdent = gm.pk.ident
		out = append(out, gm)
	}
	return out, diags
}

// glueImports are the import aliases the glue's own code uses.
var glueImports = map[string]string{
	"context": "context", "json": "encoding/json", "errors": "errors", "fmt": "fmt", "io": "io",
	"strconv": "strconv", "time": "time", "slices": "slices", "http": "net/http", "strings": "strings",
	"momo":      "github.com/azrtydxb/momo",
	"apiquery":  "github.com/azrtydxb/momo/gen/apiquery",
	"graphql":   "github.com/99designs/gqlgen/graphql",
	"handler":   "github.com/99designs/gqlgen/graphql/handler",
	"extension": "github.com/99designs/gqlgen/graphql/handler/extension",
	"ast":       "github.com/vektah/gqlparser/v2/ast",
	"gqlerror":  "github.com/vektah/gqlparser/v2/gqlerror",
}

// importsOf lists the import aliases the models of ms use, with the glue's own.
func importsOf(ms []gmodel) map[string]string {
	imp := maps.Clone(glueImports)
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
	if !gosrc.PkgName.MatchString(pkg) {
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
	byModel := map[*gen.Model]gmodel{}
	for _, m := range ms {
		byModel[m.m] = m
	}
	for _, m := range ms {
		writeGoModel(&b, m, byModel)
	}
	body := b.String()

	var out strings.Builder
	out.WriteString(gen.Header() + "\n\n")
	out.WriteString("// Package " + pkg + " holds the Go types, list functions and scalars the gqlgen\n// server of the GraphQL schema binds to.\npackage " + pkg + "\n\n")
	out.WriteString(gosrc.Imports(body, importsOf(ms)))
	out.WriteString(body)
	return []byte(out.String()), nil
}

const glueHelpers = `// PageInfo describes the page a connection holds.
type PageInfo struct {
	HasNextPage bool
	EndCursor   *string
}

// The limits that bound what one request may cost. Document them to your
// clients; NewServer enforces the first two, the rest are enforced where the
// request is turned into statements.
const (
	// MaxInValues is the longest list an in operator accepts.
	MaxInValues = 100
	// MaxFilterDepth is the deepest nesting of and, or and not in a filter.
	MaxFilterDepth = 4
	// MaxFilterTerms is the most terms in one and or or list.
	MaxFilterTerms = 20
	// MaxRelationRows is the most rows a to-many relation returns per parent;
	// the rows are the first ones by primary key.
	MaxRelationRows = 100
	// MaxPreloadRows bounds, per request, the rows the selected relations could
	// load: the page size times the rows each parent could have, level by
	// level (a to-many relation counts MaxRelationRows). A request whose worst
	// case is above it is refused before any statement runs.
	MaxPreloadRows = 10000
	// MaxRequestBytes is the largest request body NewHandler reads.
	MaxRequestBytes = 1 << 20
)

// Limits bound one request.
type Limits struct {
	// MaxDepth is the deepest selection a query may have, fragments included.
	MaxDepth int
	// MaxComplexity bounds the fields a query selects (gqlgen counts one each).
	MaxComplexity int
}

// DefaultLimits are the limits NewServer uses.
var DefaultLimits = Limits{MaxDepth: 8, MaxComplexity: 500}

// NewServer returns the gqlgen server of es with DefaultLimits and an error
// presenter that never shows a database error to the client. Use NewHandler,
// which also limits the size of a request, or this, never
// handler.NewDefaultServer, to serve the generated schema.
func NewServer(es graphql.ExecutableSchema) *handler.Server {
	return NewServerWithLimits(es, DefaultLimits)
}

// NewServerWithLimits is NewServer with other limits.
func NewServerWithLimits(es graphql.ExecutableSchema, l Limits) *handler.Server {
	srv := handler.NewDefaultServer(es)
	srv.Use(extension.FixedComplexityLimit(l.MaxComplexity))
	srv.Use(depthLimit(l.MaxDepth))
	srv.SetErrorPresenter(presentError)
	return srv
}

// NewHandler returns NewServer behind an http.MaxBytesReader of MaxRequestBytes.
// Serve the schema with it, not with the bare server.
func NewHandler(es graphql.ExecutableSchema) http.Handler {
	return NewHandlerWithLimits(es, DefaultLimits)
}

// NewHandlerWithLimits is NewHandler with other limits.
func NewHandlerWithLimits(es graphql.ExecutableSchema, l Limits) http.Handler {
	srv := NewServerWithLimits(es, l)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
		srv.ServeHTTP(w, r)
	})
}

// depthLimit is a gqlgen extension that refuses a query nested deeper than its
// value. An introspection query (every top-level field starts with two
// underscores) is exempt from the depth limit, because the standard
// introspection query is deeper than a sensible limit; it still counts toward
// MaxComplexity.
type depthLimit int

func (depthLimit) ExtensionName() string                  { return "DepthLimit" }
func (depthLimit) Validate(graphql.ExecutableSchema) error { return nil }

func (d depthLimit) InterceptOperation(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	if oc := graphql.GetOperationContext(ctx); oc != nil && oc.Operation != nil && !onlyIntrospection(oc.Operation.SelectionSet) &&
		selectionDepth(oc.Operation.SelectionSet, map[string]int{}) > int(d) {
		return func(ctx context.Context) *graphql.Response {
			return graphql.ErrorResponse(ctx, "query is too deep: at most %d levels", int(d))
		}
	}
	return next(ctx)
}

// onlyIntrospection reports whether every top-level field of set is an
// introspection field.
func onlyIntrospection(set ast.SelectionSet) bool {
	if len(set) == 0 {
		return false
	}
	for _, sel := range set {
		f, ok := sel.(*ast.Field)
		if !ok || !strings.HasPrefix(f.Name, "__") {
			return false
		}
	}
	return true
}

// selectionDepth is the depth of set. memo holds the depth of each fragment
// already measured, so a fragment spread many times is measured once.
func selectionDepth(set ast.SelectionSet, memo map[string]int) int {
	deepest := 0
	for _, sel := range set {
		d := 0
		switch s := sel.(type) {
		case *ast.Field:
			d = 1 + selectionDepth(s.SelectionSet, memo)
		case *ast.InlineFragment:
			d = selectionDepth(s.SelectionSet, memo)
		case *ast.FragmentSpread:
			if s.Definition != nil {
				if known, ok := memo[s.Name]; ok {
					d = known
				} else {
					d = selectionDepth(s.Definition.SelectionSet, memo)
					memo[s.Name] = d
				}
			}
		}
		deepest = max(deepest, d)
	}
	return deepest
}

// InputError is a request the server refuses; its message is written for the
// client and is shown as it is.
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func invalid(msg string) error { return &InputError{Msg: msg} }

// presentError shows the client a fixed message for an error of the database,
// which would otherwise name the operation, the table and the constraint. The
// errors of gqlgen itself (parse, validation, complexity) and InputError keep
// their text; every other error, wrapped or not, is mapped by its sentinel.
func presentError(ctx context.Context, err error) *gqlerror.Error {
	out := graphql.DefaultErrorPresenter(ctx, err)
	var input *InputError
	var gqlErr *gqlerror.Error
	// An error of gqlgen itself (parse, validation, complexity) is a gqlerror
	// with no cause, and its text is for the client. An error of a resolver is
	// wrapped in a gqlerror whose message is the text of its cause, which names
	// the table and the constraint: it is mapped below, never passed through.
	if errors.As(err, &gqlErr) && gqlErr.Err == nil {
		return out
	}
	if errors.As(err, &input) {
		out.Message = input.Msg
		return out
	}
	code, msg := "INTERNAL", "internal error"
	switch {
	case errors.Is(err, momo.ErrNotFound):
		code, msg = "NOT_FOUND", "not found"
	case errors.Is(err, momo.ErrUniqueViolation):
		code, msg = "CONFLICT", "a unique constraint refused the change"
	case errors.Is(err, momo.ErrForeignKeyViolation):
		code, msg = "CONFLICT", "a foreign key constraint refused the change"
	case errors.Is(err, momo.ErrCheckViolation), errors.Is(err, momo.ErrNotNullViolation):
		code, msg = "BAD_INPUT", "a constraint refused the value"
	case errors.Is(err, momo.ErrBusy), errors.Is(err, momo.ErrSerialization):
		code, msg = "UNAVAILABLE", "temporarily unavailable; retry"
	case errors.Is(err, momo.ErrValidation):
		code, msg = "BAD_INPUT", "invalid input"
	}
	out.Message = msg
	out.Extensions = map[string]any{"code": code}
	return out
}

type lookupFunc func(name string) (momo.ColumnRef, error)

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

// combine joins the predicates of the children of a filter with join. The list
// is bounded, and the caller bounds the nesting.
func combine[F any](children []*F, pred func(*F) (momo.Predicate, error), join func(...momo.Predicate) momo.Predicate) (momo.Predicate, error) {
	if len(children) > MaxFilterTerms {
		return nil, invalid("a filter list has too many terms")
	}
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
		if len(o.in) > MaxInValues {
			return nil, invalid("in takes at most 100 values")
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

// fieldsAt returns the fields the request selects below the field being
// resolved, following path. Every field of a name on the path counts, aliases
// included, so their selections are merged. Outside a gqlgen request it returns
// nil, so no relation is loaded.
func fieldsAt(ctx context.Context, path ...string) []graphql.CollectedField {
	fc := graphql.GetFieldContext(ctx)
	if fc == nil {
		return nil
	}
	oc := graphql.GetOperationContext(ctx)
	fields := graphql.CollectFields(oc, fc.Field.Selections, nil)
	for _, name := range path {
		var next []graphql.CollectedField
		for _, f := range fields {
			if f.Name == name {
				next = append(next, graphql.CollectFields(oc, f.Selections, nil)...)
			}
		}
		if next == nil {
			return nil
		}
		fields = next
	}
	return fields
}

// fieldGroup is every selection of one field name, with the sub-selections of
// all of them: a: posts { id } b: posts { comments { id } } is one group.
type fieldGroup struct {
	Name string
	Subs []graphql.CollectedField
}

// groupFields groups fields by name, in the order of first appearance.
func groupFields(ctx context.Context, fields []graphql.CollectedField) []fieldGroup {
	var groups []fieldGroup
	for _, f := range fields {
		subs := subFields(ctx, f)
		i := slices.IndexFunc(groups, func(g fieldGroup) bool { return g.Name == f.Name })
		if i < 0 {
			groups = append(groups, fieldGroup{Name: f.Name, Subs: subs})
			continue
		}
		groups[i].Subs = append(groups[i].Subs, subs...)
	}
	return groups
}

// checkPreloadBudget refuses a request whose selected relations could load more
// than MaxPreloadRows: rows parents, each with perRow relation rows at most.
func checkPreloadBudget(rows, perRow int) error {
	if perRow > 0 && rows*perRow > MaxPreloadRows {
		return invalid("the selected relations could load too many rows; ask for fewer rows or fewer nested relations")
	}
	return nil
}

// subFields returns the fields the request selects below f.
func subFields(ctx context.Context, f graphql.CollectedField) []graphql.CollectedField {
	return graphql.CollectFields(graphql.GetOperationContext(ctx), f.Selections, nil)
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
	case scStringEq:
		return r.Replace(`
// StringEqFilter is the filter input of a text column that cannot be searched
// with contains: a nullable one.
type StringEqFilter struct {
	Eq *string
	Ne *string
	In []string
}

func (f *StringEqFilter) predicate(lookup lookupFunc, col string) (momo.Predicate, error) {
	if f == nil {
		return nil, nil
	}
	ps, err := cmpOps[string]{eq: f.Eq, ne: f.Ne, in: f.In}.predicates(lookup, col, func(s string) string { return s })
	if err != nil {
		return nil, err
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

func writeGoModel(b *strings.Builder, gm gmodel, all map[*gen.Model]gmodel) {
	m := gm.m
	pkCol := gm.pk.name
	r := strings.NewReplacer(
		"{{M}}", m.GoName,
		"{{lm}}", gosrc.LowerFirst(m.GoName),
		"{{models}}", gm.modelAlias,
		"{{desc}}", gm.descAlias,
		"{{PK}}", gm.pk.f.GoName,
		"{{PKI}}", gm.pk.ident,
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
func (f *{{M}}Filter) predicate(depth int) (momo.Predicate, error) {
	if f == nil {
		return nil, nil
	}
	if depth > MaxFilterDepth {
		return nil, invalid("a filter is nested too deep")
	}
	nested := func(c *{{M}}Filter) (momo.Predicate, error) { return c.predicate(depth + 1) }
	steps := []func() (momo.Predicate, error){
		func() (momo.Predicate, error) { return combine(f.And, nested, momo.And) },
		func() (momo.Predicate, error) { return combine(f.Or, nested, momo.Or) },
		func() (momo.Predicate, error) {
			p, err := f.Not.predicate(depth + 1)
			if err != nil || p == nil {
				return nil, err
			}
			return momo.Not(p), nil
		},
`))
	for _, c := range gm.cols {
		call := "f." + c.f.GoName + ".predicate({{desc}}.ColumnByName, " + strconv.Quote(c.name)
		switch {
		case c.sc == scString:
			call += ", func(s string) momo.Predicate { return momo.ContainsLiteral({{desc}}." + c.ident + ", s) })"
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
`))
	b.WriteString(r.Replace(`
// {{lm}}Loaders returns the loaders of the relations the request selects on {{M}}
// rows, nested as deep as the request goes. Each relation costs one statement
// per level however many rows the page has, and a to-many relation returns at
// most MaxRelationRows rows per parent.
func {{lm}}Loaders(ctx context.Context, fields []graphql.CollectedField) []momo.Loader[{{models}}.{{M}}] {
`))
	if len(gm.rels) == 0 {
		b.WriteString("\treturn nil\n}\n\n")
	} else {
		b.WriteString(r.Replace(`	var loaders []momo.Loader[{{models}}.{{M}}]
	for _, grp := range groupFields(ctx, fields) {
		switch grp.Name {
`))
		for _, rel := range gm.rels {
			t := all[rel.target]
			opts := ""
			if rel.r.Kind == gen.HasMany || rel.r.Kind == gen.ManyToMany {
				opts = "momo.LoadOrderBy[" + t.modelAlias + "." + t.m.GoName + "](" + t.descAlias + "." + t.pkIdent + ".Asc()), momo.LoadPerParentLimit[" + t.modelAlias + "." + t.m.GoName + "](MaxRelationRows)"
			}
			b.WriteString(r.Replace("\tcase " + strconv.Quote(gosrc.LowerFirst(rel.r.GoName)) + ":\n\t\t\tloaders = append(loaders, {{desc}}." + rel.ident + ".Scope(" + opts + ").Then(" + gosrc.LowerFirst(t.m.GoName) + "Loaders(ctx, grp.Subs)...))\n"))
		}
		b.WriteString("\t\t}\n\t}\n\treturn loaders\n}\n\n")
	}

	b.WriteString(r.Replace(`// {{lm}}Cost is the most rows the relations selected on one {{M}} could load,
// nested as deep as the request goes: a to-many relation counts MaxRelationRows.
func {{lm}}Cost(ctx context.Context, fields []graphql.CollectedField) int {
`))
	if len(gm.rels) == 0 {
		b.WriteString("\treturn 0\n}\n\n")
	} else {
		b.WriteString("\ttotal := 0\n\tfor _, grp := range groupFields(ctx, fields) {\n\t\tswitch grp.Name {\n")
		for _, rel := range gm.rels {
			t := all[rel.target]
			mult := "1"
			if rel.r.Kind == gen.HasMany || rel.r.Kind == gen.ManyToMany {
				mult = "MaxRelationRows"
			}
			b.WriteString("\t\tcase " + strconv.Quote(gosrc.LowerFirst(rel.r.GoName)) + ":\n\t\t\ttotal += " + mult + " * (1 + " + gosrc.LowerFirst(t.m.GoName) + "Cost(ctx, grp.Subs))\n")
		}
		b.WriteString("\t\t}\n\t}\n\treturn total\n}\n\n")
	}

	b.WriteString(r.Replace(`
// {{lm}}OrderColumns maps an order field value to its column.
var {{lm}}OrderColumns = map[string]string{
`))
	for _, c := range gm.cols {
		if orderable(c.column) {
			b.WriteString("\t" + strconv.Quote(c.enum) + ": " + strconv.Quote(c.name) + ",\n")
		}
	}
	b.WriteString(r.Replace(`}

// {{lm}}Key formats the order key of column col of d; false means col cannot be
// an order key.
func {{lm}}Key(col string, d *{{models}}.{{M}}) (string, bool) {
	switch col {
`))
	for _, c := range gm.cols {
		if !orderable(c.column) {
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
		if !orderable(c.column) {
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
		items = append(items, {{desc}}.{{PKI}}.Asc())
	}
	sel := momo.Select[{{models}}.{{M}}](q).OrderBy(items...).Limit(int64(limit + 1))
	if filter != nil {
		p, err := filter.predicate(0)
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
		b.WriteString(r.Replace("\tif err := checkPreloadBudget(limit, {{lm}}Cost(ctx, fieldsAt(ctx, \"edges\", \"node\"))); err != nil {\n\t\treturn nil, err\n\t}\n\tif loaders := {{lm}}Loaders(ctx, fieldsAt(ctx, \"edges\", \"node\")); len(loaders) > 0 {\n\t\tsel = sel.Preload(loaders...)\n\t}\n"))
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
	sel := momo.Select[{{models}}.{{M}}](q).Where({{desc}}.{{PKI}}.Eq(id))
`))
	if len(gm.rels) > 0 {
		b.WriteString(r.Replace("\tif err := checkPreloadBudget(1, {{lm}}Cost(ctx, fieldsAt(ctx))); err != nil {\n\t\treturn nil, err\n\t}\n\tif loaders := {{lm}}Loaders(ctx, fieldsAt(ctx)); len(loaders) > 0 {\n\t\tsel = sel.Preload(loaders...)\n\t}\n"))
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
		"ID":             {importPath + "/model.ID64", "github.com/99designs/gqlgen/graphql.ID"},
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
	b.WriteString("resolver:\n  layout: follow-schema\n  dir: .\n  package: graph\n  filename: resolver.go\n  filename_template: \"{name}.resolvers.go\"\n\n")
	b.WriteString("models:\n")
	for _, n := range names {
		b.WriteString("  " + n + ":\n    model:\n")
		for _, mod := range models[n] {
			b.WriteString("      - " + mod + "\n")
		}
	}
	return []byte(b.String())
}

// resolverScaffold returns the starting points of the resolver package: the
// root resolver (resolver.go) and the resolvers of the generated queries
// (schema.resolvers.go), which delegate to the list and get functions. gqlgen
// follows the schema layout: it never touches resolver.go once it exists and
// rewrites schema.resolvers.go keeping the bodies of the resolvers.
func resolverScaffold(ms []gmodel, pkg, importPath string) (root, queries []byte) {
	root = []byte("// SPDX-License-Identifier: Apache-2.0\n\npackage " + pkg + `

import "github.com/azrtydxb/momo"

// Resolver is the root resolver. It is yours to edit: momo generate and gqlgen
// never overwrite this file. Serve the schema with model.NewHandler, which
// limits the request size, the depth and the complexity and presents errors; the
// resolvers do no authentication or authorization, so wrap the handler.
type Resolver struct {
	DB *momo.DB
}
`)
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
// Query returns the generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
` + b.String()
	imp := map[string]string{
		"context":   "context",
		"generated": importPath + "/generated",
		"model":     importPath + "/model",
	}
	for _, m := range ms {
		mp, _ := gosrc.ModelImport(m.m)
		imp[m.modelAlias] = mp
	}
	queries = []byte("package " + pkg + "\n\n" + gosrc.Imports(body, imp) + strings.TrimPrefix(body, "\n"))
	return root, queries
}
