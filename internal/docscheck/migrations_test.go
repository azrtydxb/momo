// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigrationsGuideSections keeps docs/migrations.md covering each part of
// the migration safety and recovery guide that the engine implements.
func TestMigrationsGuideSections(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "migrations.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{
		"momo.sum",
		"momo migrate resolve",
		"-- momo:no-transaction",
		"CONCURRENTLY",
		"momo migrate baseline",
		"momo migrate drift",
		"momo migrate status",
		"## 4. Failure and recovery",
		"## 5. Production safety checklist",
		"### Lock",
		// Task 31 step 2: the sections the guide must carry.
		"### Format versions",
		"## 2. CLI",
		"## 3a. SQLite",
		"momo_migration_lock",
		"**The rebuild**",
		"### Roles and extensions",
		"### NOT NULL columns",
		"--enforce-not-null",
		"--rename-table",
		"ErrBranchConflict",
		"momo migrate rebase",
		"momo migrate lint",
		"momo migrate rollback",
		"## 6. Custom SQL migrations",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("docs/migrations.md lacks %q", want)
		}
	}
	idx, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(idx), "(migrations.md)") {
		t.Error("docs/README.md does not index migrations.md")
	}
}
