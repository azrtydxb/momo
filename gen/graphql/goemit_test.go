// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/schema"
)

const (
	testModels = "github.com/azrtydxb/momo/internal/testmodels/example"
	testImport = "example.com/app/graph"
)

func exampleGraph(t *testing.T) *gen.Graph {
	t.Helper()
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	return g
}

// emitWith runs plugin p over g into a writer rooted at root and commits it.
func emitWith(t *testing.T, root string, g *gen.Graph, p Plugin) error {
	t.Helper()
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	if err := p.Generate(g, w); err != nil {
		return err
	}
	_, err := w.Commit()
	return err
}

// Catches: a scaffolded resolver that a second generation overwrites (losing
// handwritten code), or a plugin that skips one of its four outputs.
func TestGraphQLScaffold(t *testing.T) {
	g := exampleGraph(t)
	root := t.TempDir()
	p := Plugin{Dir: filepath.Join(root, "graph"), Package: "graph", ModelsPackage: testModels, ImportPath: testImport}
	if err := emitWith(t, root, g, p); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"schema.graphqls", "gqlgen.yml", "resolver.go", "schema.resolvers.go", filepath.Join("model", "momo_gen.go")} {
		if _, err := os.Stat(filepath.Join(root, "graph", f)); err != nil {
			t.Errorf("the plugin did not write %s: %v", f, err)
		}
	}
	resolver := filepath.Join(root, "graph", "resolver.go")
	if err := os.WriteFile(resolver, []byte("package graph\n// hand edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := emitWith(t, root, g, p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(resolver) // #nosec G304 -- a path under t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "// hand edit") {
		t.Fatal("scaffold overwrote a handwritten resolver")
	}
	if p.Name() != "graphql" || p.APIVersion() != gen.PluginAPIVersion {
		t.Errorf("name %q version %d", p.Name(), p.APIVersion())
	}
}

