// SPDX-License-Identifier: Apache-2.0

//go:build integration

package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMigrateRollbackCLI runs the rollback command end to end: it refuses with
// exit 1 and runs nothing while the migration has no down file, rolls the
// migration back once it has one, and the version is pending again so the next
// apply runs it anew.
func TestMigrateRollbackCLI(t *testing.T) {
	dsn, root := startedProject(t)
	if code, _, errOut := cliRun(t, dsn, "apply"); code != 0 {
		t.Fatalf("apply: exit %d: %s", code, errOut)
	}
	code, _, errOut := cliRun(t, dsn, "rollback")
	if code != 1 || !strings.Contains(errOut, "down") {
		t.Fatalf("rollback without a down file: exit %d, want 1 naming the missing down: %s", code, errOut)
	}
	if code, out, _ := cliRun(t, dsn, "plan"); code != 0 || !strings.Contains(out, "no pending migrations") {
		t.Fatalf("a refused rollback must change nothing, plan said %q", out)
	}

	ups, err := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*.up.sql"))
	if err != nil || len(ups) != 1 {
		t.Fatalf("want one up file, got %v %v", ups, err)
	}
	writeFile(t, strings.TrimSuffix(ups[0], ".up.sql")+".down.sql", "DROP TABLE tenants;\n")
	if code, _, errOut := cliRun(t, dsn, "hash"); code != 0 {
		t.Fatalf("hash: exit %d: %s", code, errOut)
	}
	code, out, errOut := cliRun(t, dsn, "rollback", "--group")
	if code != 0 || !strings.Contains(out, "rolled back") {
		t.Fatalf("rollback: exit %d, out %q, err %q", code, out, errOut)
	}
	if code, out, _ := cliRun(t, dsn, "plan"); code != 0 || !strings.Contains(out, "pending") || strings.Contains(out, "no pending") {
		t.Fatalf("the rolled back migration must be pending again, plan said %q", out)
	}
	if code, out, errOut := cliRun(t, dsn, "apply"); code != 0 || !strings.Contains(out, "applied") {
		t.Fatalf("apply after rollback: exit %d, out %q, err %q", code, out, errOut)
	}
}
