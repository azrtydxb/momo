// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vectorModels = `package vecmodels

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

// Doc has a vector column, so its migration declares the vector extension.
type Doc struct {
	momo.Model ` + "`momo:\"table:docs\"`" + `
	ID         int64         ` + "`momo:\"pk,identity\"`" + `
	Embedding  vector.Vector ` + "`momo:\"dim:3\"`" + `
}
`

// diffUp runs migrate diff init in a new module with the given migrate: yaml
// and returns the text of the generated up file.
func diffUp(t *testing.T, migrateYAML string) string {
	t.Helper()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "models.go"), vectorModels)
	root := copyModule(t, src)
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\nmigrate:\n  dir:\n    postgres: m\n"+migrateYAML)
	chdir(t, root)
	var out, errOut bytes.Buffer
	if code := runMigrate(&out, &errOut, []string{"diff", "init"}, noEnv); code != 0 {
		t.Fatalf("diff: exit %d: %s", code, errOut.String())
	}
	files, err := filepath.Glob(filepath.Join(root, "m", "*_init.up.sql"))
	if err != nil || len(files) != 1 {
		t.Fatalf("up files %v, %v", files, err)
	}
	b, err := os.ReadFile(files[0]) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestMigrateExternalExtensionsDiff: the vector column makes diff declare the
// extension, unless migrate.external_extensions lists it.
func TestMigrateExternalExtensionsDiff(t *testing.T) {
	if up := diffUp(t, ""); !strings.Contains(up, "EXTENSION IF NOT EXISTS") {
		t.Fatalf("control: the extension must be emitted by default:\n%s", up)
	}
	up := diffUp(t, "  external_extensions: [vector]\n")
	if strings.Contains(up, "EXTENSION") || !strings.Contains(up, "CREATE TABLE") {
		t.Fatalf("an external extension must not be emitted, the table must:\n%s", up)
	}
}

// A bad entry is a configuration error (2) before anything is written.
func TestMigrateExternalExtensionsBadConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  external_extensions: [Vector]\n")
	var out, errOut bytes.Buffer
	if code := runMigrate(&out, &errOut, []string{"--config", cfg, "validate"}, noEnv); code != 2 || !strings.Contains(errOut.String(), "external_extensions") {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
}
