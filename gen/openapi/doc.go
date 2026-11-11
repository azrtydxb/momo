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
	"github.com/azrtydxb/momo/gen/internal/gosrc"
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
	// Ops chooses the operations served per model; nil serves every one. The
	// document and the handlers both read it.
	Ops apiquery.Ops
	// OnError names a function in the handlers package with the signature
	// func(ctx context.Context, op string, err error). Every handler calls it
	// once for each error it answers from a statement, with the operation name
	// (the operationId of the document, for example getDocument) and the
	// original error, before it writes the fixed message. Empty means no hook.
	// Document does not read it.
	OnError string
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
	column string
	schema obj
	// required is set when a create and an update must carry the key: a
	// column that is not null and that the database does not fill in.
	required bool
	// notNull is set for a column the output schema lists in required: a
	// response always carries it, whatever the database default or access.
	notNull bool
	// writable is set for a column a request may carry: not the key, not an
	// identity or serial column and not read-only. It is the input schema.
	writable bool
	vector   bool
	base     string // base Go type, for the filter and key schemas
	field    *gen.Field
}

// requiredOnWrite reports whether a write must carry f: it is not null, not
// part of the primary key, not an identity or read-only column, has no
// database default and is not a vector (which the database may leave empty).
func requiredOnWrite(f *gen.Field, pk bool) bool {
	return !isNullable(f) && !pk && !f.Tags.PK && !f.Tags.Identity && !f.Tags.Serial &&
		f.Access != gen.ReadOnly && f.Tags.Default == "" && !strings.HasPrefix(f.BaseType, "vector.")
}

// isNullable reports whether the column can hold NULL.
func isNullable(f *gen.Field) bool {
	return f.Nullable || strings.HasPrefix(f.BaseType, "*") || strings.HasPrefix(f.BaseType, "sql.Null")
}

// writableColumn reports whether a request may carry f: it is not part of the
// primary key and not an identity, serial or read-only column.
func writableColumn(f *gen.Field, pk bool) bool {
	return !pk && !f.Tags.PK && !f.Tags.Identity && !f.Tags.Serial && f.Access != gen.ReadOnly
}

// schemaNameRE is what OpenAPI allows in a component name.
var schemaNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// pathSegmentRE is what a table name may contain to be one safe URL path
// segment: the unreserved characters of RFC 3986. A name with a slash, brace,
// percent sign, dot-only segment or any other character is refused instead of
// escaped, so a table name can never add path segments or template variables.
var pathSegmentRE = regexp.MustCompile(`^[A-Za-z0-9_~-][A-Za-z0-9._~-]*$`)

