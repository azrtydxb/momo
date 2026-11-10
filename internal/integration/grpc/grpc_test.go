// SPDX-License-Identifier: Apache-2.0

package grpc_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/azrtydxb/momo/gen"
	ggrpc "github.com/azrtydxb/momo/gen/grpc"
	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/grpc/example"
	"github.com/azrtydxb/momo/internal/integration/grpc/example/pb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// protocPath returns the path of protoc v36.2 from PROTOC. A missing value
// fails the test when integration tests are required and skips it otherwise,
// so the unit job, which has no protoc, stays green.
func protocPath(t *testing.T) string {
	t.Helper()
	p := os.Getenv("PROTOC")
	if p == "" {
		if os.Getenv("MOMO_REQUIRE_INTEGRATION") == "1" {
			t.Fatal("PROTOC is required: install protoc v36.2 and set PROTOC to its path (see internal/integration/tools.lock)")
		}
		t.Skip("PROTOC is not set; run make protoc and set PROTOC=$(pwd)/internal/integration/bin/protoc")
	}
	return p
}

// goTool builds the Go plugin pkg of the integration module into dir and
// returns the path of the binary.
func goTool(t *testing.T, modRoot, dir, pkg string) string {
	t.Helper()
	out := filepath.Join(dir, filepath.Base(pkg))
	cmd := exec.Command("go", "build", "-o", out, pkg) // #nosec G204 -- the package is a constant of this test
	cmd.Dir = modRoot
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
	return out
}

