// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os/exec"
	"testing"
)

// Catches: a missing or unpinned tool dependency (gqlgen, gRPC, protobuf, the
// JSON Schema validator) that the ecosystem generator tests rely on.
func TestIntegrationModuleBuilds(t *testing.T) {
	for _, pkg := range []string{
		"github.com/99designs/gqlgen",
		"google.golang.org/grpc",
		"google.golang.org/protobuf/proto",
		"github.com/santhosh-tekuri/jsonschema/v6",
	} {
		cmd := exec.Command("go", "list", pkg)
		cmd.Dir = "."
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", pkg, err, out)
		}
	}
}
