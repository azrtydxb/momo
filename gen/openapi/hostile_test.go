// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/schema"
)

func modelWith(name, table string, fields ...*gen.Field) *gen.Model {
	return &gen.Model{GoName: name, Table: table, Fields: fields}
}

// Catches: a table name that adds path segments or template variables, two
// models sharing one collection path, or a model named Error replacing the
// shared error schema.
func TestDocumentHostileTables(t *testing.T) {
	id := &gen.Field{GoName: "ID", Column: "id", BaseType: "int64"}
	g := &gen.Graph{Models: []*gen.Model{
		modelWith("Slash", "a/b", id),
		modelWith("Brace", "{id}", id),
		modelWith("Dots", "..", id),
		modelWith("Pct", "a%2fb", id),
		modelWith("Empty", "", id),
		modelWith("First", "items", id),
		modelWith("Second", "items", id),
		modelWith("Error", "errors", id),
		modelWith("Fine", "fine_things", id),
	}}
	raw, diags := Document(g, Options{Title: "t", Version: "1"})
	var doc struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for p := range doc.Paths {
		if p != "/items" && p != "/fine_things" {
			t.Errorf("unexpected path %q", p)
		}
	}
	if _, ok := doc.Paths["/items"]; !ok {
		t.Errorf("first model for a shared path lost its path: %v", doc.Paths)
	}
	joined := ""
	for _, d := range diags {
		joined += d.Msg + "\n"
	}
	for _, want := range []string{
		`table name "a/b" of Slash cannot be an OpenAPI path segment`,
		`table name "{id}" of Brace cannot be an OpenAPI path segment`,
		`table name ".." of Dots cannot be an OpenAPI path segment`,
		`table name "a%2fb" of Pct cannot be an OpenAPI path segment`,
		`table name "" of Empty cannot be an OpenAPI path segment`,
		"OpenAPI path /items of Second is already used by First",
		"OpenAPI schema name Error is duplicated or reserved",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("no diagnostic containing %q in:\n%s", want, joined)
		}
	}
}

// Catches: unsigned integers advertised as signed 32-bit, which overflow, and
// 64-bit unsigned claiming a format that cannot hold them.
func TestDocumentUnsignedWidths(t *testing.T) {
	g := &gen.Graph{
		Models: []*gen.Model{modelWith("Num", "nums",
			&gen.Field{GoName: "ID", Column: "id", BaseType: "int64"},
			&gen.Field{GoName: "A", Column: "a", BaseType: "uint8"},
			&gen.Field{GoName: "B", Column: "b", BaseType: "uint32"},
			&gen.Field{GoName: "C", Column: "c", BaseType: "uint64"},
		)},
		Schema: &schema.Schema{Tables: []schema.Table{{Name: "nums", PrimaryKey: []string{"id"}}}},
	}
	raw, diags := Document(g, Options{Title: "t", Version: "1"})
	if len(diags) != 0 {
		t.Fatalf("diags = %v", diags)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]map[string]any `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	p := doc.Components.Schemas["Num"].Properties
	if p["a"]["format"] != "int32" || p["a"]["minimum"] != float64(0) {
		t.Errorf("uint8 = %v", p["a"])
	}
	if p["b"]["format"] != "int64" || p["b"]["minimum"] != float64(0) {
		t.Errorf("uint32 = %v", p["b"])
	}
	if _, has := p["c"]["format"]; has || p["c"]["minimum"] != float64(0) {
		t.Errorf("uint64 = %v", p["c"])
	}
}
