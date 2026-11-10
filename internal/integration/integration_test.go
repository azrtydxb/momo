// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
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
	// One version, written in the archive names only: scripts/fetch-protoc.sh
	// reads it from here.
	version := ""
	for name := range got {
		parts := strings.SplitN(name, "-", 3)
		if len(parts) != 3 || parts[0] != "protoc" || !strings.HasSuffix(name, ".zip") {
			t.Errorf("tools.lock entry %q is not protoc-<version>-<platform>.zip", name)
			continue
		}
		if version != "" && parts[1] != version {
			t.Errorf("tools.lock mixes protoc versions %s and %s", version, parts[1])
		}
		version = parts[1]
	}
	for _, platform := range []string{"osx-aarch_64", "osx-x86_64", "linux-x86_64", "linux-aarch_64", "win64"} {
		if got["protoc-"+version+"-"+platform+".zip"] == "" {
			t.Errorf("tools.lock has no entry for protoc-%s-%s.zip", version, platform)
		}
	}
	if len(got) != 5 {
		t.Errorf("tools.lock has %d entries, want 5", len(got))
	}
	first := strings.Fields(strings.SplitN(string(raw), "\n", 2)[0])[0]
	if !strings.HasPrefix(first, "protoc-"+version+"-") {
		t.Errorf("the first line of tools.lock names %s, which fetch-protoc.sh reads the version from", first)
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

// Catches: a vendored OpenAPI schema edited or truncated without its recorded
// checksum, which the document validation would then trust.
func TestVendoredSchemasMatchSHA256SUMS(t *testing.T) {
	dir := filepath.Join("openapi", "testdata")
	raw, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for i, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || len(f[0]) != 64 {
			t.Fatalf("SHA256SUMS line %d is not \"<sha256>  <file>\": %q", i+1, line)
		}
		data, err := os.ReadFile(filepath.Join(dir, f[1]))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != f[0] {
			t.Errorf("%s has SHA-256 %s, SHA256SUMS records %s", f[1], got, f[0])
		}
		listed[f[1]] = true
	}
	for _, name := range []string{"oas31-schema.json", "oas31-dialect-base.json"} {
		if !listed[name] {
			t.Errorf("SHA256SUMS does not list %s", name)
		}
	}
}
