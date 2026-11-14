// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Catches: --schema being read on a dialect that has one namespace per
// database, or being ignored there, so a run that asked for a schema inspects
// the whole database and writes models the user did not ask for.
func TestGenerateFromDBSchemaFlagPostgresOnly(t *testing.T) {
	cases := []struct{ dialect, src string }{
		{"mysql", "mysql://user:secret@127.0.0.1:3306/app"},
		{"sqlite", "app.db"},
	}
	for _, c := range cases {
		t.Run(c.dialect, func(t *testing.T) {
			root := t.TempDir()
			chdir(t, root)
			writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\npackages:\n  - ./models\ndialects: ["+c.dialect+"]\n")
			var out, errb bytes.Buffer
			code := Run([]string{"generate", "--from-db", c.src, "--schema", "a"}, &out, &errb)
			if code != exitUsage || !strings.Contains(errb.String(), "--schema applies to PostgreSQL only") {
				t.Fatalf("exit %d, %q", code, errb.String())
			}
			if strings.Contains(errb.String(), "secret") {
				t.Fatalf("the error leaks the URL password: %q", errb.String())
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if e.Name() != "momo.yaml" {
					t.Fatalf("a refused run wrote %s", e.Name())
				}
			}
		})
	}
}
