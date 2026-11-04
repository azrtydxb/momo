// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os/exec"
	"strings"
	"testing"
)

const modPath = "github.com/azrtydxb/momo"

// coreAllowed lists the module-internal packages the core may depend on.
var coreAllowed = map[string]bool{
	modPath:              true,
	modPath + "/dialect": true,
	modPath + "/schema":  true,
	modPath + "/vector":  true,
	modPath + "/types":   true,
}

func TestImportGraph(t *testing.T) {
	for _, pkg := range []string{modPath, modPath + "/dialect", modPath + "/schema", modPath + "/vector"} {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", pkg)
		cmd.Dir = repoRoot(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Fields(string(out)) {
			if !coreAllowed[dep] {
				t.Errorf("%s depends on %s, which the core may not import", pkg, dep)
			}
		}
	}
}