// Catches: a stale .proto, lock, server adapter or generated stub; a .proto
// protoc rejects; a server that fails a create, read, list, update or delete
// over gRPC against PostgreSQL; and a rename of a Go field that moves its
// field number.
func TestProtoGenerator(t *testing.T) {
	pkgDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	modRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(pkgDir, "example")
	modelsDir, err := filepath.Abs("../../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}

	// (a) the plugin output equals the committed .proto, lock and server
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: modelsDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: modRoot})
	plugin := ggrpc.Plugin{
		Dir:          outDir,
		Package:      "example",
		ProtoPackage: "momo.example.v1",
		GoPackage:    "github.com/azrtydxb/momo/internal/integration/grpc/example/pb",
	}
	if err := plugin.Generate(g, w); err != nil {
		t.Fatal(err)
	}
	for _, o := range w.Outputs() {
		ok, err := golden.Diff(o.Path, o.Data, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%s is stale; regenerate with momo generate", o.Path)
		}
	}

	// (b) protoc output equals the committed stubs
	protoc := protocPath(t)
	tools := t.TempDir()
	genGo := goTool(t, modRoot, tools, "google.golang.org/protobuf/cmd/protoc-gen-go")
	genGRPC := goTool(t, modRoot, tools, "google.golang.org/grpc/cmd/protoc-gen-go-grpc")
	tmp := t.TempDir()
	args := []string{
		"--plugin=protoc-gen-go=" + genGo, "--plugin=protoc-gen-go-grpc=" + genGRPC,
		"--proto_path=" + outDir,
		"--go_out=" + tmp, "--go_opt=paths=source_relative",
		"--go-grpc_out=" + tmp, "--go-grpc_opt=paths=source_relative",
		"momo.proto",
	}
	if out, err := exec.Command(protoc, args...).CombinedOutput(); err != nil { // #nosec G204 -- protoc is the tool this test exists to run
		t.Fatalf("protoc: %v\n%s", err, out)
	}
	for _, name := range []string{"momo.pb.go", "momo_grpc.pb.go"} {
		got, err := os.ReadFile(filepath.Join(tmp, name)) // #nosec G304 -- a file protoc just wrote under t.TempDir
		if err != nil {
			t.Fatal(err)
		}
		ok, err := golden.Diff(filepath.Join(outDir, "pb", name), got, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("pb/%s is stale; rerun protoc with the committed options", name)
		}
	}
	checkJSONNameRule(t, protoc)

	// (c) the stubs and the server compile
	build := exec.Command("go", "build", "./grpc/...")
	build.Dir = modRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// (d) CRUD round trip over bufconn, backed by PostgreSQL
	db := testopen.Postgres(t)
	fixture.Apply(t, db)
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterDocumentServiceServer(srv, example.NewDocumentServer(db))
	served := make(chan error, 1)
	go func() { served <- srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		if err := <-served; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	client := pb.NewDocumentServiceClient(conn)
	ctx := context.Background()

	created, err := client.Create(ctx, &pb.Document{TenantId: 1, Title: "round trip", Status: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Id == 0 {
		t.Fatal("create returned no id")
	}
	got, err := client.Get(ctx, &pb.DocumentGetRequest{Id: created.Id})
	if err != nil || got.Title != "round trip" {
		t.Fatalf("get = %v, %v", got, err)
	}
	created.Title = "round trip v2"
	if _, err := client.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	list, err := client.List(ctx, &pb.DocumentListRequest{Limit: 10, Filter: []*pb.Filter{{Column: "title", Op: "eq", Value: "round trip v2"}}})
	if err != nil || len(list.Items) != 1 || list.Items[0].Id != created.Id {
		t.Fatalf("filtered list = %v, %v", list, err)
	}
	if _, err := client.List(ctx, &pb.DocumentListRequest{Filter: []*pb.Filter{{Column: "nope", Op: "eq", Value: "x"}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("filter on an unknown column: want InvalidArgument, got %v", err)
	}
	first, err := client.List(ctx, &pb.DocumentListRequest{Limit: 2})
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %v, %v", first, err)
	}
	second, err := client.List(ctx, &pb.DocumentListRequest{Limit: 2, After: first.NextCursor})
	if err != nil || len(second.Items) == 0 || second.Items[0].Id == first.Items[1].Id {
		t.Fatalf("keyset page = %v, %v", second, err)
	}
	if _, err := client.Delete(ctx, &pb.DocumentDeleteRequest{Id: created.Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(ctx, &pb.DocumentGetRequest{Id: created.Id}); status.Code(err) != codes.NotFound {
		t.Fatalf("get after delete: want NotFound, got %v", err)
	}

	// (e) renaming a Go field keeps the field number recorded for its column
	renameDir, err := filepath.Abs("testdata/rename")
	if err != nil {
		t.Fatal(err)
	}
	rg, rdiags, err := gen.Load([]string{"./"}, gen.Config{Dir: renameDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(rdiags) != 0 {
		t.Fatalf("rename diagnostics: %v", rdiags)
	}
	lock, err := ggrpc.LoadLock(filepath.Join(outDir, "momo.proto.lock"))
	if err != nil {
		t.Fatal(err)
	}
	before := lock.Messages["Document"].Fields["title"]
	src, ds := ggrpc.Proto(rg, lock, "momo.example.v1")
	if len(ds) != 0 {
		t.Fatalf("diagnostics: %v", ds)
	}
	if after := lock.Messages["Document"].Fields["title"]; after != before || before != 3 {
		t.Fatalf("title number changed: before %d, after %d", before, after)
	}
	if !strings.Contains(string(src), "string title = 3;") {
		t.Fatalf("renamed field does not keep number 3:\n%s", src)
	}
	if _, invented := lock.Messages["Document"].Fields["name"]; invented {
		t.Fatal("rename created a new field identity")
	}
}

// checkJSONNameRule has protoc rule on the JSON-name collision the emitter
// guards against: fields whose JSON names differ only in case are an error,
// while a message with distinct names compiles.
func checkJSONNameRule(t *testing.T, protoc string) {
	t.Helper()
	compile := func(fields string) ([]byte, error) {
		dir := t.TempDir()
		src := "syntax = \"proto3\";\npackage jsoncheck;\nmessage M {\n" + fields + "}\n"
		if err := os.WriteFile(filepath.Join(dir, "m.proto"), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(protoc, "--proto_path="+dir, "--descriptor_set_out="+filepath.Join(dir, "m.pb"), "m.proto") // #nosec G204 -- protoc is the tool under test
		return cmd.CombinedOutput()
	}
	if out, err := compile("  string foo_bar = 1;\n  string baz = 2;\n"); err != nil {
		t.Fatalf("a message with distinct JSON names is rejected: %v\n%s", err, out)
	}
	for name, fields := range map[string]string{
		"foo_bar and foobar": "  string foo_bar = 1;\n  string foobar = 2;\n",
		"foo_bar and fooBar": "  string foo_bar = 1;\n  string fooBar = 2;\n",
		"Foo and foo":        "  string Foo = 1;\n  string foo = 2;\n",
	} {
		if out, err := compile(fields); err == nil {
			t.Errorf("protoc accepts %s in one message, so the emitter's case-insensitive rule is stricter than protoc needs:\n%s", name, out)
		}
	}
}
