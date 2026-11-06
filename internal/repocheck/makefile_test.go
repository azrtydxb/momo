// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os/exec"
	"testing"
)

func TestMakefileTargets(t *testing.T) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make is not installed (for example a Windows runner)")
	}
	targets := []string{
		"test", "test-race", "test-compilefail", "test-fuzz", "test-integration",
		"test-integration-postgres", "bench-smoke", "fmt-check", "vet", "tidy-check",
		"staticcheck", "govulncheck", "gosec", "db-up", "db-down",
	}
	for _, target := range targets {
		cmd := exec.Command(makePath, "-n", target)
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("make -n %s: %v\n%s", target, err, out)
		}
	}
}
