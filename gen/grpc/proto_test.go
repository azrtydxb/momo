// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/schema"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Catches: any change to the protobuf generated for the example models,
// which the golden file records after review.
func TestProtoEmitGolden(t *testing.T) {
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	lock := &Lock{}
	src, ds := Proto(g, lock, "momo.example.v1")
	if len(ds) != 0 {
		t.Fatalf("diags %v", ds)
	}
	ok, err := golden.Diff("testdata/example.proto", src, *update)
	if err != nil || !ok {
		t.Fatalf("golden mismatch (err=%v); got:\n%s\nrun with -update after review", err, src)
	}
	// A second run over the filled lock yields the same bytes.
	again, _ := Proto(g, lock, "momo.example.v1")
	if string(again) != string(src) {
		t.Fatal("regenerating with the recorded lock changed the output")
	}
}

func syntheticGraph() *gen.Graph {
	m := &gen.Model{
		GoName: "Thing", Table: "things",
		Fields: []*gen.Field{
			{GoName: "ID", Column: "id", BaseType: "int64"},
			{GoName: "Secret", Column: "secret", BaseType: "string", Tags: tags.Tags{Sensitive: true}},
			{GoName: "Emb", Column: "emb", BaseType: "vector.Vector", Nullable: true},
			{GoName: "Note", Column: "note", BaseType: "string", Nullable: true},
			{GoName: "Data", Column: "data", BaseType: "[]byte"},
			{GoName: "Odd", Column: "odd", BaseType: "map[string]string"},
		},
	}
	return &gen.Graph{
		Models: []*gen.Model{m},
		Schema: &schema.Schema{Tables: []schema.Table{{Name: "things", PrimaryKey: []string{"id"}}}},
	}
}

// Catches: sensitive columns in the API, a wrong optional or repeated
// prefix, an unmapped type passing silently, a retired number not written as
// reserved, or a bad package name accepted.
func TestProtoRules(t *testing.T) {
	lock := &Lock{}
	lock.Assign("Thing", []string{"id", "gone", "note"}) // "gone" was field 2
	src, diags := Proto(syntheticGraph(), lock, "acme.v1")
	s := string(src)
	for _, want := range []string{
		"syntax = \"proto3\";\n\npackage acme.v1;\n",
		"  reserved 2;\n",
		"  int64 id = 1;\n",
		"  optional string note = 3;\n",
		"  repeated float emb = 4;\n",
		"  bytes data = 5;\n",
		"  rpc Get(ThingGetRequest) returns (Thing);\n",
		"message ThingGetRequest {\n  int64 id = 1;\n}\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("proto lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "secret") || strings.Contains(s, "odd") {
		t.Errorf("proto exposes a sensitive or unmapped column:\n%s", s)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "no protobuf mapping for map[string]string") {
		t.Fatalf("diags = %v", diags)
	}
	if _, d := Proto(syntheticGraph(), &Lock{}, "Bad Package"); len(d) != 1 {
		t.Fatalf("package name accepted: %v", d)
	}
}
