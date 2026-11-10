// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/apiquery"
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
	// note is the comment written above the field: what a request ignores.
	note string
}

// lockType is the type the lock records for the field: the protobuf type with
// its cardinality, so a change of either gives the field a new number.
func (f protoField) lockType() string {
	switch {
	case f.repeated:
		return "repeated " + f.typ
	case f.optional:
		return "optional " + f.typ
	}
	return f.typ
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
	return protoSource(g, lock, pkg, "", nil)
}

// goPackageRE is what a go_package option may hold: an import path, with an
// optional ;name suffix.
var goPackageRE = regexp.MustCompile(`^[A-Za-z0-9._~/-]+(;[A-Za-z_][A-Za-z0-9_]*)?$`)

// protoSource is Proto with the go_package option set to goPackage when that
// is not empty, which protoc-gen-go needs to place the Go stubs.
func protoSource(g *gen.Graph, lock *Lock, pkg, goPackage string, ops apiquery.Ops) ([]byte, []gen.Diagnostic) {
	var diags []gen.Diagnostic
	if err := ops.Validate(); err != nil {
		return nil, []gen.Diagnostic{{Msg: "gRPC " + err.Error()}}
	}
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

	// live holds every message name this run writes, so a lock entry under a
	// name still in use is never taken over by a renamed model.
	live := map[string]bool{}
	for _, m := range models {
		for _, n := range derivedMessages(m.GoName) {
			live[n] = true
		}
	}
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
		mops, err := ops.ForModel(m.GoName, m.IsView)
		if err != nil {
			fail("gRPC " + err.Error())
			continue
		}
		fields, hidden, ts, fd := fieldsOf(m, lock)
		diags = append(diags, fd...)
		needTimestamp = needTimestamp || ts
		writeModel(&body, g, m, fields, hidden, lock, live, mops, &diags)
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
// Two columns with the same proto3 JSON name (foo_bar and fooBar) are a protoc
// error. protoc compares the names exactly: foo_bar and foobar, or Foo and foo,
// are accepted (checked against protoc 36.2 in internal/integration). The column
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
	// winner maps a JSON name to the candidate that keeps it: the
	// column with the lowest recorded number, else the first one.
	winner := map[string]int{}
	for i, c := range cands {
		key := c.jsonName
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
		key := c.jsonName
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
func message(b *strings.Builder, lock *Lock, name, table string, live map[string]bool, fields []protoField, hidden []string, diags *[]gen.Diagnostic) {
	cols := make([]string, len(fields))
	types := make(map[string]string, len(fields))
	for i, f := range fields {
		cols[i] = f.column
		types[f.column] = f.lockType()
	}
	nums, d := lock.AssignTyped(name, table, live, cols, types, hidden)
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
		if f.note != "" {
			b.WriteString("  // " + f.note + "\n")
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

func writeModel(b *strings.Builder, g *gen.Graph, m *gen.Model, fields []protoField, hidden []string, lock *Lock, live map[string]bool, ops apiquery.OpSet, diags *[]gen.Diagnostic) {
	name := m.GoName
	// The key of the row: the primary key columns of the schema table.
	pkCols := g.PrimaryKey(m)
	message(b, lock, name, m.Table, live, annotate(m, fields, pkCols), hidden, diags)

	var key []protoField
	for _, pk := range pkCols {
		for _, f := range fields {
			if f.column == pk {
				k := f
				k.optional = false
				key = append(key, k)
			}
		}
	}
	if len(key) == 0 {
		*diags = append(*diags, gen.Diagnostic{Pos: m.Pos, Msg: "model " + name + " has no exposed primary key, so Get and Delete cannot address a row"})
	}
	message(b, lock, name+"GetRequest", m.Table+"#get", live, key, nil, diags)
	message(b, lock, name+"DeleteRequest", m.Table+"#delete", live, key, nil, diags)
	b.WriteString("\nmessage " + name + "DeleteResponse {}\n")
	b.WriteString("\nmessage " + name + "ListRequest {\n  string after = 1;\n  int32 limit = 2;\n  repeated Filter filter = 3;\n}\n")
	b.WriteString("\nmessage " + name + "ListResponse {\n  repeated " + name + " items = 1;\n  string next_cursor = 2;\n}\n")
	b.WriteString("\nservice " + name + "Service {\n")
	rpcs := []struct {
		op   apiquery.Op
		line string
	}{
		{apiquery.OpGet, "  rpc Get(" + name + "GetRequest) returns (" + name + ");\n"},
		{apiquery.OpList, "  rpc List(" + name + "ListRequest) returns (" + name + "ListResponse);\n"},
		{apiquery.OpCreate, "  rpc Create(" + name + ") returns (" + name + ");\n"},
		{apiquery.OpUpdate, "  rpc Update(" + name + ") returns (" + name + ");\n"},
		{apiquery.OpDelete, "  rpc Delete(" + name + "DeleteRequest) returns (" + name + "DeleteResponse);\n"},
	}
	for _, r := range rpcs {
		if ops.Has(r.op) {
			b.WriteString(r.line)
		}
	}
	b.WriteString("}\n")
}

// annotate returns fields with the note on each column a request ignores: the
// key (a Create ignores it), the identity, serial and read-only columns (a
// Create and an Update ignore them) and the version, soft-delete and timestamp
// columns momo owns (an Update ignores them). The generated server writes
// exactly these rules; see Update in the server source.
func annotate(m *gen.Model, fields []protoField, pkCols []string) []protoField {
	byCol := map[string]*gen.Field{}
	for _, f := range m.Fields {
		if _, dup := byCol[f.Column]; !dup {
			byCol[f.Column] = f
		}
	}
	isKey := map[string]bool{}
	for _, c := range pkCols {
		isKey[c] = true
	}
	out := make([]protoField, len(fields))
	copy(out, fields)
	for i, pf := range out {
		f := byCol[pf.column]
		switch {
		case f == nil:
		case isKey[pf.column] || f.Tags.PK:
			out[i].note = "The key of the row. Create ignores it (the database assigns it); Update takes it to find the row."
		case f.Tags.Identity || f.Tags.Serial || f.Access == gen.ReadOnly:
			out[i].note = "Read-only: Create and Update ignore it."
		case f.Tags.Version || f.Tags.SoftDelete != "" || f.Tags.CreatedAt || f.Tags.UpdatedAt:
			out[i].note = "Maintained by momo: Update ignores it."
		}
	}
	return out
}
