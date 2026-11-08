// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitepure

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
)

// TestSQLitePureGo is S-89. Vector functions are reported as ErrUnsupported,
// and the package builds and links without cgo: the import graph, resolved
// with CGO_ENABLED=0, holds neither the cgo SQLite driver nor sqlite-vec. CI
// also runs the whole package with CGO_ENABLED=0 (make test-sqlitepure), where
// a build that needs cgo fails to compile.
func TestSQLitePureGo(t *testing.T) {
	_, d, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TABLE p (x int)", nil); err != nil {
		t.Fatal(err)
	}
	_, err = d.Exec(ctx, "SELECT vec_version()", nil)
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("vector function err = %v, want ErrUnsupported", err)
	}
	var me *momo.Error
	if !errors.As(err, &me) || me.Op == "" {
		t.Fatalf("unsupported error has no Op: %+v", me)
	}
	if _, err := d.Query(ctx, "SELECT vec_distance_L2(x, x) FROM p", nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("vector query err = %v, want ErrUnsupported", err)
	}
}

func TestSQLitePureImportGraphHasNoCgoDriver(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.Contains(pkg, "mattn") || strings.Contains(pkg, "sqlite-vec") || pkg == "C" || strings.HasSuffix(pkg, "/driver/sqlitedriver") {
			t.Errorf("the pure-Go package depends on %s", pkg)
		}
	}
	if !strings.Contains(string(out), "modernc.org/sqlite") {
		t.Errorf("modernc.org/sqlite is not in the import graph:\n%s", out)
	}
}