// Catches: Go glue that does not parse, a list function that does not preload
// the requested relations or skips the allowlist, a config that does not bind
// the scalars, or a scaffold that does not delegate to the glue.
func TestGoSourceExample(t *testing.T) {
	g := exampleGraph(t)
	src, diags := GoSource(g, "model", testModels)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "momo_gen.go", src, 0); err != nil {
		t.Fatalf("glue does not parse: %v\n%s", err, src)
	}
	s := string(src)
	for _, must := range []string{
		"package model",
		"func ListDocuments(ctx context.Context, q momo.Querier, first *int, after *string, filter *DocumentFilter, order *DocumentOrder) (*DocumentConnection, error)",
		"func GetDocument(ctx context.Context, q momo.Querier, id int64) (*example.Document, error)",
		"documentdesc.Comments.Scope(momo.LoadOrderBy[example.Comment](commentdesc.ID.Asc()), momo.LoadPerParentLimit[example.Comment](MaxRelationRows)).Then(commentLoaders(ctx, grp.Subs)...)",
		"documentdesc.Tenant.Scope().Then(tenantLoaders(ctx, grp.Subs)...)",
		`documentLoaders(ctx, fieldsAt(ctx, "edges", "node"))`,
		"sel = sel.Preload(loaders...)",
		"func NewServer(es graphql.ExecutableSchema) *handler.Server",
		"func NewHandler(es graphql.ExecutableSchema) http.Handler",
		"http.MaxBytesReader(w, r.Body, MaxRequestBytes)",
		"errors.As(err, &gqlErr) && gqlErr.Err == nil",
		"selectionDepth(oc.Operation.SelectionSet, map[string]int{})",
		"func onlyIntrospection(set ast.SelectionSet) bool",
		"srv.Use(extension.FixedComplexityLimit(l.MaxComplexity))",
		"srv.SetErrorPresenter(presentError)",
		"MaxInValues = 100",
		"if depth > MaxFilterDepth {",
		"errors.Is(err, momo.ErrUniqueViolation)",
		"apiquery.ClampLimit(limit)",
		"apiquery.DecodeCursor(*after)",
		"apiquery.ParseFilter(lookup",
		"momo.ContainsLiteral(documentdesc.Title, s)",
		"func MarshalInt64(i int64) graphql.Marshaler",
		"func UnmarshalID64(v any) (int64, error)",
		"type StringFilter struct",
		"type TimeFilter struct",
		`"CREATED_AT": "created_at",`,
	} {
		if !strings.Contains(s, must) {
			t.Errorf("glue lacks %q", must)
		}
	}
	if strings.Contains(s, ".With(") || strings.Contains(s, "wanted(") {
		t.Error("relations must load through Preload, never the CTE method With, and through the nested loaders")
	}
	if strings.Contains(s, "type BoolFilter") || strings.Contains(s, "type FloatFilter") {
		t.Error("a filter no column uses was emitted")
	}
	again, _ := GoSource(g, "model", testModels)
	if string(again) != s {
		t.Error("GoSource is not deterministic")
	}

	ms, _ := analyzeGo(g, testModels)
	cfg := string(gqlgenConfig(ms, testImport))
	for _, must := range []string{
		"# Code generated by momo",
		"filename: generated/exec.go",
		"layout: follow-schema",
		"  Document:\n    model:\n      - " + testModels + ".Document\n",
		"  DocumentConnection:\n    model:\n      - example.com/app/graph/model.DocumentConnection\n",
		"  DocumentOrderField:\n    model:\n      - github.com/99designs/gqlgen/graphql.String\n",
		"  ID:\n    model:\n      - example.com/app/graph/model.ID64\n      - github.com/99designs/gqlgen/graphql.ID\n",
		"  Int64:\n    model:\n      - example.com/app/graph/model.Int64\n",
		"      - github.com/99designs/gqlgen/graphql.Time\n",
	} {
		if !strings.Contains(cfg, must) {
			t.Errorf("gqlgen.yml lacks %q:\n%s", must, cfg)
		}
	}

	rootSrc, querySrc := resolverScaffold(ms, "graph", testImport)
	if !strings.Contains(string(rootSrc), "DB *momo.DB") {
		t.Errorf("root resolver lacks the DB field:\n%s", rootSrc)
	}
	if !strings.HasPrefix(string(rootSrc), "// SPDX-License-Identifier: Apache-2.0\n") || !strings.Contains(string(rootSrc), "model.NewHandler") {
		t.Errorf("root resolver lacks its SPDX header or the NewHandler advice:\n%s", rootSrc)
	}
	res := string(querySrc)
	for _, must := range []string{
		"package graph",
		"func (r *Resolver) Query() generated.QueryResolver",
		"func (r *queryResolver) Documents(ctx context.Context, first *int, after *string, filter *model.DocumentFilter, order *model.DocumentOrder) (*model.DocumentConnection, error)",
		"return model.ListDocuments(ctx, r.DB, first, after, filter, order)",
		"func (r *queryResolver) Document(ctx context.Context, id int64) (*example.Document, error)",
		"return model.GetDocument(ctx, r.DB, id)",
	} {
		if !strings.Contains(res, must) {
			t.Errorf("resolver scaffold lacks %q:\n%s", must, res)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "schema.resolvers.go", res, 0); err != nil {
		t.Fatalf("resolver scaffold does not parse: %v\n%s", err, res)
	}
}

func glueModel(fields ...*gen.Field) *gen.Graph {
	m := &gen.Model{GoName: "Thing", Table: "things", PkgPath: "example.com/m", PkgName: "m", SubPkg: "thing", Fields: fields}
	return &gen.Graph{Models: []*gen.Model{m}, Schema: &schema.Schema{Tables: []schema.Table{{Name: "things", PrimaryKey: []string{"id"}}}}}
}

func gf(goName, col, goType string) *gen.Field {
	return &gen.Field{GoName: goName, Column: col, GoType: goType, BaseType: strings.TrimPrefix(goType, "*"), Path: []int{0}, Nullable: strings.HasPrefix(goType, "*")}
}