func propertiesOf(g *gen.Graph, m *gen.Model) ([]property, []gen.Diagnostic) {
	pks := map[string]bool{}
	for _, c := range g.PrimaryKey(m) {
		pks[c] = true
	}
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
		props = append(props, property{column: f.Column, schema: s, required: requiredOnWrite(f, pks[f.Column]), notNull: !isNullable(f), writable: writableColumn(f, pks[f.Column]), vector: strings.HasPrefix(f.BaseType, "vector."), base: f.BaseType, field: f})
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
	if !gosrc.BasePath.MatchString(base) {
		diags = append(diags, gen.Diagnostic{Msg: fmt.Sprintf("OpenAPI BasePath %q must be empty or slash-separated segments of letters, digits and . _ ~ -", opts.BasePath)})
	}
	if err := opts.Ops.Validate(); err != nil {
		diags = append(diags, gen.Diagnostic{Msg: "OpenAPI " + err.Error()})
	}
	if len(diags) > 0 {
		return nil, diags // an invalid option: no document at all
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
	names := map[string]bool{}
	for _, m := range models {
		names[m.GoName] = true
	}
	seen := map[string]bool{}
	collections := map[string]string{}
	for _, m := range models {
		// The input schema exists only for a model with a create or update
		// route, so only then can its name clash with another model's.
		ops, err := opts.Ops.ForModel(m.GoName, m.IsView)
		if err != nil {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: "OpenAPI " + err.Error()})
			continue
		}
		if wops := ops; names[m.GoName+"Input"] && (wops.Has(apiquery.OpCreate) || wops.Has(apiquery.OpUpdate)) {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: "OpenAPI schema name " + m.GoName + "Input is the input schema of " + m.GoName + " and is also a model name"})
			continue
		}
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
		props, pd := propertiesOf(g, m)
		diags = append(diags, pd...)

		// The output schema is what a response carries: every column, with
		// the not-null ones (the key included) required. The input schema,
		// added below for a model with a write route, is what a request may
		// carry.
		properties := obj{}
		var outRequired []string
		inProps := obj{}
		var inRequired []string
		for _, p := range props {
			properties[p.column] = p.schema
			if p.notNull {
				outRequired = append(outRequired, p.column)
			}
			if p.writable {
				inProps[p.column] = p.schema
				if p.required {
					inRequired = append(inRequired, p.column)
				}
			}
		}
		schema := obj{"type": "object", "properties": properties}
		if len(outRequired) > 0 {
			schema["required"] = outRequired
		}
		schemas[m.GoName] = schema

		pk := servedPK(g, m)
		if pk == nil {
			// The model keeps its schema and gets no route: the handlers do not
			// serve it either.
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: "model " + m.GoName + " has no single exposed int64 primary key, so the API serves no routes for it"})
			continue
		}
		if ops.Has(apiquery.OpCreate) || ops.Has(apiquery.OpUpdate) {
			in := obj{"type": "object", "properties": inProps}
			if len(inRequired) > 0 {
				in["required"] = inRequired
			}
			schemas[m.GoName+"Input"] = in
		}
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
		body := obj{"required": true, "content": obj{"application/json": obj{"schema": ref(m.GoName + "Input")}}}
		errResp := func(extra ...string) obj {
			r := obj{
				"400": response("Invalid request", ref("Error")),
				"499": response("The client cancelled the request", ref("Error")),
				"503": response("The database is busy or the transaction conflicted; retry", ref("Error")),
				"504": response("The deadline of the request passed", ref("Error")),
			}
			for _, code := range extra {
				switch code {
				case "404":
					r[code] = response("Not found", ref("Error"))
				case "409":
					r[code] = response("A unique or foreign key constraint refused the change", ref("Error"))
				case "413":
					r[code] = response("The request body is larger than 1 MiB", ref("Error"))
				case "415":
					r[code] = response("The request body is not application/json", ref("Error"))
				}
			}
			return r
		}
		with := func(r obj, code string, v obj) obj { r[code] = v; return r }
		item := obj{}
		coll := obj{}
		if ops.Has(apiquery.OpList) {
			coll["get"] = obj{
				"operationId": "list" + m.GoName,
				"parameters":  params,
				"responses":   with(errResp(), "200", response("A page of "+m.GoName+" rows", listBody)),
			}
		}
		if ops.Has(apiquery.OpCreate) {
			coll["post"] = obj{
				"operationId": "create" + m.GoName,
				"requestBody": body,
				"responses":   with(errResp("409", "413", "415"), "201", response("Created", ref(m.GoName))),
			}
		}
		idParam := []any{obj{"name": "id", "in": "path", "required": true, "schema": obj{"type": "integer", "format": "int64"}}}
		if ops.Has(apiquery.OpGet) {
			item["get"] = obj{
				"operationId": "get" + m.GoName,
				"parameters":  idParam,
				"responses":   with(errResp("404"), "200", response(m.GoName, ref(m.GoName))),
			}
		}
		if ops.Has(apiquery.OpUpdate) {
			item["put"] = obj{
				"operationId": "update" + m.GoName,
				"summary":     "Update a " + m.GoName,
				"description": "The body is the " + m.GoName + "Input schema and must carry every required property. An optional property that is absent keeps its stored value, and only the properties the body carries are written. The response is the " + m.GoName + " schema.",
				"parameters":  idParam,
				"requestBody": body,
				"responses":   with(errResp("404", "409", "413", "415"), "200", response("Updated", ref(m.GoName))),
			}
		}
		if ops.Has(apiquery.OpDelete) {
			item["delete"] = obj{
				"operationId": "delete" + m.GoName,
				"parameters":  idParam,
				"responses":   with(errResp("404", "409"), "204", response("Deleted", nil)),
			}
		}
		if len(coll) > 0 {
			paths[collection] = coll
		}
		if len(item) > 0 {
			paths[collection+"/{id}"] = item
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

// servedPK returns the primary key field of m when the handlers serve the
// model: a single exposed int64 column. The document describes exactly the
// routes the handlers register, so a model without such a key has no paths.
func servedPK(g *gen.Graph, m *gen.Model) *gen.Field {
	pk := gosrc.SinglePK(g, m)
	if pk == nil {
		return nil
	}
	goType := pk.GoType
	if goType == "" { // a field built by hand, without the spelling the loader records
		goType = pk.BaseType
	}
	if goType != "int64" || pk.Tags.Sensitive {
		return nil
	}
	return pk
}
