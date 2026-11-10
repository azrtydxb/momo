// SPDX-License-Identifier: Apache-2.0

package openapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/azrtydxb/momo/gen"
	ggopenapi "github.com/azrtydxb/momo/gen/openapi"
	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/openapi/example/handlers"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

const (
	metaURL    = "https://spec.openapis.org/oas/3.1/schema/2022-10-07"
	dialectURL = "https://spec.openapis.org/oas/3.1/dialect/base"
)

// Catches: a document that is not valid OpenAPI 3.1, a committed document or
// set of handlers that is stale, handlers whose response keys differ from the
// schema properties of the document, a filter that skips the allowlist, and a
// create, read, update or delete that does not round trip over PostgreSQL.
func TestOpenAPIGenerator(t *testing.T) {
	models, err := filepath.Abs("../../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: models})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	opts := ggopenapi.Options{Title: "momo example", Version: "1.0.0", BasePath: "/v1", Handlers: true}
	doc, ds := ggopenapi.Document(g, opts)
	if len(ds) != 0 {
		t.Fatalf("document diagnostics: %v", ds)
	}

	// 1. the committed document is current
	ok, err := golden.Diff("../../../gen/openapi/testdata/example.openapi.json", doc, false)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("committed OpenAPI document is stale; regenerate it")
	}

	// 2. the document is valid OpenAPI 3.1 against the official meta-schema
	c := jsonschema.NewCompiler()
	for url, file := range map[string]string{dialectURL: "testdata/oas31-dialect-base.json", metaURL: "testdata/oas31-schema.json"} {
		raw, err := os.ReadFile(file) // #nosec G304 -- a vendored test fixture
		if err != nil {
			t.Fatal(err)
		}
		schema, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.AddResource(url, schema); err != nil {
			t.Fatal(err)
		}
	}
	meta, err := c.Compile(metaURL)
	if err != nil {
		t.Fatal(err)
	}
	doc31, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Validate(doc31); err != nil {
		t.Fatalf("document is not OpenAPI 3.1: %v", err)
	}

	// 3. the committed plugin output (document and handlers) is current
	modRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	outDir, err := filepath.Abs("example/handlers")
	if err != nil {
		t.Fatal(err)
	}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: modRoot})
	if err := (ggopenapi.Plugin{Dir: outDir, Package: "handlers", Options: opts}).Generate(g, w); err != nil {
		t.Fatal(err)
	}
	outs := w.Outputs()
	if len(outs) != 2 {
		t.Fatalf("plugin wrote %d files, want openapi.yaml and handlers_gen.go", len(outs))
	}
	for _, o := range outs {
		ok, err := golden.Diff(o.Path, o.Data, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%s is stale; regenerate it with momo generate", o.Path)
		}
	}

	// 4. the handlers over HTTP, backed by PostgreSQL
	db := testopen.Postgres(t)
	fixture.Apply(t, db)
	mux := http.NewServeMux()
	(&handlers.DocumentHandler{DB: db}).Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/v1/documents", "application/json",
		strings.NewReader(`{"tenant_id":1,"title":"openapi","status":"draft"}`))
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	decodeClose(t, resp, http.StatusCreated, &created)
	id := fmt.Sprint(created["id"])

	var got map[string]any
	decodeClose(t, mustGet(t, srv.URL+"/v1/documents/"+id), http.StatusOK, &got)
	want := schemaProperties(t, doc, "Document")
	if have := keysOf(got); !reflect.DeepEqual(have, want) {
		t.Fatalf("GET keys %v do not match Document schema properties %v", have, want)
	}

	// update: the whole document goes back, with a new title
	got["title"] = "openapi v2"
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	put, err := http.NewRequest(http.MethodPut, srv.URL+"/v1/documents/"+id, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	presp, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	var updated map[string]any
	decodeClose(t, presp, http.StatusOK, &updated)
	if updated["title"] != "openapi v2" {
		t.Fatalf("PUT returned %v", updated)
	}

	var list struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"next_cursor"`
	}
	decodeClose(t, mustGet(t, srv.URL+"/v1/documents?filter[title]=openapi%20v2&limit=10"), http.StatusOK, &list)
	if len(list.Items) != 1 {
		t.Fatalf("filtered list has %d items, want 1", len(list.Items))
	}

	// keyset paging: a page of two, then the next page, without overlap
	var page1, page2 struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"next_cursor"`
	}
	decodeClose(t, mustGet(t, srv.URL+"/v1/documents?limit=2"), http.StatusOK, &page1)
	if len(page1.Items) != 2 || page1.NextCursor == nil {
		t.Fatalf("first page = %v cursor=%v", page1.Items, page1.NextCursor)
	}
	decodeClose(t, mustGet(t, srv.URL+"/v1/documents?limit=2&after="+*page1.NextCursor), http.StatusOK, &page2)
	if len(page2.Items) == 0 || page2.Items[0]["id"] == page1.Items[1]["id"] {
		t.Fatalf("second page overlaps the first: %v", page2.Items)
	}

	var bad map[string]string
	decodeClose(t, mustGet(t, srv.URL+"/v1/documents?filter[nope]=x"), http.StatusBadRequest, &bad)
	if bad["error"] == "" {
		t.Fatal("400 response lacks an error message")
	}

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/documents/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	dresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := dresp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if dresp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: status %d, want 204", dresp.StatusCode)
	}
	gone := mustGet(t, srv.URL+"/v1/documents/"+id)
	if err := gone.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after DELETE: status %d, want 404", gone.StatusCode)
	}
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url) // #nosec G107 -- the URL of the test server
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeClose(t *testing.T, resp *http.Response, status int, v any) {
	t.Helper()
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	if resp.StatusCode != status {
		var msg bytes.Buffer
		_, _ = msg.ReadFrom(resp.Body)
		t.Fatalf("status %d, want %d: %s", resp.StatusCode, status, msg.String())
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func schemaProperties(t *testing.T, doc []byte, name string) []string {
	t.Helper()
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
	s, ok := d.Components.Schemas[name]
	if !ok {
		t.Fatalf("schema %s missing", name)
	}
	out := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
