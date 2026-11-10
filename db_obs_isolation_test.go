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

// TestCoreHasNoOTel: no package of the core module links an OpenTelemetry
// package. ./... covers the whole root module and, as a package pattern, stops
// at nested modules, so the OpenTelemetry hook in the otelmomo module is the
// one place that may import it.
func TestCoreHasNoOTel(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "./...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	if bad := otelImports(strings.Fields(string(out))); len(bad) > 0 {
		t.Fatalf("core imports OpenTelemetry: %v", bad)
	}
}
