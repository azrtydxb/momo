// SPDX-License-Identifier: Apache-2.0

package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/gen"
	ggraphql "github.com/azrtydxb/momo/gen/graphql"
	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph/generated"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph/model"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// counter counts the statements momo sends to the database.
type counter struct{ n int }

func (c *counter) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}

func (c *counter) AfterQuery(context.Context, *momo.QueryEvent) {}

func openDB(t *testing.T, c *counter) *momo.DB {
	t.Helper()
	db := testopen.Postgres(t, momo.WithHook(c))
	fixture.Apply(t, db)
	c.n = 0 // count only the statements of the requests
	return db
}

func post(t *testing.T, url, q string) map[string]any {
	t.Helper()
	data, errs := postRaw(t, url, q)
	if len(errs) != 0 {
		t.Fatalf("graphql errors: %v", errs)
	}
	return data
}

// postRaw posts a query and returns its data and the messages of its errors.
func postRaw(t *testing.T, url, q string) (map[string]any, []string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": q})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body)) // #nosec G107 -- the URL of the test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	var out struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, e := range out.Errors {
		msgs = append(msgs, e.Message)
	}
	return out.Data, msgs
}

func connection(t *testing.T, data map[string]any) (ids []string, endCursor string) {
	t.Helper()
	conn, ok := data["documents"].(map[string]any)
	if !ok {
		t.Fatalf("no documents connection in %v", data)
	}
	edges, ok := conn["edges"].([]any)
	if !ok {
		t.Fatalf("no edges in %v", conn)
	}
	for _, e := range edges {
		node := e.(map[string]any)["node"].(map[string]any)
		ids = append(ids, node["id"].(string))
	}
	page := conn["pageInfo"].(map[string]any)
	endCursor, ok = page["endCursor"].(string)
	if !ok {
		t.Fatalf("no endCursor in %v", page)
	}
	return ids, endCursor
}

const pageQuery = `query {
  documents(first: %d%s, filter: {title: {contains: "a"}}, order: {field: ID, direction: ASC}) {
    edges { node { id title comments { id body } tags { id name } } cursor }
    pageInfo { hasNextPage endCursor }
  }
}`

