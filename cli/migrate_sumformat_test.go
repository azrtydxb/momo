// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hash writes format 2 by default, --sum-format 1 downgrades, and a plain hash
// upgrades a format 1 file again and says so.
func TestMigrateHashSumFormat(t *testing.T) {
	dir := t.TempDir()
	mdir := filepath.Join(dir, "m")
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(mdir)+"\n")
	writeFile(t, filepath.Join(mdir, "20260101000000_a.up.sql"), "CREATE TABLE a (id int);\n")
	hash := func(extra ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := runMigrate(&out, &errOut, append([]string{"--config", cfg, "hash"}, extra...), noEnv)
		return code, out.String(), errOut.String()
	}
	sum := filepath.Join(mdir, "momo.sum")

	if code, _, errOut := hash(); code != 0 {
		t.Fatalf("hash: exit %d: %s", code, errOut)
	}
	if got := readOrEmpty(t, sum); !strings.HasPrefix(got, "# momo.sum format=2\n") || strings.Contains(got, "h1:") {
		t.Fatalf("hash must write format 2 without a directory hash:\n%s", got)
	}

	if code, out, errOut := hash("--sum-format", "1"); code != 0 || !strings.Contains(out, "format 2 -> 1") {
		t.Fatalf("downgrade: exit %d: %s %s", code, out, errOut)
	}
	if got := readOrEmpty(t, sum); !strings.HasPrefix(got, "# momo.sum format=1\nh1:") {
		t.Fatalf("--sum-format 1 must write the directory hash:\n%s", got)
	}

	if code, out, errOut := hash(); code != 0 || !strings.Contains(out, "format 1 -> 2") {
		t.Fatalf("upgrade: exit %d: %s %s", code, out, errOut)
	}
	if got := readOrEmpty(t, sum); !strings.HasPrefix(got, "# momo.sum format=2\n") {
		t.Fatalf("hash must upgrade format 1:\n%s", got)
	}

	if code, _, errOut := hash("--sum-format", "3"); code != 2 || !strings.Contains(errOut, "--sum-format") {
		t.Fatalf("an unknown format is a usage error, got exit %d: %s", code, errOut)
	}
}

func cliSnapshot(version, parent string, tables ...string) string {
	var ts []string
	for _, n := range tables {
		ts = append(ts, `{"name":"`+n+`","columns":[{"name":"id","type":"bigint"}]}`)
	}
	return `{"format_version":1,"version":"` + version + `","parent":"` + parent + `","schema":{"format_version":1,"dialect":"postgres","tables":[` + strings.Join(ts, ",") + `]}}` + "\n"
}

// rebase --renumber moves a pending migration after the newest applied one,
// and the directory still validates.
func TestMigrateRebaseRenumber(t *testing.T) {
	dir := t.TempDir()
	mdir := filepath.Join(dir, "m")
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(mdir)+"\n")
	files := map[string]string{
		"20260101000000_init.up.sql":         "-- momo:generated\nCREATE TABLE base (id bigint);\n",
		"20260101000000_init.snapshot.json":  cliSnapshot("20260101000000", "", "base"),
		"20260102000000_add_a.up.sql":        "-- momo:generated\nCREATE TABLE a (id bigint);\n",
		"20260102000000_add_a.snapshot.json": cliSnapshot("20260102000000", "20260101000000", "base", "a"),
		"20260102000500_add_b.up.sql":        "-- momo:generated\nCREATE TABLE b (id bigint);\n",
		"20260102000500_add_b.snapshot.json": cliSnapshot("20260102000500", "20260101000000", "base", "b"),
	}
	for name, body := range files {
		writeFile(t, filepath.Join(mdir, name), body)
	}
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := runMigrate(&out, &errOut, append([]string{"--config", cfg}, args...), noEnv)
		return code, out.String(), errOut.String()
	}
	if code, _, errOut := run("hash"); code != 0 {
		t.Fatalf("hash: exit %d: %s", code, errOut)
	}
	if code, out, errOut := run("validate"); code != 0 {
		t.Fatalf("validate before: exit %d: %s %s", code, out, errOut)
	}

	if code, _, errOut := run("rebase", "--renumber", "--onto", "x"); code != 2 {
		t.Fatalf("--renumber with --onto is a usage error, got exit %d: %s", code, errOut)
	}
	if code, out, errOut := run("rebase", "--renumber", "--applied", "20260101000000"); code != 0 || !strings.Contains(out, "nothing to renumber") {
		t.Fatalf("nothing is out of order: exit %d: %s %s", code, out, errOut)
	}

	code, out, errOut := run("rebase", "--renumber", "--applied", "20260101000000,20260102000500")
	if code != 0 || !strings.Contains(out, "renumbered: 20260102000000_add_a -> 20260102000501_add_a") {
		t.Fatalf("renumber: exit %d: %s %s", code, out, errOut)
	}
	for _, gone := range []string{"20260102000000_add_a.up.sql", "20260102000000_add_a.snapshot.json"} {
		if _, err := os.Stat(filepath.Join(mdir, gone)); err == nil {
			t.Errorf("%s must be gone", gone)
		}
	}
	for _, here := range []string{"20260102000501_add_a.up.sql", "20260102000501_add_a.snapshot.json"} {
		if _, err := os.Stat(filepath.Join(mdir, here)); err != nil {
			t.Errorf("%s must exist: %v", here, err)
		}
	}
	if code, out, errOut := run("validate"); code != 0 {
		t.Fatalf("validate after: exit %d: %s %s", code, out, errOut)
	}
}
