// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Catches: the on_error hook not called by the generated handlers, called with
// the wrong arguments, or its absence changing what compiles.
func TestOnErrorOption(t *testing.T) {
	g := exampleGraph(t)
	with, ds := Handlers(g, "handlers", Options{Title: "t", Version: "1", OnError: "reportError"})
	if with == nil {
		t.Fatal(ds)
	}
	without, ds := Handlers(g, "handlers", Options{Title: "t", Version: "1"})
	if without == nil {
		t.Fatal(ds)
	}
	if n := strings.Count(string(with), "reportError(ctx, op, err)"); n != 1 {
		t.Errorf("the hook is called %d times in writeError, want once:\n%s", n, with)
	}
	if strings.Contains(string(without), "reportError") || strings.Contains(string(without), "(ctx, op, err)") {
		t.Errorf("handlers without on_error call a hook:\n%s", without)
	}
	for name, src := range map[string][]byte{"with": with, "without": without} {
		if _, err := parser.ParseFile(token.NewFileSet(), "handlers_gen.go", src, 0); err != nil {
			t.Errorf("%s the option: %v", name, err)
		}
	}
	for _, bad := range []string{"not a name", "func", "a.b", "1x", "x()"} {
		if src, ds := Handlers(g, "handlers", Options{Title: "t", Version: "1", OnError: bad}); src != nil || len(ds) == 0 {
			t.Errorf("on_error %q accepted", bad)
		}
	}
	// Every answered statement error names its operation.
	for _, op := range []string{`"listDocument"`, `"createDocument"`, `"getDocument"`, `"updateDocument"`, `"deleteDocument"`} {
		if !strings.Contains(string(with), "writeError(w, r, "+op) {
			t.Errorf("no handler answers with the operation name %s", op)
		}
	}
}