// Catches: stale committed output, a server that loads relations per row (the
// statement count would grow with the page), and a cursor that repeats rows.
func TestGraphQLGenerator(t *testing.T) {
	// 1. the committed output equals a fresh generation from the example models
	models, err := filepath.Abs("../../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	outDir, err := filepath.Abs("example/graph")
	if err != nil {
		t.Fatal(err)
	}
	modRoot, err := filepath.Abs("..")
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
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: modRoot})
	plugin := ggraphql.Plugin{Dir: outDir, Package: "graph", ModelsPackage: "github.com/azrtydxb/momo/internal/testmodels/example"}
	if err := plugin.Generate(g, w); err != nil {
		t.Fatal(err)
	}
	for _, o := range w.Outputs() {
		ok, err := golden.Diff(o.Path, o.Data, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%s is stale; regenerate the example with momo generate and gqlgen generate", o.Path)
		}
	}

	// 2. serve the gqlgen server over HTTP, backed by the database
	c := &counter{}
	db := openDB(t, c)
	exec := generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{DB: db}})
	srv := httptest.NewServer(model.NewHandler(exec))
	t.Cleanup(srv.Close)

	// 3. batched relations: the statement count does not grow with the page size
	c.n = 0
	first2 := post(t, srv.URL, fmt.Sprintf(pageQuery, 2, ""))
	n2 := c.n
	c.n = 0
	first5 := post(t, srv.URL, fmt.Sprintf(pageQuery, 5, ""))
	n5 := c.n
	if n2 != n5 || n2 > 3 {
		t.Fatalf("statements for page of 2 = %d, of 5 = %d; want equal and at most 3 (list plus one batch per relation)", n2, n5)
	}

	// 4. keyset pagination: the page after endCursor has no overlap with the first page
	ids1, cursor := connection(t, first2)
	if len(ids1) != 2 || cursor == "" {
		t.Fatalf("first page = %v cursor=%q", ids1, cursor)
	}
	ids2, lastCursor := connection(t, post(t, srv.URL, fmt.Sprintf(pageQuery, 2, fmt.Sprintf(`, after: %q`, cursor))))
	if lastCursor == "" {
		t.Fatal("second page has no endCursor")
	}
	if len(ids2) != 2 {
		t.Fatalf("second page = %v", ids2)
	}
	seen := map[string]bool{}
	for _, id := range ids1 {
		seen[id] = true
	}
	for _, id := range ids2 {
		if seen[id] {
			t.Fatalf("id %s appears on both pages", id)
		}
	}
	if ids5, last5 := connection(t, first5); len(ids5) != 5 || last5 == "" {
		t.Fatalf("page of 5 = %v cursor=%q", ids5, last5)
	}
	if last := ids1[len(ids1)-1]; ids2[0] <= last && len(last) == len(ids2[0]) {
		t.Fatalf("page 2 starts at %s, not after page 1's last id %s: the cursor was ignored", ids2[0], last)
	}

	// 5. the limits: the in list, the filter depth, the query depth and the complexity
	_, errs := postRaw(t, srv.URL, `query { documents(filter: {title: {in: [`+strings.Repeat(`"a",`, model.MaxInValues)+`"a"]}}) { edges { cursor } } }`)
	if len(errs) != 1 || !strings.Contains(errs[0], "in takes at most") {
		t.Fatalf("an in list over the limit: %v", errs)
	}
	nested := `{title: {eq: "a"}}`
	for i := 0; i <= model.MaxFilterDepth; i++ {
		nested = `{not: ` + nested + `}`
	}
	if _, errs = postRaw(t, srv.URL, `query { documents(filter: `+nested+`) { edges { cursor } } }`); len(errs) != 1 || !strings.Contains(errs[0], "nested too deep") {
		t.Fatalf("a filter nested over the limit: %v", errs)
	}
	shallow := httptest.NewServer(model.NewHandlerWithLimits(exec, model.Limits{MaxDepth: 3, MaxComplexity: 500}))
	t.Cleanup(shallow.Close)
	if _, errs = postRaw(t, shallow.URL, fmt.Sprintf(pageQuery, 2, "")); len(errs) != 1 || !strings.Contains(errs[0], "too deep") {
		t.Fatalf("a query deeper than MaxDepth: %v", errs)
	}
	cheap := httptest.NewServer(model.NewHandlerWithLimits(exec, model.Limits{MaxDepth: 8, MaxComplexity: 5}))
	t.Cleanup(cheap.Close)
	if _, errs = postRaw(t, cheap.URL, fmt.Sprintf(pageQuery, 2, "")); len(errs) != 1 || !strings.Contains(strings.ToLower(errs[0]), "complex") {
		t.Fatalf("a query dearer than MaxComplexity: %v", errs)
	}

	// the rows the selected relations could load are bounded per request: 200
	// documents with two to-many relations of up to 100 rows each is 40000
	_, errs = postRaw(t, srv.URL, `query { documents(first: 200) { edges { node { id comments { id } tags { id } } } } }`)
	if len(errs) != 1 || !strings.Contains(errs[0], "too many rows") {
		t.Fatalf("a selection that could load too many rows: %v", errs)
	}
	// two aliases of one relation load it once, so a small page passes
	_, errs = postRaw(t, srv.URL, `query { documents(first: 5) { edges { node { id a: comments { id } b: comments { body } } } } }`)
	if len(errs) != 0 {
		t.Fatalf("two aliases of one relation on a small page: %v", errs)
	}

	// a request body over MaxRequestBytes is refused
	big, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"query":"query { documents { edges { cursor } } }","pad":"`+strings.Repeat("a", 2<<20)+`"}`)) // #nosec G107 -- the URL of the test server
	if err != nil {
		t.Fatal(err)
	}
	if err := big.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if big.StatusCode < 400 {
		t.Fatalf("a request over the size limit got status %d", big.StatusCode)
	}

	// 6. a database error reaches the client as a fixed message
	_, errs = postRaw(t, srv.URL, `query { documents(filter: {title: {eq: "a"}}, order: {field: ID, direction: SIDEWAYS}) { edges { cursor } } }`)
	if len(errs) == 0 {
		t.Fatal("an unknown order direction was accepted")
	}
	for _, e := range errs {
		if strings.Contains(e, "documents") || strings.Contains(e, "momo:") || strings.Contains(e, "SQL") {
			t.Fatalf("the error leaks internals: %q", e)
		}
	}
}
