// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// Catches: a malformed, truncated or incomplete tools.lock, which the protoc
// download step will rely on to verify archives (story 219 must check the
// SHA-256 before running protoc and fail closed).
func TestToolsLockWellFormed(t *testing.T) {
	raw, err := os.ReadFile("tools.lock")
	if err != nil {
		t.Fatal(err)
	}
	sum := regexp.MustCompile(`^[0-9a-f]{64}$`)
	got := map[string]string{}
	for i, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || !sum.MatchString(f[1]) {
			t.Fatalf("tools.lock line %d is not \"<archive> <sha256>\": %q", i+1, line)
		}
		if _, dup := got[f[0]]; dup {
			t.Fatalf("tools.lock lists %s twice", f[0])
		}
		got[f[0]] = f[1]
	}
	for _, name := range []string{"protoc-36.2-osx-aarch_64.zip", "protoc-36.2-linux-x86_64.zip", "protoc-36.2-win64.zip"} {
		if got[name] == "" {
			t.Errorf("tools.lock has no entry for %s", name)
		}
	}
	if len(got) != 3 {
		t.Errorf("tools.lock has %d entries, want 3", len(got))
	}
}

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
