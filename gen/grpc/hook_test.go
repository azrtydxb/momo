// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Catches: the on_error hook missing from the adapters, called with the wrong
// arguments, or its absence changing what the adapters call; a cancelled call
// answered Internal.
func TestServerOnError(t *testing.T) {
	g := exampleGraph(t)
	lock := &Lock{}
	with, ds := serverSourceHook(g, lock, "example", testPB, nil, "reportError")
	if with == nil {
		t.Fatal(ds)
	}
	without, ds := serverSourceHook(g, lock, "example", testPB, nil, "")
	if without == nil {
		t.Fatal(ds)
	}
	if n := strings.Count(string(with), "reportError(ctx, op, err)"); n != 1 {
		t.Errorf("the hook is called %d times in statusOf, want once", n)
	}
	if strings.Contains(string(without), "reportError") || strings.Contains(string(without), "(ctx, op, err)\n") {
		t.Errorf("adapters without on_error call a hook")
	}
	for name, src := range map[string][]byte{"with": with, "without": without} {
		if _, err := parser.ParseFile(token.NewFileSet(), "server_gen.go", src, 0); err != nil {
			t.Errorf("%s the option: %v", name, err)
		}
	}
	for _, op := range []string{"DocumentService.Get", "DocumentService.List", "DocumentService.Create", "DocumentService.Update", "DocumentService.Delete"} {
		if !strings.Contains(string(with), `statusOf(ctx, "`+op+`"`) {
			t.Errorf("no call answers with the operation name %s", op)
		}
	}
	for _, want := range []string{"codes.Canceled", "codes.DeadlineExceeded", "errors.Is(err, context.Canceled)", "errors.Is(err, context.DeadlineExceeded)"} {
		if !strings.Contains(string(with), want) {
			t.Errorf("adapters lack %q", want)
		}
	}
	for _, bad := range []string{"not a name", "func", "a.b", "x()"} {
		if src, ds := serverSourceHook(g, lock, "example", testPB, nil, bad); src != nil || len(ds) == 0 {
			t.Errorf("on_error %q accepted", bad)
		}
	}
}
