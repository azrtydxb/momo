// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/apiquery"
	"github.com/azrtydxb/momo/schema"
)

func clashGraph() *gen.Graph {
	id := func() *gen.Field { return &gen.Field{GoName: "ID", Column: "id", BaseType: "int64"} }
	return &gen.Graph{
		Models: []*gen.Model{
			{GoName: "Doc", Table: "docs", Fields: []*gen.Field{id()}},
			{GoName: "DocInput", Table: "doc_inputs", Fields: []*gen.Field{id()}},
		},
		Schema: &schema.Schema{Tables: []schema.Table{
			{Name: "docs", PrimaryKey: []string{"id"}},
			{Name: "doc_inputs", PrimaryKey: []string{"id"}},
		}},
	}
}

// Catches: the name check for <Model>Input dropping a model that has no write
// route and therefore no input schema, and a clash with a model name that is
// not reported when the input schema is written.
func TestInputNameClashOnlyWhenTheInputSchemaIsWritten(t *testing.T) {
	read := Options{Title: "t", Version: "1", Ops: apiquery.Ops{apiquery.DefaultModel: {"list", "get"}}}
	raw, ds := Document(clashGraph(), read)
	if len(ds) != 0 {
		t.Fatalf("a read-only API has no input schema, so there is no clash: %v", ds)
	}
	var d schemaDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Doc", "DocInput"} {
		if _, ok := d.Components.Schemas[name]; !ok {
			t.Errorf("read-only API lost the schema %s", name)
		}
	}
	_, ds = Document(clashGraph(), Options{Title: "t", Version: "1"})
	if len(ds) == 0 || !strings.Contains(ds[0].Msg, "DocInput") {
		t.Fatalf("a write route for Doc must report the clash with the model DocInput: %v", ds)
	}
}

// Catches: handlers: false with on_error set, which would silently ignore the
// hook because only the handlers call it.
func TestOnErrorWithoutHandlersIsReported(t *testing.T) {
	g := exampleGraph(t)
	root := t.TempDir()
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	err := (Plugin{Dir: filepath.Join(root, "api"), Options: Options{Title: "t", Version: "1", OnError: "OnError"}}).Generate(g, w)
	var ed *gen.ErrDiagnostics
	if !errors.As(err, &ed) || !strings.Contains(ed.Diagnostics[0].Msg, "on_error") {
		t.Fatalf("err = %v, want a diagnostic naming on_error", err)
	}
	w = gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	if err := (Plugin{Dir: filepath.Join(root, "api"), Options: Options{Title: "t", Version: "1", Handlers: true, OnError: "OnError"}}).Generate(g, w); err != nil {
		t.Fatalf("on_error with handlers: %v", err)
	}
}

// Catches: a panic in the application's hook taking down the process (a handler
// runs on the server's goroutine; net/http recovers a panic of its own, but the
// hook is called inside the error path, and a recovered panic there must still
// answer the request).
func TestOnErrorHookPanicIsRecovered(t *testing.T) {
	src, ds := Handlers(exampleGraph(t), "handlers", Options{Title: "t", Version: "1", OnError: "reportError"})
	if src == nil {
		t.Fatal(ds)
	}
	s := string(src)
	i := strings.Index(s, "reportError(")
	if i < 0 {
		t.Fatal("no hook call")
	}
	before := s[:i]
	// The call sits in a function literal (or helper) that defers a recover.
	j := strings.LastIndex(before, "func")
	if j < 0 || !strings.Contains(s[j:i], "recover()") {
		t.Errorf("the hook call is not guarded by a recover:\n%s", s[j:i+60])
	}
}
