// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen/apiquery"
)

type schemaDoc struct {
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		} `json:"schemas"`
	} `json:"components"`
}

func exampleDoc(t *testing.T) schemaDoc {
	t.Helper()
	raw, ds := Document(exampleGraph(t), Options{Title: "t", Version: "1", BasePath: "/v1"})
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	var d schemaDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// Catches: one schema for what a client sends and what the server returns, so
// that the key is optional in a response that always has it and appears as a
// property of a create body that ignores it.
func TestInputAndOutputSchemas(t *testing.T) {
	d := exampleDoc(t)
	out, ok := d.Components.Schemas["Document"]
	if !ok {
		t.Fatal("no output schema Document")
	}
	in, ok := d.Components.Schemas["DocumentInput"]
	if !ok {
		t.Fatal("no input schema DocumentInput")
	}
	for _, col := range []string{"id", "tenant_id", "title", "body", "status", "created_at"} {
		if _, ok := out.Properties[col]; !ok {
			t.Errorf("output schema lacks the property %s", col)
		}
		if !slices.Contains(out.Required, col) {
			t.Errorf("output schema does not require the not-null column %s: %v", col, out.Required)
		}
	}
	for _, col := range []string{"id", "created_at"} {
		if _, ok := in.Properties[col]; ok {
			t.Errorf("input schema has the key or read-only property %s", col)
		}
		if slices.Contains(in.Required, col) {
			t.Errorf("input schema requires %s", col)
		}
	}
	for _, col := range []string{"tenant_id", "title", "body", "status"} {
		if _, ok := in.Properties[col]; !ok {
			t.Errorf("input schema lacks the writable property %s", col)
		}
	}
	if want := []string{"tenant_id", "title", "body"}; !slices.Equal(in.Required, want) {
		t.Errorf("input required = %v, want %v", in.Required, want)
	}
}

// Catches: a create or PUT that still takes the output schema, or a response
// that takes the input schema.
func TestOperationsReferenceTheRightSchema(t *testing.T) {
	d := exampleDoc(t)
	type op struct {
		RequestBody struct {
			Content map[string]struct {
				Schema struct {
					Ref string `json:"$ref"`
				} `json:"schema"`
			} `json:"content"`
		} `json:"requestBody"`
		Responses map[string]struct {
			Content map[string]struct {
				Schema struct {
					Ref   string `json:"$ref"`
					Props struct {
						Items struct {
							Items struct {
								Ref string `json:"$ref"`
							} `json:"items"`
						} `json:"items"`
					} `json:"properties"`
				} `json:"schema"`
			} `json:"content"`
		} `json:"responses"`
	}
	get := func(path, method string) op {
		t.Helper()
		raw, ok := d.Paths[path][method]
		if !ok {
			t.Fatalf("no %s %s", method, path)
		}
		var o op
		if err := json.Unmarshal(raw, &o); err != nil {
			t.Fatal(err)
		}
		return o
	}
	const in, out = "#/components/schemas/DocumentInput", "#/components/schemas/Document"
	for _, c := range []struct{ path, method, status string }{
		{"/v1/documents", "post", "201"}, {"/v1/documents/{id}", "put", "200"}, {"/v1/documents/{id}", "get", "200"},
	} {
		o := get(c.path, c.method)
		if got := o.Responses[c.status].Content["application/json"].Schema.Ref; got != out {
			t.Errorf("%s %s answers %q, want %q", c.method, c.path, got, out)
		}
		if c.method != "get" {
			if got := o.RequestBody.Content["application/json"].Schema.Ref; got != in {
				t.Errorf("%s %s takes %q, want %q", c.method, c.path, got, in)
			}
		}
	}
	list := get("/v1/documents", "get")
	if got := list.Responses["200"].Content["application/json"].Schema.Props.Items.Items.Ref; got != out {
		t.Errorf("list items are %q, want %q", got, out)
	}
	for method, path := range map[string]string{"get": "/v1/documents", "post": "/v1/documents", "put": "/v1/documents/{id}"} {
		o := get(path, method)
		for _, code := range []string{"499", "504"} {
			if _, ok := o.Responses[code]; !ok {
				t.Errorf("%s %s does not document the %s response", method, path, code)
			}
		}
	}
}

// Catches: a model with no write route still getting an input schema nobody
// references.
func TestInputSchemaOnlyForWritableModels(t *testing.T) {
	g := exampleGraph(t)
	raw, ds := Document(g, Options{Title: "t", Version: "1", Ops: apiquery.Ops{apiquery.DefaultModel: {"list", "get"}}})
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	var d schemaDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Components.Schemas["DocumentInput"]; ok {
		t.Error("a read-only API still has the input schema DocumentInput")
	}
	if _, ok := d.Components.Schemas["Document"]; !ok {
		t.Error("a read-only API lost the output schema")
	}
}

// Catches: a cancelled request answered as a server error.
func TestCancellationStatuses(t *testing.T) {
	src, ds := Handlers(exampleGraph(t), "handlers", Options{Title: "t", Version: "1"})
	if src == nil {
		t.Fatal(ds)
	}
	for _, want := range []string{
		"statusClientClosedRequest = 499",
		"errors.Is(err, context.DeadlineExceeded)",
		"http.StatusGatewayTimeout",
		"errors.Is(err, context.Canceled)",
		`"request canceled"`,
		`"request timed out"`,
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("handlers lack %q", want)
		}
	}
}
