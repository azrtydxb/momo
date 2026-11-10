// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"testing"

	"github.com/azrtydxb/momo/gen/grpc"
	"github.com/azrtydxb/momo/gen/openapi"
)

// Catches: the on_error option not reaching the openapi and grpc plugins, or
// accepted by a plugin that has no hook (graphql).
func TestPluginOnError(t *testing.T) {
	ps, err := buildPlugins(map[string]map[string]any{
		"openapi": {"dir": "api", "title": "t", "version": "1", "handlers": true, "on_error": "OnError"},
		"grpc":    {"dir": "rpc", "on_error": "OnRPCError"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := ps[0].(grpc.Plugin).OnError; got != "OnRPCError" {
		t.Errorf("grpc on_error = %q", got)
	}
	if got := ps[1].(openapi.Plugin).Options.OnError; got != "OnError" {
		t.Errorf("openapi on_error = %q", got)
	}
	if _, err := buildPlugins(map[string]map[string]any{"graphql": {"dir": "g", "on_error": "OnError"}}, ""); err == nil {
		t.Error("graphql accepted on_error")
	}
	if _, err := buildPlugins(map[string]map[string]any{"grpc": {"dir": "rpc", "on_error": 3}}, ""); err == nil {
		t.Error("a non-string on_error was accepted")
	}
}
