// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"encoding/json"
	"flag"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/schema"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Catches: any change to the document generated for the example models,
// which the golden file records after review.
func TestOpenAPIDocumentGolden(t *testing.T) {
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	doc, ds := Document(g, Options{Title: "momo example", Version: "1.0.0", BasePath: "/v1"})
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	ok, err := golden.Diff("testdata/example.openapi.json", doc, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("document differs from golden; got:\n%s\nreview and rerun with -update", doc)
	}
}

func syntheticGraph() *gen.Graph {
	m := &gen.Model{
		GoName: "Thing", Table: "things",
		Fields: []*gen.Field{
			{GoName: "ID", Column: "id", BaseType: "int64"},
			{GoName: "Secret", Column: "secret", BaseType: "string", Tags: tags.Tags{Sensitive: true}},
			{GoName: "Note", Column: "note", BaseType: "string", Nullable: true},
			{GoName: "Emb", Column: "emb", BaseType: "vector.Vector"},
			{GoName: "Odd", Column: "odd", BaseType: "map[string]string"},
		},
	}
	pair := &gen.Model{
		GoName: "Pair", Table: "pairs",
		Fields: []*gen.Field{{GoName: "A", Column: "a", BaseType: "int32"}, {GoName: "B", Column: "b", BaseType: "int32"}},
	}
	return &gen.Graph{
		Models: []*gen.Model{m, pair},
		Schema: &schema.Schema{Tables: []schema.Table{
			{Name: "things", PrimaryKey: []string{"id"}},
			{Name: "pairs", PrimaryKey: []string{"a", "b"}},
		}},
	}
}

// Catches: the OpenAPI 3.0 nullable keyword, sensitive or unmapped columns in
// the schema, a vector offered as a filter, or an {id} path for a composite
// key.
func TestDocumentRules(t *testing.T) {
	raw, diags := Document(syntheticGraph(), Options{Title: "t", Version: "1", BasePath: "/api/"})
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "no OpenAPI mapping for map[string]string") {
		t.Fatalf("diags = %v", diags)
	}
	s := string(raw)
	if strings.Contains(s, `"nullable"`) || strings.Contains(s, "secret") || strings.Contains(s, `"odd"`) {
		t.Errorf("document has nullable, a sensitive or an unmapped column:\n%s", s)
	}
	if strings.Contains(s, "filter[emb]") || !strings.Contains(s, "filter[note]") {
		t.Errorf("filter parameters wrong:\n%s", s)
	}
	var doc struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Errorf("openapi = %q", doc.OpenAPI)
	}
	if _, ok := doc.Paths["/api/things/{id}"]; !ok {
		t.Errorf("no item path for a single key: %v", doc.Paths)
	}
	if _, ok := doc.Paths["/api/pairs/{id}"]; ok {
		t.Errorf("item path for a composite key")
	}
	if !regexp.MustCompile(`"type": \[\s*"string",\s*"null"\s*\]`).MatchString(s) {
		t.Errorf("nullable column lacks the 3.1 type array:\n%s", s)
	}
	if _, d := Document(syntheticGraph(), Options{BasePath: "api"}); len(d) < 3 {
		t.Errorf("missing title, version and bad base path not reported: %v", d)
	}
}
