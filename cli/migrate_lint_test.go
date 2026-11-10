// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

const lintSnapshot = `{"format_version":1,"version":"20260101000000","parent":"","schema":{"format_version":1,"dialect":"postgres","tables":[{"name":"docs","columns":[{"name":"id","type":"integer"}]}]}}`

// lintDir writes a migration directory whose second migration holds body and
// returns the --config argument for it.
func lintDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	mdir := filepath.Join(dir, "m")
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(mdir)+"\n")
	writeFile(t, filepath.Join(mdir, "20260101000000_init.up.sql"), "CREATE TABLE docs (id integer);\n")
	writeFile(t, filepath.Join(mdir, "20260101000000_init.snapshot.json"), lintSnapshot)
	writeFile(t, filepath.Join(mdir, "20260102000000_change.up.sql"), body)
	return cfg
}

// Catches: lint that reports an error but exits 0 would let a risky migration
// through a CI gate; one that fails on warnings would block the staged flow.
func TestMigrateLintExitCodes(t *testing.T) {
	cases := []struct {
		name string
		body string
		code int
		want string
	}{
		{"error finding", "ALTER TABLE docs ADD COLUMN tenant bigint NOT NULL;\n", 1, "not-null-without-default"},
		{"blocking index", "CREATE INDEX docs_id ON docs (id);\n", 1, "blocking-index"},
		{"warning only", "ALTER TABLE docs ALTER COLUMN id SET NOT NULL;\n", 0, "warning 20260102000000"},
		{"clean", "ALTER TABLE docs ADD COLUMN note text;\n", 0, "no lint findings"},
	}
	for _, c := range cases {
		var out, errOut bytes.Buffer
		code := runMigrate(&out, &errOut, []string{"--config", lintDir(t, c.body), "lint"}, noEnv)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d (%s %s)", c.name, code, c.code, out.String(), errOut.String())
		}
		if got := out.String(); !strings.Contains(got, c.want) {
			t.Errorf("%s: output lacks %q: %s", c.name, c.want, got)
		}
		if c.code == 1 && !strings.Contains(out.String(), "alternative:") {
			t.Errorf("%s: a finding must print its alternative: %s", c.name, out.String())
		}
	}
}

func TestMigrateRollbackAndRebaseNeedTheirInputs(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	for name, args := range map[string][]string{
		"rollback without a database url": {"--config", cfg, "rollback"},
		"rollback bad lock wait":          {"--config", cfg, "rollback", "--lock-wait", "soon", "--url", "postgres://u:p@127.0.0.1:1/x"},
		"rebase without --onto":           {"--config", cfg, "rebase"},
		"rebase onto a missing directory": {"--config", cfg, "rebase", "--onto", filepath.Join(dir, "nope")},
		"lint stray argument":             {"--config", cfg, "lint", "extra"},
		"drift --strict is a known flag":  {"--config", cfg, "drift", "--strict"},
	} {
		var out, errOut bytes.Buffer
		if code := runMigrate(&out, &errOut, args, noEnv); code != 2 {
			t.Errorf("%s: exit %d, want 2 (%s)", name, code, errOut.String())
		}
	}
}
