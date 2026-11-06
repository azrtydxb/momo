// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// createExtension matches a CREATE EXTENSION statement in Go source and
// captures the token naming the extension.
var createExtension = regexp.MustCompile(`(?i)CREATE EXTENSION (?:IF NOT EXISTS )?(\S+)`)

// identPrefix is the extension identifier at the start of the captured token;
// a token starting with a string break ("+ext+") names it at run time.
var identPrefix = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)

// TestTestsKeepExtensionsOutOfSharedDatabase pins that a test which creates a
// database-wide extension other than vector does so in a scratch database.
// The shared test database is inspected by migrate's drift and round-trip
// tests, which report any extra extension as unmanaged; in CI every package
// shares one database, so one such test breaks the others (PR #4, hstore).
func TestTestsKeepExtensionsOutOfSharedDatabase(t *testing.T) {
	root := repoRoot(t)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "testdata", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p) // #nosec G304 -- walking the repository's own test files
		if err != nil {
			return err
		}
		src := string(b)
		if strings.Contains(src, "ScratchDatabase(") {
			return nil
		}
		for _, m := range createExtension.FindAllStringSubmatch(src, -1) {
			name := strings.ToLower(identPrefix.FindString(strings.TrimLeft(m[1], "\"`")))
			if name == "" {
				name = "<built at run time>"
			}
			if name != "vector" {
				rel, _ := filepath.Rel(root, p)
				t.Errorf("%s creates extension %q outside a scratch database; use testdb.ScratchDatabase with testopen.PostgresIn", rel, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
