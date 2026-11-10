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

// derivedMessages lists the message and service names a model name derives.
func derivedMessages(name string) []string {
	return []string{name, name + "GetRequest", name + "DeleteRequest", name + "DeleteResponse", name + "ListRequest", name + "ListResponse", name + "Service"}
}

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
// pkg, and the diagnostics for columns and models that cannot be exposed.
// Field numbers come from lock, which Proto updates; the caller writes it with
// Lock.Marshal. Sensitive columns are not part of the API. A model or derived
// message name that is not a valid ASCII protobuf identifier or that collides
// with another message (Filter, or XGetRequest against a model called
// XGetRequest) is a diagnostic and the model is left out. The output is
// deterministic.
func Proto(g *gen.Graph, lock *Lock, pkg string) ([]byte, []gen.Diagnostic) {
	return protoSource(g, lock, pkg, "")
}

// goPackageRE is what a go_package option may hold: an import path, with an
// optional ;name suffix.
var goPackageRE = regexp.MustCompile(`^[A-Za-z0-9._~/-]+(;[A-Za-z_][A-Za-z0-9_]*)?$`)

// protoSource is Proto with the go_package option set to goPackage when that
// is not empty, which protoc-gen-go needs to place the Go stubs.
func protoSource(g *gen.Graph, lock *Lock, pkg, goPackage string) ([]byte, []gen.Diagnostic) {
	var diags []gen.Diagnostic
	if goPackage != "" && !goPackageRE.MatchString(goPackage) {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid Go package %q for the protobuf go_package option", goPackage)}}
	}
	if !packageRE.MatchString(pkg) {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid protobuf package %q", pkg)}}
	}
	models := append([]*gen.Model(nil), g.Models...)
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].GoName != models[j].GoName {
			return models[i].GoName < models[j].GoName
		}
		return models[i].PkgPath < models[j].PkgPath
	})

	var body strings.Builder
	taken := map[string]string{"Filter": ""}
	needTimestamp := false
	for _, m := range models {
		fail := func(msg string) { diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: msg}) }
		if !identRE.MatchString(m.GoName) {
			fail("model name " + m.GoName + " is not a valid protobuf identifier")
			continue
		}
		clash := ""
		for _, n := range derivedMessages(m.GoName) {
			if owner, dup := taken[n]; dup {
				if owner == "" {
					clash = n + " is the shared Filter message"
				} else {
					clash = n + " is also derived from model " + owner
				}
			}
		}
		if clash != "" {
			fail("protobuf messages of model " + m.GoName + " collide: " + clash)
			continue
		}
		for _, n := range derivedMessages(m.GoName) {
			taken[n] = m.GoName
		}
		fields, hidden, ts, fd := fieldsOf(m, lock)
		diags = append(diags, fd...)
		needTimestamp = needTimestamp || ts
		writeModel(&body, g, m, fields, hidden, lock, &diags)
	}

	var out strings.Builder
	out.WriteString(gen.Header() + "\n\n")
	out.WriteString("syntax = \"proto3\";\n\npackage " + pkg + ";\n")
	if goPackage != "" {
		out.WriteString("\noption go_package = " + strconv.Quote(goPackage) + ";\n")
	}
	if needTimestamp {
		out.WriteString("\nimport \"google/protobuf/timestamp.proto\";\n")
	}
	out.WriteString("\nmessage Filter {\n  string column = 1;\n  string op = 2;\n  string value = 3;\n}\n")
	out.WriteString(body.String())
	return []byte(out.String()), diags
}

