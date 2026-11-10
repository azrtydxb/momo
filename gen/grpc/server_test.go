// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/apiquery"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/schema"
)

const testPB = "example.com/app/pb"

func exampleGraph(t *testing.T) *gen.Graph {
	t.Helper()
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	return g
}

// Catches: GoCamelCase drifting from protoc-gen-go, which would make the
// adapters reference message fields that do not exist.
func TestGoCamel(t *testing.T) {
	for in, want := range map[string]string{
		"id": "Id", "tenant_id": "TenantId", "created_at": "CreatedAt", "next_cursor": "NextCursor",
		"foo2bar": "Foo2Bar", "_x": "XX", "a_1": "A_1",
	} {
		if got := goCamel(in); got != want {
			t.Errorf("goCamel(%q) = %q, want %q", in, got, want)
		}
	}
}

// Catches: server code that does not parse, skips the allowlist or the typed
// builders, or maps a column the .proto does not carry.
func TestServerSourceExample(t *testing.T) {
	g := exampleGraph(t)
	lock := &Lock{}
	if _, ds := Proto(g, lock, "momo.example.v1"); len(ds) != 0 {
		t.Fatalf("proto diagnostics: %v", ds)
	}
	src, diags := ServerSource(g, lock, "example", testPB)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "server_gen.go", src, 0); err != nil {
		t.Fatalf("server does not parse: %v\n%s", err, src)
	}
	s := string(src)
	for _, must := range []string{
		"package example",
		"func NewDocumentServer(db *momo.DB) pb.DocumentServiceServer",
		"pb.UnimplementedDocumentServiceServer",
		"apiquery.ParseFilter(documentdesc.ColumnByName",
		"apiquery.ClampLimit",
		"apiquery.DecodeCursor",
		"momo.Select[example.Document](s.db)",
		"momo.Insert[example.Document](s.db)",
		"momo.Update[example.Document](s.db)",
		"momo.Delete[example.Document](s.db)",
		"TenantId: m.TenantID,",
		"CreatedAt: timestamppb.New(m.CreatedAt),",
		"Embedding: []float32(m.Embedding),",
		"ins = ins.Omit(documentdesc.Embedding)",
		"codes.NotFound",
		"codes.AlreadyExists",
		"codes.FailedPrecondition",
		"codes.Unavailable",
		"func isZero[T comparable](v T) bool",
		"if !isZero(p.Status) {",
		"if isZero(m.Status) {\n\t\tins = ins.Omit(documentdesc.Status)",
		"authentication or authorization: wrap the grpc.Server",
		`"google.golang.org/grpc/status"`,
		`"example.com/app/pb"`,
	} {
		if !strings.Contains(s, must) {
			t.Errorf("server lacks %q", must)
		}
	}
	if strings.Contains(s, ".With(") {
		t.Error("the server must use the typed builders only")
	}
	if strings.Contains(s, "p.CreatedAt") && strings.Contains(s, "m.CreatedAt = p.CreatedAt") {
		t.Error("a read-only column is written from the request")
	}
	if strings.Contains(s, "m.ID = p.Id") {
		t.Error("the primary key is written from the message")
	}
	again, _ := ServerSource(g, lock, "example", testPB)
	if string(again) != s {
		t.Error("ServerSource is not deterministic")
	}
}

func serverModel(fields ...*gen.Field) *gen.Graph {
	m := &gen.Model{GoName: "Thing", Table: "things", PkgPath: "example.com/m", PkgName: "m", SubPkg: "thing", Fields: fields}
	return &gen.Graph{Models: []*gen.Model{m}, Schema: &schema.Schema{Tables: []schema.Table{{Name: "things", PrimaryKey: []string{"id"}}}}}
}

func sf(goName, col, goType string) *gen.Field {
	return &gen.Field{GoName: goName, Column: col, GoType: goType, BaseType: strings.TrimPrefix(goType, "*"), Path: []int{0}, Nullable: strings.HasPrefix(goType, "*")}
}

