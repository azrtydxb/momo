// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/apiquery"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/schema"
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

// jsonKeys returns, per dto struct of the generated source, the JSON keys of
// its fields.
func jsonKeys(t *testing.T, src []byte) map[string][]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "handlers_gen.go", src, 0)
	if err != nil {
		t.Fatalf("generated handlers do not parse: %v\n%s", err, src)
	}
	out := map[string][]string{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, s := range gd.Specs {
			ts, ok := s.(*ast.TypeSpec)
			if !ok || !strings.HasPrefix(ts.Name.Name, "dto") {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, fl := range st.Fields.List {
				tag := reflect.StructTag(strings.Trim(fl.Tag.Value, "`")).Get("json")
				out[strings.TrimPrefix(ts.Name.Name, "dto")] = append(out[strings.TrimPrefix(ts.Name.Name, "dto")], tag)
			}
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// Catches: the document and the generated handlers disagreeing on the fields
// of a model, or a handler that skips the allowlist, the body limit or the
// typed builders.
func TestHandlersMatchDocument(t *testing.T) {
	g := exampleGraph(t)
	opts := Options{Title: "momo example", Version: "1.0.0", BasePath: "/v1"}
	doc, ds := Document(g, opts)
	if len(ds) != 0 {
		t.Fatalf("document diagnostics: %v", ds)
	}
	src, hds := Handlers(g, "handlers", opts)
	if len(hds) != 0 {
		t.Fatalf("handler diagnostics: %v", hds)
	}
	var d struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	keys := jsonKeys(t, src)
	for _, model := range []string{"Comment", "Document", "Tag", "Tenant"} {
		var want []string
		for k := range d.Components.Schemas[model].Properties {
			want = append(want, k)
		}
		sort.Strings(want)
		if !reflect.DeepEqual(keys[model], want) {
			t.Errorf("%s: handler keys %v, document properties %v", model, keys[model], want)
		}
	}
	s := string(src)
	for _, must := range []string{
		`mux.HandleFunc("GET /v1/documents", h.list)`,
		`mux.HandleFunc("POST /v1/documents", h.create)`,
		`mux.HandleFunc("GET /v1/documents/{id}", h.get)`,
		`mux.HandleFunc("PUT /v1/documents/{id}", h.update)`,
		`mux.HandleFunc("DELETE /v1/documents/{id}", h.delete)`,
		"http.MaxBytesReader(w, r.Body, maxBodyBytes)",
		"const maxBodyBytes = 1 << 20",
		"apiquery.ParseFilter(documentdesc.ColumnByName",
		"apiquery.ClampLimit",
		"apiquery.DecodeCursor",
		"momo.Select[example.Document](h.DB)",
		"momo.Insert[example.Document](h.DB)",
		"momo.Update[example.Document](h.DB)",
		"momo.Delete[example.Document](h.DB)",
		"ins.Omit(documentdesc.Embedding)",
		"DisallowUnknownFields",
		"StatusUnsupportedMediaType",
		"StatusRequestEntityTooLarge",
		"momo.ErrUniqueViolation",
		"StatusServiceUnavailable",
		"missing required property",
		"ins.Omit(documentdesc.Status)",
		"do no authentication or\n// authorization: wrap the mux",
	} {
		if !strings.Contains(s, must) {
			t.Errorf("handlers lack %q", must)
		}
	}
	if strings.Contains(s, ".With(") || strings.Contains(s, "momo.Raw") {
		t.Error("handlers must use the typed builders only")
	}
	again, _ := Handlers(g, "handlers", opts)
	if string(again) != s {
		t.Error("Handlers is not deterministic")
	}
}

func handlerModel(name, table string, fields ...*gen.Field) *gen.Model {
	return &gen.Model{GoName: name, Table: table, PkgPath: "example.com/m", PkgName: "m", SubPkg: strings.ToLower(name), Fields: fields}
}

func hf(goName, col, goType string, opts ...func(*gen.Field)) *gen.Field {
	f := &gen.Field{GoName: goName, Column: col, GoType: goType, BaseType: strings.TrimPrefix(goType, "*"), Path: []int{0}, Nullable: strings.HasPrefix(goType, "*")}
	for _, o := range opts {
		o(f)
	}
	return f
}

// Catches: a sensitive column, a composite-key model or an unsupported type
// reaching the handlers.
func TestHandlersRules(t *testing.T) {
	thing := handlerModel("Thing", "things",
		hf("ID", "id", "int64"),
		hf("Secret", "secret", "string", func(f *gen.Field) { f.Tags = tags.Tags{Sensitive: true} }),
		hf("Note", "note", "*string"),
	)
	pair := handlerModel("Pair", "pairs", hf("A", "a", "int32"), hf("B", "b", "int32"))
	g := &gen.Graph{Models: []*gen.Model{thing, pair}, Schema: &schema.Schema{Tables: []schema.Table{
		{Name: "things", PrimaryKey: []string{"id"}},
		{Name: "pairs", PrimaryKey: []string{"a", "b"}},
	}}}
	src, diags := Handlers(g, "api", Options{Title: "t", Version: "1"})
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "Pair") {
		t.Fatalf("want one diagnostic for the skipped Pair, got %v", diags)
	}
	if !strings.Contains(string(src), "// Not served: Pair (no single exposed int64 primary key)") {
		t.Errorf("the skipped model is not named in the output:\n%s", src)
	}
	s := string(src)
	if strings.Contains(s, "secret") || strings.Contains(s, "Secret") {
		t.Errorf("sensitive column in the handlers:\n%s", s)
	}
	if strings.Contains(s, "PairHandler") {
		t.Errorf("a composite-key model has handlers:\n%s", s)
	}
	if !strings.Contains(s, "package api") || !strings.Contains(s, `mux.HandleFunc("GET /things/{id}", h.get)`) {
		t.Errorf("package or routes wrong:\n%s", s)
	}

	bad := handlerModel("Bad", "bad", hf("ID", "id", "int64"), hf("Odd", "odd", "sql.NullString"))
	_, diags = Handlers(&gen.Graph{Models: []*gen.Model{bad}, Schema: &schema.Schema{Tables: []schema.Table{{Name: "bad", PrimaryKey: []string{"id"}}}}}, "api", Options{})
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "do not support the type sql.NullString") {
		t.Errorf("unsupported type diags = %v", diags)
	}
	if _, diags = Handlers(g, "Bad Name", Options{}); len(diags) != 1 {
		t.Errorf("bad package accepted: %v", diags)
	}
	for _, bp := range []string{"v1", "/a b", "/a/../b", "/{id}", "/v1?x"} {
		if src, diags = Handlers(g, "api", Options{BasePath: bp}); src != nil || len(diags) != 1 {
			t.Errorf("base path %q accepted: %v", bp, diags)
		}
	}
	if src, diags = Handlers(g, "api", Options{Ops: apiquery.Ops{"*": {"drop"}}}); src != nil || len(diags) != 1 {
		t.Errorf("unknown operation accepted: %v", diags)
	}
}

// Catches: a plugin that writes handlers without being asked, a document file
// without the generated header, or a YAML document that differs from the JSON
// one.
func TestPluginGenerate(t *testing.T) {
	g := exampleGraph(t)
	root := t.TempDir()
	opts := Options{Title: "momo example", Version: "1.0.0", BasePath: "/v1"}

	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	if err := (Plugin{Dir: filepath.Join(root, "api"), Options: opts}).Generate(g, w); err != nil {
		t.Fatal(err)
	}
	outs := w.Outputs()
	if len(outs) != 1 || filepath.Base(outs[0].Path) != "openapi.yaml" {
		t.Fatalf("outputs = %v", outs)
	}
	if !strings.HasPrefix(string(outs[0].Data), "# Code generated by momo ") {
		t.Errorf("no generated header:\n%s", outs[0].Data)
	}
	doc, _ := Document(g, opts)
	var fromJSON, fromYAML any
	if err := json.Unmarshal(doc, &fromJSON); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(outs[0].Data, &fromYAML); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(fromJSON)
	b, _ := json.Marshal(fromYAML)
	if string(a) != string(b) {
		t.Errorf("YAML document differs from the JSON one:\n%s\n%s", a, b)
	}

	opts.Handlers = true
	w = gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	p := Plugin{Dir: filepath.Join(root, "api"), Options: opts}
	if err := p.Generate(g, w); err != nil {
		t.Fatal(err)
	}
	if len(w.Outputs()) != 2 || filepath.Base(w.Outputs()[0].Path) != "handlers_gen.go" {
		t.Errorf("outputs = %v", w.Outputs())
	}
	if !strings.Contains(string(w.Outputs()[0].Data), "package api") {
		t.Errorf("package defaults to the directory name:\n%s", w.Outputs()[0].Data)
	}
	if p.Name() != "openapi" || p.APIVersion() != gen.PluginAPIVersion {
		t.Errorf("name %q version %d", p.Name(), p.APIVersion())
	}
}

// Catches: an operation that was switched off still being routed or documented,
// and a read-only API that still writes.
func TestHandlersOps(t *testing.T) {
	g := exampleGraph(t)
	opts := Options{Title: "t", Version: "1", BasePath: "/v1", Ops: apiquery.Ops{apiquery.DefaultModel: {"list", "get"}, "Tag": {"list", "create", "delete"}}}
	src, diags := Handlers(g, "handlers", opts)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	s := string(src)
	for _, no := range []string{
		`"POST /v1/documents"`, `"PUT /v1/documents/{id}"`, `"DELETE /v1/documents/{id}"`,
		`"GET /v1/tags/{id}"`, `"PUT /v1/tags/{id}"`,
		"func (h *DocumentHandler) create", "func (h *DocumentHandler) update", "func (h *DocumentHandler) delete",
	} {
		if strings.Contains(s, no) {
			t.Errorf("handlers contain %s although the operation is off", no)
		}
	}
	for _, yes := range []string{`"GET /v1/documents"`, `"GET /v1/documents/{id}"`, `"POST /v1/tags"`, `"DELETE /v1/tags/{id}"`} {
		if !strings.Contains(s, yes) {
			t.Errorf("handlers lack %s", yes)
		}
	}
	doc, ds := Document(g, opts)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	var d struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Paths["/v1/documents"]["post"]; ok {
		t.Error("the document lists POST /v1/documents")
	}
	if _, ok := d.Paths["/v1/documents/{id}"]["put"]; ok {
		t.Error("the document lists PUT /v1/documents/{id}")
	}
	if _, ok := d.Paths["/v1/tags/{id}"]["get"]; ok {
		t.Error("the document lists GET /v1/tags/{id}")
	}
	if _, ok := d.Paths["/v1/tags/{id}"]["delete"]; !ok {
		t.Error("the document lacks DELETE /v1/tags/{id}")
	}
}

// Catches: identity, key, read-only and defaulted columns counted as required
// in the input schema, which would make a client send the id and the creation
// time, and a required column that is not enforced.
func TestRequiredProperties(t *testing.T) {
	g := exampleGraph(t)
	doc, ds := Document(g, Options{Title: "t", Version: "1", BasePath: "/v1"})
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	var d struct {
		Components struct {
			Schemas map[string]struct {
				Required []string `json:"required"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"Comment": {"document_id", "body"}, "Document": {"tenant_id", "title", "body"},
		"Tag": {"name"}, "Tenant": {"name"},
	}
	for model, req := range want {
		if !reflect.DeepEqual(d.Components.Schemas[model+"Input"].Required, req) {
			t.Errorf("%sInput required = %v, want %v", model, d.Components.Schemas[model+"Input"].Required, req)
		}
	}
	src, _ := Handlers(g, "handlers", Options{Title: "t", Version: "1"})
	if !strings.Contains(string(src), `var documentRequired = []string{"tenant_id", "title", "body"}`) {
		t.Errorf("the handlers do not enforce the same required set:\n%s", src)
	}
}

// Catches: a plugin that leaves a stale handlers_gen.go serving routes the
// document no longer lists, or that fails the whole run for one bad model.
func TestPluginStaleHandlersAndSkippedModel(t *testing.T) {
	g := exampleGraph(t)
	root := t.TempDir()
	dir := filepath.Join(root, "api")
	opts := Options{Title: "t", Version: "1", Handlers: true}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	if err := (Plugin{Dir: dir, Options: opts}).Generate(g, w); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	opts.Handlers = false
	w = gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	err := (Plugin{Dir: dir, Options: opts}).Generate(g, w)
	var de *gen.ErrDiagnostics
	if !errors.As(err, &de) || !strings.Contains(err.Error(), "handlers_gen.go") {
		t.Fatalf("a stale handlers_gen.go was not reported: %v", err)
	}

	bad := &gen.Graph{Models: append(append([]*gen.Model(nil), g.Models...), handlerModel("Pair", "pairs", hf("A", "a", "int32"), hf("B", "b", "int32"))), Schema: g.Schema}
	w = gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	opts.Handlers = true
	if err := (Plugin{Dir: filepath.Join(root, "other"), Options: opts}).Generate(bad, w); err != nil {
		t.Fatalf("one bad model failed the whole run: %v", err)
	}
	for _, o := range w.Outputs() {
		if !strings.Contains(string(o.Data), "Pair") {
			t.Errorf("%s does not name the skipped model", o.Path)
		}
	}
}

// Catches: a PUT that reads a row, merges and writes every column back, which
// loses a concurrent update of a column the request did not carry, and a key
// matched case-insensitively and then ignored.
func TestHandlersUpdateWritesOnlyCarriedKeys(t *testing.T) {
	src, diags := Handlers(exampleGraph(t), "handlers", Options{Title: "t", Version: "1"})
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s := string(src)
	i := strings.Index(s, "func (h *DocumentHandler) update(")
	if i < 0 {
		t.Fatal("no update handler")
	}
	upd := s[i:]
	if j := strings.Index(upd, "\nfunc (h *DocumentHandler) delete("); j > 0 {
		upd = upd[:j]
	}
	for _, must := range []string{
		`if _, ok := keys["title"]; ok {`,
		"documentdesc.Title.Set(in.Title)",
		".Set(sets...)",
	} {
		if !strings.Contains(upd, must) {
			t.Errorf("update lacks %q:\n%s", must, upd)
		}
	}
	if strings.Contains(upd, ".Model(&m)") || strings.Contains(upd, "in.apply(&m, keys)") {
		t.Errorf("update still writes the whole row:\n%s", upd)
	}
	if !strings.Contains(s, "unknown property") {
		t.Error("a key that is not an exact column name is not refused")
	}
}

// Catches: the PUT rules only in the summary, which clients do not show.
func TestPutDescriptionStatesTheRules(t *testing.T) {
	doc, diags := Document(exampleGraph(t), Options{Title: "t", Version: "1", BasePath: "/v1"})
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	var d struct {
		Paths map[string]map[string]struct {
			Summary, Description string
		} `json:"paths"`
	}
	if err := json.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	put := d.Paths["/v1/documents/{id}"]["put"]
	if !strings.Contains(put.Description, "keeps its stored value") || !strings.Contains(put.Description, "every required property") {
		t.Errorf("PUT description = %q", put.Description)
	}
	if len(put.Summary) > 60 {
		t.Errorf("the summary should be short, got %q", put.Summary)
	}
}