// fieldsOf returns the exposed fields of m. hidden lists the columns of m
// that are not exposed (sensitive, unmapped or badly named), so the lock does
// not retire their numbers.
//
// Two columns whose proto3 JSON names are equal when compared without regard
// to case (foo_bar and foobar, Foo and foo) are a protoc error. The column
// the lock already numbers keeps its field and the other is left out, so adding
// a column never drops an existing field from the wire; without a lock entry
// the first column wins.
func fieldsOf(m *gen.Model, lock *Lock) (fields []protoField, hidden []string, timestamp bool, diags []gen.Diagnostic) {
	type cand struct {
		f        *gen.Field
		pf       protoField
		ts       bool
		jsonName string
	}
	var cands []cand
	seen := map[string]bool{}
	for _, f := range m.Fields {
		if seen[f.Column] {
			continue
		}
		seen[f.Column] = true
		if f.Tags.Sensitive {
			hidden = append(hidden, f.Column)
			continue
		}
		typ, repeated, ts, ok := protoType(f.BaseType)
		if !ok {
			hidden = append(hidden, f.Column)
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: "no protobuf mapping for " + f.BaseType})
			continue
		}
		if !identRE.MatchString(f.Column) {
			hidden = append(hidden, f.Column)
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: fmt.Sprintf("column %q is not a valid protobuf field name", f.Column)})
			continue
		}
		cands = append(cands, cand{
			f: f, ts: ts, jsonName: jsonName(f.Column),
			pf: protoField{column: f.Column, typ: typ, repeated: repeated, optional: f.Nullable && !repeated},
		})
	}

	var locked map[string]int
	if lock != nil && lock.Messages[m.GoName] != nil {
		locked = lock.Messages[m.GoName].Fields
	}
	// winner maps a case-folded JSON name to the candidate that keeps it: the
	// column with the lowest recorded number, else the first one.
	winner := map[string]int{}
	for i, c := range cands {
		key := strings.ToLower(c.jsonName)
		w, taken := winner[key]
		if !taken {
			winner[key] = i
			continue
		}
		n, isLocked := locked[c.f.Column]
		wn, winnerLocked := locked[cands[w].f.Column]
		if isLocked && (!winnerLocked || n < wn) {
			winner[key] = i
		}
	}
	for i, c := range cands {
		key := strings.ToLower(c.jsonName)
		if w := winner[key]; w != i {
			hidden = append(hidden, c.f.Column)
			diags = append(diags, gen.Diagnostic{Pos: c.f.Pos, Msg: fmt.Sprintf("column %q of %s has the same proto3 JSON name %q as column %q", c.f.Column, m.GoName, c.jsonName, cands[w].f.Column)})
			continue
		}
		timestamp = timestamp || c.ts
		fields = append(fields, c.pf)
	}
	return fields, hidden, timestamp, diags
}

// jsonName is the proto3 JSON name protoc derives from a field name: each
// underscore is dropped and the letter after it is upper-cased. Two fields of
// one message with the same JSON name (foo_bar and fooBar) are a protoc error.
func jsonName(field string) string {
	var b strings.Builder
	upper := false
	for _, r := range field {
		switch {
		case r == '_':
			upper = true
		case upper:
			b.WriteString(strings.ToUpper(string(r)))
			upper = false
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// message writes one message whose fields are numbered through the lock.
func message(b *strings.Builder, lock *Lock, name string, fields []protoField, hidden []string, diags *[]gen.Diagnostic) {
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = f.column
	}
	nums, d := lock.AssignKeeping(name, cols, hidden)
	*diags = append(*diags, d...)
	b.WriteString("\nmessage " + name + " {\n")
	ml := lock.Messages[name]
	if len(ml.Reserved) > 0 {
		parts := make([]string, len(ml.Reserved))
		for i, n := range ml.Reserved {
			parts[i] = strconv.Itoa(n)
		}
		b.WriteString("  reserved " + strings.Join(parts, ", ") + ";\n")
	}
	if len(ml.ReservedNames) > 0 {
		parts := make([]string, len(ml.ReservedNames))
		for i, n := range ml.ReservedNames {
			parts[i] = strconv.Quote(n)
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

func writeModel(b *strings.Builder, g *gen.Graph, m *gen.Model, fields []protoField, hidden []string, lock *Lock, diags *[]gen.Diagnostic) {
	name := m.GoName
	message(b, lock, name, fields, hidden, diags)

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
	message(b, lock, name+"GetRequest", key, nil, diags)
	message(b, lock, name+"DeleteRequest", key, nil, diags)
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