// Catches: a column the server cannot bind reaching the glue, a model without
// an int64 key, or a models package that does not match.
func TestGoSourceRules(t *testing.T) {
	secret := gf("Secret", "secret", "string")
	secret.Tags = tags.Tags{Sensitive: true}
	g := glueModel(gf("ID", "id", "int64"), secret, gf("Note", "note", "*string"), gf("N", "n", "*int32"))
	src, diags := GoSource(g, "model", "")
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	s := string(src)
	if strings.Contains(s, "Secret") {
		t.Errorf("sensitive column in the glue:\n%s", s)
	}
	if !strings.Contains(s, "Note *StringEqFilter") || !strings.Contains(s, "N *IntFilter") {
		t.Errorf("filter fields missing:\n%s", s)
	}
	// "note" is a nullable text column: contains is not wired up for it, and it
	// cannot be an order key.
	if !strings.Contains(s, `f.Note.predicate(thingdesc.ColumnByName, "note")`) {
		t.Errorf("nullable text column offers contains:\n%s", s)
	}
	if strings.Contains(s, `case "note":`) {
		t.Errorf("nullable column is an order key:\n%s", s)
	}

	_, diags = GoSource(glueModel(gf("ID", "id", "int64"), gf("Small", "small", "int8")), "model", "")
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "cannot bind the type int8") {
		t.Errorf("int8 diags = %v", diags)
	}
	_, diags = GoSource(glueModel(gf("ID", "id", "int32")), "model", "")
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "needs a single int64 primary key") {
		t.Errorf("int32 key diags = %v", diags)
	}
	_, diags = GoSource(g, "model", "example.com/other")
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "not in the configured models package") {
		t.Errorf("models package diags = %v", diags)
	}
	if _, diags = GoSource(g, "Bad Name", ""); len(diags) != 1 {
		t.Errorf("bad package accepted: %v", diags)
	}
}

// Catches: a resolver method name gqlgen would spell differently (an
// initialism) being generated under the wrong name.
func TestHasInitialism(t *testing.T) {
	for in, want := range map[string]bool{"documents": false, "apiKeys": true, "user_id": true, "tenant": false, "urlMaps": true, "grid": false} {
		if got := hasInitialism(in); got != want {
			t.Errorf("hasInitialism(%q) = %v, want %v", in, got, want)
		}
	}
}

func relModel(name, table string, fields ...*gen.Field) *gen.Model {
	return &gen.Model{GoName: name, Table: table, PkgPath: "example.com/m", PkgName: "mod", SubPkg: strings.ToLower(name), Fields: fields}
}

