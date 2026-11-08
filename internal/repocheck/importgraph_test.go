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

// deps lists the module and third-party packages pkg imports, directly or not.
func deps(t *testing.T, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", pkg)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}
	return strings.Fields(string(out))
}

func isSQLiteDep(dep string) bool {
	return strings.HasPrefix(dep, modPath+"/driver/sqlitedriver") ||
		strings.Contains(dep, "mattn/go-sqlite3") || strings.Contains(dep, "sqlite-vec-go-bindings")
}

// TestMomotestAvoidsSQLite keeps the C sources of SQLite and sqlite-vec out of
// the build of every test binary that uses momotest for PostgreSQL: SQLite
// lives in momotest/sqlitetest, which a test imports only when it wants it.
func TestMomotestAvoidsSQLite(t *testing.T) {
	for _, dep := range deps(t, modPath+"/momotest") {
		if isSQLiteDep(dep) {
			t.Errorf("momotest depends on %s, which pulls cgo into every user; keep SQLite in momotest/sqlitetest", dep)
		}
	}
	found := false
	for _, dep := range deps(t, modPath+"/momotest/sqlitetest") {
		found = found || dep == modPath+"/driver/sqlitedriver"
	}
	if !found {
		t.Error("momotest/sqlitetest does not import driver/sqlitedriver")
	}
}
