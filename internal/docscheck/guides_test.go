// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var requiredSections = []string{"## Models", "## Queries", "## Relations", "## Hooks", "## Transactions", "## Migrations", "## Errors"}

// TestMigrationGuidesComplete fails when a migration guide for GORM, Bun or Ent
// users lacks one of the seven sections every guide carries. The Go blocks of
// the guides are compiled by TestDocsCodeBlocks.
func TestMigrationGuidesComplete(t *testing.T) {
	root := repoRoot(t)
	for _, g := range []string{"gorm", "bun", "ent"} {
		b, err := os.ReadFile(filepath.Join(root, "docs", "guides", g+".md")) // #nosec G304 -- fixed names under the repository's docs directory
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range requiredSections {
			if !strings.Contains(string(b), "\n"+h+"\n") {
				t.Errorf("%s guide missing section %q", g, h)
			}
		}
	}
}
