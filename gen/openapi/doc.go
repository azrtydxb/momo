// SPDX-License-Identifier: Apache-2.0

// Package openapi generates an OpenAPI 3.1 document from momo models. The
// document describes CRUD and list endpoints with allowlisted filters and
// keyset pagination.
package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/apiquery"
)

// Options configures Document.
type Options struct {
	Title, Version string
	// BasePath prefixes every path, for example /v1. It is empty or starts
	// with a slash.
	BasePath string
	// Handlers asks the plugin to also emit net/http handlers; Document does
	// not read it.
	Handlers bool
}

// obj is a JSON object. encoding/json writes the keys of a map in sorted
// order, so every document is deterministic.
type obj = map[string]any

// schemaFor maps a base Go type to a JSON schema. nullable uses the 3.1 form
// "type": [t, "null"]; the nullable keyword is never written.
func schemaFor(baseType string, nullable bool) (obj, bool) {
	return schemaWith(baseType, nullable, true)
}

// valueSchema is the schema of one non-null value of baseType, for query
// parameters and keys: a filter compares against a value, never against null,
// even for a pointer or sql.Null column.
func valueSchema(baseType string) (obj, bool) {
	return schemaWith(baseType, false, false)
}

// schemaWith is schemaFor; typeNull says whether a pointer or sql.Null type
// makes the schema nullable by itself.
func schemaWith(baseType string, nullable, typeNull bool) (obj, bool) {
	t := strings.TrimPrefix(baseType, "*")
	if typeNull && (strings.HasPrefix(baseType, "*") || strings.HasPrefix(t, "sql.Null")) {
		nullable = true
	}
	typ := func(name string) any {
		if nullable {
			return []string{name, "null"}
		}
		return name
	}
	switch t {
	case "int64", "int", "sql.NullInt64":
		return obj{"type": typ("integer"), "format": "int64"}, true
	case "uint32":
		// Does not fit int32; int64 holds it.
		return obj{"type": typ("integer"), "format": "int64", "minimum": 0}, true
	case "uint64", "uint":
		// No standard format is wide enough, so none is claimed.
		return obj{"type": typ("integer"), "minimum": 0}, true
	case "uint16", "uint8", "sql.NullByte":
		return obj{"type": typ("integer"), "format": "int32", "minimum": 0}, true
	case "int32", "int16", "int8", "sql.NullInt32", "sql.NullInt16":
		return obj{"type": typ("integer"), "format": "int32"}, true
	case "string", "sql.NullString":
		return obj{"type": typ("string")}, true
	case "bool", "sql.NullBool":
		return obj{"type": typ("boolean")}, true
	case "float64", "sql.NullFloat64":
		return obj{"type": typ("number"), "format": "double"}, true
	case "float32":
		return obj{"type": typ("number"), "format": "float"}, true
	case "time.Time", "sql.NullTime":
		return obj{"type": typ("string"), "format": "date-time"}, true
	case "[]byte":
		return obj{"type": typ("string"), "contentEncoding": "base64"}, true
	case "vector.Vector":
		return obj{"type": typ("array"), "items": obj{"type": "number"}}, true
	}
	return nil, false
}

type property struct {
	column   string
	schema   obj
	required bool
	vector   bool
	base     string // base Go type, for the filter and key schemas
}

// schemaNameRE is what OpenAPI allows in a component name.
var schemaNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// pathSegmentRE is what a table name may contain to be one safe URL path
// segment: the unreserved characters of RFC 3986. A name with a slash, brace,
// percent sign, dot-only segment or any other character is refused instead of
// escaped, so a table name can never add path segments or template variables.
var pathSegmentRE = regexp.MustCompile(`^[A-Za-z0-9_~-][A-Za-z0-9._~-]*$`)

func propertiesOf(m *gen.Model) ([]property, []gen.Diagnostic) {
	var props []property
	var diags []gen.Diagnostic
	seen := map[string]bool{}
	for _, f := range m.Fields {
		if f.Tags.Sensitive || seen[f.Column] {
			continue
		}
		seen[f.Column] = true
		s, ok := schemaFor(f.BaseType, f.Nullable)
		if !ok {
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: "no OpenAPI mapping for " + f.BaseType})
			continue
		}
		nullable := f.Nullable || strings.HasPrefix(f.BaseType, "*") || strings.HasPrefix(f.BaseType, "sql.Null")
		props = append(props, property{column: f.Column, schema: s, required: !nullable, vector: strings.HasPrefix(f.BaseType, "vector."), base: f.BaseType})
	}
	return props, diags
}

func ref(name string) obj { return obj{"$ref": "#/components/schemas/" + name} }

func response(desc string, schema obj) obj {
	r := obj{"description": desc}
	if schema != nil {
		r["content"] = obj{"application/json": obj{"schema": schema}}
	}
	return r
}

