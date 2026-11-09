// SPDX-License-Identifier: Apache-2.0

package gendocs

import (
	"strings"
	"testing"
)

// Catches: a new driver package that nobody declared, which the generator used
// to guess about from the dialect identifiers in its source, so a docs page
// could say a database is usable (or not) without anyone deciding it.
func TestDriverWithoutDeclarationFailsGeneration(t *testing.T) {
	root := fakeRepo(t, false)
	write(t, root, "driver/newdb/d.go", "package newdb\n\nimport \"github.com/azrtydxb/momo/dialect\"\n\nvar _ = dialect.SQLite\n")
	for _, name := range []string{"status", "capabilities", "features"} {
		_, err := Render(root, name)
		if err == nil {
			t.Fatalf("block %s was generated although driver/newdb has no //momo:driver declaration", name)
		}
		if !strings.Contains(err.Error(), "driver/newdb") {
			t.Errorf("block %s: error %q does not name driver/newdb", name, err)
		}
	}
}
