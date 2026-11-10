// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"os/exec"
	"strings"
	"testing"
)

// otelImports returns the OpenTelemetry packages among deps.
func otelImports(deps []string) []string {
	var out []string
	for _, d := range deps {
		if strings.Contains(d, "go.opentelemetry.io") {
			out = append(out, d)
		}
	}
	return out
}

// TestOtelImportsDetector keeps TestCoreHasNoOTel honest: the detector finds an
// OpenTelemetry package among a dependency list.
func TestOtelImportsDetector(t *testing.T) {
	got := otelImports([]string{"fmt", "go.opentelemetry.io/otel/trace", "github.com/azrtydxb/momo/dialect"})
	if len(got) != 1 || got[0] != "go.opentelemetry.io/otel/trace" {
		t.Fatalf("otelImports = %v, want the trace package only", got)
	}
}

// TestCoreHasNoOTel: the core module links no OpenTelemetry package. The
// OpenTelemetry hook lives in the separate otelmomo module.
func TestCoreHasNoOTel(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/azrtydxb/momo").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	if bad := otelImports(strings.Fields(string(out))); len(bad) > 0 {
		t.Fatalf("core imports OpenTelemetry: %v", bad)
	}
}
