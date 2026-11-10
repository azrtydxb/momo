// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"os"
	"strings"
	"testing"
)

// Catches: a panic in the application's on_error hook killing the process,
// because grpc-go does not recover a panic in a handler.
func TestOnErrorHookPanicIsRecovered(t *testing.T) {
	src, ds := serverSourceHook(exampleGraph(t), &Lock{}, "example", testPB, nil, "reportError")
	if src == nil {
		t.Fatal(ds)
	}
	s := string(src)
	i := strings.Index(s, "reportError(")
	if i < 0 {
		t.Fatal("no hook call")
	}
	j := strings.LastIndex(s[:i], "func")
	if j < 0 || !strings.Contains(s[j:i], "recover()") {
		t.Errorf("the hook call is not guarded by a recover:\n%s", s[j:i+60])
	}
}

// Catches: the doc of ServerSource saying an update reads the row first, which
// it no longer does.
func TestServerSourceDocDoesNotClaimAReadFirst(t *testing.T) {
	b, err := os.ReadFile("server.go") // #nosec G304 -- a fixed file of this package
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(strings.ReplaceAll(string(b), "//", " ")), " ")
	if strings.Contains(flat, "update reads the row first") {
		t.Error("the ServerSource doc still says an update reads the row first")
	}
}
