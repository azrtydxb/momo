// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/gen"
)

var (
	packageRE = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
	identRE   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// protoType maps a field's base Go type to a protobuf type. repeated is set
// for types that map to a repeated field, which cannot be optional.
func protoType(baseType string) (typ string, repeated, timestamp, ok bool) {
	t := strings.TrimPrefix(baseType, "*")
	switch t {
	case "int64", "int", "sql.NullInt64":
		return "int64", false, false, true
	case "int32", "int16", "int8", "sql.NullInt32", "sql.NullInt16":
		return "int32", false, false, true
	case "uint32", "uint16", "uint8", "sql.NullByte":
		return "uint32", false, false, true
	case "uint64", "uint":
		return "uint64", false, false, true
	case "string", "sql.NullString":
		return "string", false, false, true
	case "bool", "sql.NullBool":
		return "bool", false, false, true
	case "float64", "sql.NullFloat64":
		return "double", false, false, true
	case "float32":
		return "float", false, false, true
	case "[]byte":
		return "bytes", false, false, true
	case "time.Time", "sql.NullTime":
		return "google.protobuf.Timestamp", false, true, true
	case "vector.Vector":
		return "float", true, false, true
	}
	return "", false, false, false
}

type protoField struct {
	column   string
	typ      string
	repeated bool
	optional bool
}

// Proto returns the .proto source for the models of g in the proto package
// pkg, and the diagnostics for columns with no protobuf mapping. Field
// numbers come from lock, which Proto updates; the caller saves it. Sensitive
// columns are not part of the API. The output is deterministic.
func Proto(g *gen.Graph, lock *Lock, pkg string) ([]byte, []gen.Diagnostic) {
	var diags []gen.Diagnostic
	if !packageRE.MatchString(pkg) {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid protobuf package %q", pkg)}}
	}
	models := append([]*gen.Model(nil), g.Models...)
	sort.SliceStable(models, func(i, j int) bool { return models[i].GoName < models[j].GoName })

	var body strings.Builder
	seen := map[string]bool{}
	needTimestamp := false
	for _, m := range models {
		if seen[m.GoName] {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: "protobuf message " + m.GoName + " is produced by more than one model"})
			continue
		}
		seen[m.GoName] = true
		if m.GoName == "Filter" {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: "model name Filter collides with the shared Filter message"})
			continue
		}
		fields, ts, fd := fieldsOf(m)
		diags = append(diags, fd...)
		needTimestamp = needTimestamp || ts
		writeModel(&body, g, m, fields, lock, &diags)
	}

	var out strings.Builder
	out.WriteString(gen.Header() + "\n\n")
	out.WriteString("syntax = \"proto3\";\n\npackage " + pkg + ";\n")
	if needTimestamp {
		out.WriteString("\nimport \"google/protobuf/timestamp.proto\";\n")
	}
	out.WriteString("\nmessage Filter {\n  string column = 1;\n  string op = 2;\n  string value = 3;\n}\n")
	out.WriteString(body.String())
	return []byte(out.String()), diags
}

func fieldsOf(m *gen.Model) (fields []protoField, timestamp bool, diags []gen.Diagnostic) {
	seen := map[string]bool{}
	for _, f := range m.Fields {
		if f.Tags.Sensitive || seen[f.Column] {
			continue
		}
		seen[f.Column] = true
		typ, repeated, ts, ok := protoType(f.BaseType)
		if !ok {
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: "no protobuf mapping for " + f.BaseType})
			continue
		}
		if !identRE.MatchString(f.Column) {
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: fmt.Sprintf("column %q is not a valid protobuf field name", f.Column)})
			continue
		}
		timestamp = timestamp || ts
		fields = append(fields, protoField{column: f.Column, typ: typ, repeated: repeated, optional: f.Nullable && !repeated})
	}
	return fields, timestamp, diags
}

// message writes one message whose fields are numbered through the lock.
func message(b *strings.Builder, lock *Lock, name string, fields []protoField, diags *[]gen.Diagnostic) {
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = f.column
	}
	nums, d := lock.Assign(name, cols)
	*diags = append(*diags, d...)
	b.WriteString("\nmessage " + name + " {\n")
	if r := lock.Messages[name].Reserved; len(r) > 0 {
		parts := make([]string, len(r))
		for i, n := range r {
			parts[i] = strconv.Itoa(n)
		}
		b.WriteString("  reserved " + strings.Join(parts, ", ") + ";\n")
	}
	for _, f := range fields {
		n, ok := nums[f.column]
		if !ok {
			continue
		}
		prefix := ""
		switch {
		case f.repeated:
			prefix = "repeated "
		case f.optional:
			prefix = "optional "
		}
		b.WriteString("  " + prefix + f.typ + " " + f.column + " = " + strconv.Itoa(n) + ";\n")
	}
	b.WriteString("}\n")
}

func writeModel(b *strings.Builder, g *gen.Graph, m *gen.Model, fields []protoField, lock *Lock, diags *[]gen.Diagnostic) {
	name := m.GoName
	message(b, lock, name, fields, diags)

	// The key of the row: the primary key columns of the schema table.
	var key []protoField
	if g.Schema != nil {
		if t, ok := g.Schema.Table(m.Table); ok {
			for _, pk := range t.PrimaryKey {
				for _, f := range fields {
					if f.column == pk {
						k := f
						k.optional = false
						key = append(key, k)
					}
				}
			}
		}
	}
	if len(key) == 0 {
		*diags = append(*diags, gen.Diagnostic{Pos: m.Pos, Msg: "model " + name + " has no exposed primary key, so Get and Delete cannot address a row"})
	}
	message(b, lock, name+"GetRequest", key, diags)
	message(b, lock, name+"DeleteRequest", key, diags)
	b.WriteString("\nmessage " + name + "DeleteResponse {}\n")
	b.WriteString("\nmessage " + name + "ListRequest {\n  string after = 1;\n  int32 limit = 2;\n  repeated Filter filter = 3;\n}\n")
	b.WriteString("\nmessage " + name + "ListResponse {\n  repeated " + name + " items = 1;\n  string next_cursor = 2;\n}\n")
	b.WriteString("\nservice " + name + "Service {\n")
	b.WriteString("  rpc Get(" + name + "GetRequest) returns (" + name + ");\n")
	b.WriteString("  rpc List(" + name + "ListRequest) returns (" + name + "ListResponse);\n")
	b.WriteString("  rpc Create(" + name + ") returns (" + name + ");\n")
	b.WriteString("  rpc Update(" + name + ") returns (" + name + ");\n")
	b.WriteString("  rpc Delete(" + name + "DeleteRequest) returns (" + name + "DeleteResponse);\n")
	b.WriteString("}\n")
}