// Catches: a sensitive column, a pointer or a converted integer mapped wrongly,
// or an unsupported type slipping through.
func TestServerSourceFieldRules(t *testing.T) {
	secret := sf("Secret", "secret", "string")
	secret.Tags = tags.Tags{Sensitive: true}
	g := serverModel(sf("ID", "id", "int64"), secret, sf("Note", "note", "*string"), sf("Count", "count", "int"), sf("At", "at", "*time.Time"))
	lock := &Lock{}
	if _, ds := Proto(g, lock, "acme.v1"); len(ds) != 0 {
		t.Fatalf("proto diagnostics: %v", ds)
	}
	src, diags := ServerSource(g, lock, "api", testPB)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	s := string(src)
	if strings.Contains(s, "Secret") || strings.Contains(s, "secret") {
		t.Errorf("sensitive column in the server:\n%s", s)
	}
	for _, must := range []string{
		"Note: m.Note,", "Count: int64(m.Count),", "m.Count = int(p.Count)",
		"out.At = timestamppb.New(*m.At)", "m.At = &t",
	} {
		if !strings.Contains(s, must) {
			t.Errorf("server lacks %q:\n%s", must, s)
		}
	}

	bad := serverModel(sf("ID", "id", "int64"), sf("Odd", "odd", "sql.NullInt64"))
	lock = &Lock{}
	Proto(bad, lock, "acme.v1")
	if _, diags = ServerSource(bad, lock, "api", testPB); len(diags) != 1 || !strings.Contains(diags[0].Msg, "does not support the type sql.NullInt64") {
		t.Errorf("unsupported type diags = %v", diags)
	}
	if _, diags = ServerSource(g, lock, "Bad Pkg", testPB); len(diags) != 1 {
		t.Errorf("bad package accepted: %v", diags)
	}
	if _, diags = ServerSource(g, lock, "api", "not a path"); len(diags) != 1 {
		t.Errorf("bad import path accepted: %v", diags)
	}
}

// Catches: the plugin dropping the go_package option protoc-gen-go needs,
// writing a lock LoadLock cannot read back, or renumbering on a second run.
func TestPluginGenerate(t *testing.T) {
	g := exampleGraph(t)
	root := t.TempDir()
	p := Plugin{Dir: filepath.Join(root, "example"), ProtoPackage: "momo.example.v1", GoPackage: testPB}
	if p.Name() != "grpc" || p.APIVersion() != gen.PluginAPIVersion {
		t.Fatalf("name %q version %d", p.Name(), p.APIVersion())
	}
	run := func() *gen.Writer {
		w := gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
		if err := p.Generate(g, w); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Commit(); err != nil {
			t.Fatal(err)
		}
		return w
	}
	first := run()
	outs := first.Outputs()
	if len(outs) != 3 {
		t.Fatalf("outputs = %d, want the proto, its lock and the server", len(outs))
	}
	for _, o := range outs {
		if filepath.Base(o.Path) == "momo.proto" && !strings.Contains(string(o.Data), `option go_package = "example.com/app/pb";`) {
			t.Errorf("no go_package option:\n%s", o.Data)
		}
	}
	lock, err := LoadLock(filepath.Join(root, "example", "momo.proto.lock"))
	if err != nil {
		t.Fatalf("the written lock does not load: %v", err)
	}
	if lock.Messages["Document"].Fields["title"] != 3 {
		t.Errorf("title number = %d, want 3", lock.Messages["Document"].Fields["title"])
	}
	second := run()
	for i, o := range second.Outputs() {
		if string(o.Data) != string(outs[i].Data) {
			t.Errorf("%s changed on the second run", o.Path)
		}
	}
}

// Catches: an operation that is switched off still being served, or a read-only
// service whose proto still offers writes.
func TestGRPCOps(t *testing.T) {
	g := exampleGraph(t)
	ops := apiquery.Ops{apiquery.DefaultModel: {"list", "get"}, "Tag": {"list", "create"}}
	lock := &Lock{}
	proto, diags := protoSource(g, lock, "momo.example.v1", "", ops)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	p := string(proto)
	for _, no := range []string{"rpc Create(Document)", "rpc Update(Document)", "rpc Delete(DocumentDeleteRequest)", "rpc Get(TagGetRequest)", "rpc Delete(TagDeleteRequest)"} {
		if strings.Contains(p, no) {
			t.Errorf("proto offers %q although the operation is off", no)
		}
	}
	for _, yes := range []string{"rpc Get(DocumentGetRequest)", "rpc List(DocumentListRequest)", "rpc Create(Tag)", "rpc List(TagListRequest)"} {
		if !strings.Contains(p, yes) {
			t.Errorf("proto lacks %q", yes)
		}
	}
	src, diags := serverSource(g, lock, "example", testPB, ops)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s := string(src)
	for _, no := range []string{"func (s *documentServer) Create", "func (s *documentServer) Update", "func (s *documentServer) Delete", "func (s *tagServer) Get(", "func (s *tagServer) Update"} {
		if strings.Contains(s, no) {
			t.Errorf("server implements %q although the operation is off", no)
		}
	}
	for _, yes := range []string{"func (s *documentServer) Get(", "func (s *documentServer) List(", "func (s *tagServer) Create("} {
		if !strings.Contains(s, yes) {
			t.Errorf("server lacks %q", yes)
		}
	}
	if _, diags := protoSource(g, &Lock{}, "momo.example.v1", "", apiquery.Ops{"*": {"drop"}}); len(diags) != 1 {
		t.Errorf("unknown operation accepted: %v", diags)
	}
}
