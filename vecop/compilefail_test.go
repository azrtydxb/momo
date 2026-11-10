// SPDX-License-Identifier: Apache-2.0

package vecop_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCompileFailVectorDistances checks that the distance builders left the
// root package for vecop, and that vecop keeps the metric to vector type
// pairing: a bit-vector column cannot be given to a dense-vector metric. The
// programs live in testdata/compilefail and are built, not run.
func TestCompileFailVectorDistances(t *testing.T) {
	for name, wants := range map[string][]string{
		"moved": {
			"undefined: momo.L2Distance",
			"undefined: momo.InnerProductDistance",
			"undefined: momo.CosineDistance",
			"undefined: momo.L1Distance",
			"undefined: momo.HammingDistance",
			"undefined: momo.JaccardDistance",
			"undefined: momo.DistanceExpr",
		},
		"metricmismatch": {"does not satisfy"},
	} {
		cmd := exec.Command("go", "build", "-gcflags=-e", "-o", os.DevNull, "./testdata/compilefail/"+name)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s compiled; want a compile error", name)
			continue
		}
		for _, want := range wants {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s: want %q in:\n%s", name, want, out)
			}
		}
	}
}