// Document returns the OpenAPI 3.1 document for the models of g as indented
// JSON, and the diagnostics for columns with no mapping and for options that
// cannot produce a valid document. Sensitive columns are not part of the
// API, and relations are not properties.
func Document(g *gen.Graph, opts Options) ([]byte, []gen.Diagnostic) {
	var diags []gen.Diagnostic
	if opts.Title == "" || opts.Version == "" {
		diags = append(diags, gen.Diagnostic{Msg: "OpenAPI options need a Title and a Version"})
	}
	base := strings.TrimRight(opts.BasePath, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		diags = append(diags, gen.Diagnostic{Msg: fmt.Sprintf("OpenAPI BasePath %q must be empty or start with a slash", opts.BasePath)})
	}

	models := append([]*gen.Model(nil), g.Models...)
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].GoName != models[j].GoName {
			return models[i].GoName < models[j].GoName
		}
		return models[i].PkgPath < models[j].PkgPath
	})

	schemas := obj{"Error": obj{
		"type":       "object",
		"properties": obj{"error": obj{"type": "string"}},
		"required":   []string{"error"},
	}}
	paths := obj{}
	seen := map[string]bool{}
	collections := map[string]string{}
	for _, m := range models {
		if seen[m.GoName] || m.GoName == "Error" {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: "OpenAPI schema name " + m.GoName + " is duplicated or reserved"})
			continue
		}
		if !schemaNameRE.MatchString(m.GoName) {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: fmt.Sprintf("model name %q is not a valid OpenAPI component name", m.GoName)})
			continue
		}
		if !pathSegmentRE.MatchString(m.Table) {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: fmt.Sprintf("table name %q of %s cannot be an OpenAPI path segment", m.Table, m.GoName)})
			continue
		}
		if owner, dup := collections[m.Table]; dup {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: fmt.Sprintf("OpenAPI path /%s of %s is already used by %s", m.Table, m.GoName, owner)})
			continue
		}
		collections[m.Table] = m.GoName
		seen[m.GoName] = true
		props, pd := propertiesOf(m)
		diags = append(diags, pd...)

		properties := obj{}
		var required []string
		for _, p := range props {
			properties[p.column] = p.schema
			if p.required {
				required = append(required, p.column)
			}
		}
		schema := obj{"type": "object", "properties": properties}
		if len(required) > 0 {
			schema["required"] = required
		}
		schemas[m.GoName] = schema

		collection := base + "/" + m.Table
		params := []any{
			obj{"name": "limit", "in": "query", "schema": obj{"type": "integer", "minimum": 1, "maximum": apiquery.MaxLimit, "default": apiquery.DefaultLimit}},
			obj{"name": "after", "in": "query", "schema": obj{"type": "string"}},
		}
		for _, p := range props {
			if p.vector {
				continue
			}
			s, _ := valueSchema(p.base)
			params = append(params, obj{"name": "filter[" + p.column + "]", "in": "query", "schema": s})
		}
		listBody := obj{
			"type": "object",
			"properties": obj{
				"items":       obj{"type": "array", "items": ref(m.GoName)},
				"next_cursor": obj{"type": []string{"string", "null"}},
			},
			"required": []string{"items", "next_cursor"},
		}
		body := obj{"required": true, "content": obj{"application/json": obj{"schema": ref(m.GoName)}}}
		badRequest := response("Invalid request", ref("Error"))
		paths[collection] = obj{
			"get": obj{
				"operationId": "list" + m.GoName,
				"parameters":  params,
				"responses":   obj{"200": response("A page of "+m.GoName+" rows", listBody), "400": badRequest},
			},
			"post": obj{
				"operationId": "create" + m.GoName,
				"requestBody": body,
				"responses":   obj{"201": response("Created", ref(m.GoName)), "400": badRequest},
			},
		}

		if idSchema, ok := singlePK(g, m, props); ok {
			idParam := []any{obj{"name": "id", "in": "path", "required": true, "schema": idSchema}}
			notFound := response("Not found", ref("Error"))
			paths[collection+"/{id}"] = obj{
				"get": obj{
					"operationId": "get" + m.GoName,
					"parameters":  idParam,
					"responses":   obj{"200": response(m.GoName, ref(m.GoName)), "404": notFound},
				},
				"put": obj{
					"operationId": "update" + m.GoName,
					"parameters":  idParam,
					"requestBody": body,
					"responses":   obj{"200": response("Updated", ref(m.GoName)), "400": badRequest, "404": notFound},
				},
				"delete": obj{
					"operationId": "delete" + m.GoName,
					"parameters":  idParam,
					"responses":   obj{"204": response("Deleted", nil), "404": notFound},
				},
			}
		}
	}

	doc := obj{
		"openapi":    "3.1.0",
		"info":       obj{"title": opts.Title, "version": opts.Version},
		"paths":      paths,
		"components": obj{"schemas": schemas},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, append(diags, gen.Diagnostic{Msg: "OpenAPI document cannot be encoded: " + err.Error()})
	}
	return buf.Bytes(), diags
}

// singlePK returns the schema of m's primary key when it is a single exposed
// column; a composite key has no {id} path.
func singlePK(g *gen.Graph, m *gen.Model, props []property) (obj, bool) {
	if g.Schema == nil {
		return nil, false
	}
	t, ok := g.Schema.Table(m.Table)
	if !ok || len(t.PrimaryKey) != 1 {
		return nil, false
	}
	for _, p := range props {
		if p.column == t.PrimaryKey[0] {
			return valueSchema(p.base)
		}
	}
	return nil, false
}