// Catches: relations preloaded one level deep only, so a nested selection
// (thing { parts { subs } }) costs a statement per row at the second level.
func TestGoSourceNestedLoaders(t *testing.T) {
	thing := relModel("Thing", "things", gf("ID", "id", "int64"))
	part := relModel("Part", "parts", gf("ID", "id", "int64"), gf("ThingID", "thing_id", "int64"))
	sub := relModel("Sub", "subs", gf("ID", "id", "int64"), gf("PartID", "part_id", "int64"))
	thing.Relations = []*gen.Relation{{GoName: "Parts", Kind: gen.HasMany, Target: part}}
	part.Relations = []*gen.Relation{{GoName: "Subs", Kind: gen.HasMany, Target: sub}, {GoName: "Thing", Kind: gen.BelongsTo, Target: thing}}
	g := &gen.Graph{Models: []*gen.Model{thing, part, sub}, Schema: &schema.Schema{Tables: []schema.Table{
		{Name: "things", PrimaryKey: []string{"id"}}, {Name: "parts", PrimaryKey: []string{"id"}}, {Name: "subs", PrimaryKey: []string{"id"}},
	}}}
	src, diags := GoSource(g, "model", "")
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	s := string(src)
	for _, must := range []string{
		"func thingLoaders(ctx context.Context, fields []graphql.CollectedField) []momo.Loader[mod.Thing]",
		"thingdesc.Parts.Scope(momo.LoadOrderBy[mod.Part](partdesc.ID.Asc()), momo.LoadPerParentLimit[mod.Part](MaxRelationRows)).Then(partLoaders(ctx, grp.Subs)...)",
		"partdesc.Subs.Scope(momo.LoadOrderBy[mod.Sub](subdesc.ID.Asc()), momo.LoadPerParentLimit[mod.Sub](MaxRelationRows)).Then(subLoaders(ctx, grp.Subs)...)",
		"partdesc.Thing.Scope().Then(thingLoaders(ctx, grp.Subs)...)",
		"func subLoaders(ctx context.Context, fields []graphql.CollectedField) []momo.Loader[mod.Sub] {\n\treturn nil\n}",
	} {
		if !strings.Contains(s, must) {
			t.Errorf("glue lacks %q", must)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "momo_gen.go", src, 0); err != nil {
		t.Fatalf("glue does not parse: %v\n%s", err, src)
	}
}

// Catches: the schema advertising an operation the server rejects: contains on
// a column that is not a plain string, and a nullable column in the order enum.
func TestSDLAdvertisesOnlyWhatTheServerAccepts(t *testing.T) {
	odd := gf("Odd", "odd", "sql.NullString")
	g := glueModel(gf("ID", "id", "int64"), gf("Title", "title", "string"), gf("Note", "note", "*string"), gf("Count", "count", "*int32"), odd)
	sdl, diags := SDL(g)
	_ = diags // the glue refuses sql.NullString; the SDL alone stays valid
	s := string(sdl)
	body, _, _ := strings.Cut(s[strings.Index(s, "input ThingFilter"):], "}\n")
	for field, want := range map[string]string{"title": "StringFilter", "note": "StringEqFilter", "odd": "StringEqFilter", "count": "IntFilter"} {
		if !strings.Contains(body, "  "+field+": "+want+"\n") {
			t.Errorf("filter field %s should be %s:\n%s", field, want, body)
		}
	}
	if i := strings.Index(s, "input StringEqFilter"); i < 0 || strings.Contains(s[i:i+120], "contains") {
		t.Errorf("StringEqFilter missing or offers contains:\n%s", s)
	}
	enum, _, _ := strings.Cut(s[strings.Index(s, "enum ThingOrderField"):], "}\n")
	for _, no := range []string{"NOTE", "COUNT", "ODD"} {
		if strings.Contains(enum, "  "+no+"\n") {
			t.Errorf("nullable column %s is in the order enum:\n%s", no, enum)
		}
	}
	for _, yes := range []string{"ID", "TITLE"} {
		if !strings.Contains(enum, "  "+yes+"\n") {
			t.Errorf("column %s is not in the order enum:\n%s", yes, enum)
		}
	}
}

// Catches: an enum value in the schema that the Go glue cannot order by (it
// would answer "unknown order field") or a glue ordering the schema hides.
func TestEveryAdvertisedOrderFieldWorks(t *testing.T) {
	g := exampleGraph(t)
	sdl, diags := SDL(g)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	glue, diags := GoSource(g, "model", testModels)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	for _, name := range []string{"Comment", "Document", "Tag", "Tenant"} {
		enum, _, _ := strings.Cut(string(sdl)[strings.Index(string(sdl), "enum "+name+"OrderField"):], "}\n")
		var values []string
		for _, line := range strings.Split(enum, "\n")[1:] {
			if v := strings.TrimSpace(line); v != "" {
				values = append(values, v)
			}
		}
		if len(values) == 0 {
			t.Fatalf("%s has no order values", name)
		}
		cols := string(glue)[strings.Index(string(glue), "var "+strings.ToLower(name[:1])+name[1:]+"OrderColumns"):]
		cols, _, _ = strings.Cut(cols, "}\n")
		for _, v := range values {
			if !strings.Contains(cols, `"`+v+`": "`) {
				t.Errorf("%s: order value %s is advertised but the glue cannot order by it:\n%s", name, v, cols)
			}
		}
		if got := strings.Count(cols, `": "`); got != len(values) {
			t.Errorf("%s: the glue orders by %d columns, the schema lists %d", name, got, len(values))
		}
	}
}

// Catches: nested relations bounded per parent only, so a deep selection on a
// big page loads millions of rows, and same-named fields (aliases) whose
// sub-selections are dropped except for the first.
func TestGoSourceBoundsRowsAndMergesAliases(t *testing.T) {
	g := exampleGraph(t)
	src, diags := GoSource(g, "model", testModels)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s := string(src)
	for _, must := range []string{
		"func groupFields(ctx context.Context, fields []graphql.CollectedField) []fieldGroup",
		"func documentCost(ctx context.Context, fields []graphql.CollectedField) int",
		"MaxPreloadRows",
		"checkPreloadBudget(limit, documentCost(ctx, fieldsAt(ctx, \"edges\", \"node\")))",
		"for _, grp := range groupFields(ctx, fields) {",
		"grp.Subs",
	} {
		if !strings.Contains(s, must) {
			t.Errorf("glue lacks %q", must)
		}
	}
	if strings.Contains(s, "seen[f.Name]") {
		t.Error("the loaders keep only the first field of a name")
	}
}
