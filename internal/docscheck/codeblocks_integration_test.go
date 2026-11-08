// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docscheck

import (
	"os"
	"os/exec"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// dsnFor returns the DSN of the database testopen.Each provides for d. A
// dialect added by a later milestone adds its case here.
func dsnFor(t *testing.T, d dialect.Name) string {
	t.Helper()
	if d == dialect.Postgres {
		return testdb.PostgresURL(t)
	}
	t.Fatalf("no DSN for dialect %s", d)
	return ""
}

// TestDocsRunnableBlocks executes every block marked docscheck:run against
// each dialect testopen.Each provides, with MOMO_EXAMPLE_DSN set to a fresh
// per-test database.
func TestDocsRunnableBlocks(t *testing.T) {
	root := repoRoot(t)
	var runnable []docBlock
	for _, b := range docBlocks(t, root) {
		if b.run {
			runnable = append(runnable, b)
		}
	}
	if len(runnable) == 0 {
		t.Fatal("no docscheck:run blocks in docs/: the runnable path proves nothing")
	}
	testopen.Each(t, func(t *testing.T, _ *momo.DB, d dialect.Name) {
		cleanGen(t, root)
		dsn := dsnFor(t, d)
		for n, b := range runnable {
			pkg := writeBlock(t, root, n, b.src)
			cmd := exec.Command("go", "run", pkg) //nolint:gosec // fixed arguments, the package path is generated above
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "MOMO_EXAMPLE_DRIVER="+string(d), "MOMO_EXAMPLE_DSN="+dsn)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s failed on %s: %v\n%s", b.where(root), d, err, out)
			}
		}
	})
}
