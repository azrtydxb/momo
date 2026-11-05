// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// leftovers lists the .momo-*.tmp files in dir.
func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, ".momo-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func readOrEmpty(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // test path
	if err != nil {
		return ""
	}
	return string(b)
}

func TestWriteNewFilesPublishes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "m")
	got, err := writeNewFiles(dir, map[string][]byte{
		"momo.sum":                  []byte("sum"),
		"20260101000000_a.up.sql":   []byte("up"),
		"20260101000000_a.snapshot": []byte("snap"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || filepath.Base(got[2]) != "momo.sum" {
		t.Fatalf("published %v: momo.sum must come last", got)
	}
	if len(leftovers(t, dir)) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers(t, dir))
	}
	if readOrEmpty(t, filepath.Join(dir, "20260101000000_a.up.sql")) != "up" || readOrEmpty(t, filepath.Join(dir, "momo.sum")) != "sum" {
		t.Fatal("content not published")
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, "20260101000000_a.up.sql"))
		if err != nil || fi.Mode().Perm() != 0o644 {
			t.Fatalf("mode %v, %v", fi.Mode(), err)
		}
	}
}

// A pre-existing target is refused without touching anything.
func TestWriteNewFilesRefusesExisting(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "20260101000000_a.up.sql")
	writeFile(t, target, "original")
	writeFile(t, filepath.Join(dir, "momo.sum"), "old sum")
	_, err := writeNewFiles(dir, map[string][]byte{
		"20260101000000_a.up.sql":        []byte("new"),
		"20260101000000_a.snapshot.json": []byte("snap"),
		"momo.sum":                       []byte("new sum"),
	})
	if !errors.Is(err, iofs.ErrExist) {
		t.Fatalf("want ErrExist, got %v", err)
	}
	if readOrEmpty(t, target) != "original" || readOrEmpty(t, filepath.Join(dir, "momo.sum")) != "old sum" {
		t.Fatal("a refused write changed files")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "20260101000000_a.snapshot.json")); statErr == nil {
		t.Fatal("the sibling file was published although the write was refused")
	}
	if len(leftovers(t, dir)) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers(t, dir))
	}
}

// publishNew is the race-free guard: it must refuse even when the target
// appeared after the early check, and must never replace it.
func TestPublishNewNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, ".momo-x.tmp")
	final := filepath.Join(dir, "20260101000000_a.up.sql")
	writeFile(t, tmp, "new")
	writeFile(t, final, "winner")
	if err := publishNew(tmp, final, []byte("new")); !errors.Is(err, iofs.ErrExist) {
		t.Fatalf("want ErrExist, got %v", err)
	}
	if readOrEmpty(t, final) != "winner" {
		t.Fatal("publishNew replaced an existing file")
	}
	// A free target gets the content.
	free := filepath.Join(dir, "20260101000001_b.up.sql")
	if err := publishNew(tmp, free, []byte("new")); err != nil || readOrEmpty(t, free) != "new" {
		t.Fatalf("publish to a free name: %v %q", err, readOrEmpty(t, free))
	}
}

// A failure while publishing momo.sum, the last step, removes the new
// migration files already published and every temporary.
func TestWriteNewFilesRollsBackWhenSumFails(t *testing.T) {
	dir := t.TempDir()
	// A non-empty directory named momo.sum makes the final rename fail.
	writeFile(t, filepath.Join(dir, "momo.sum", "keep"), "x")
	_, err := writeNewFiles(dir, map[string][]byte{
		"20260101000000_a.up.sql":        []byte("up"),
		"20260101000000_a.snapshot.json": []byte("snap"),
		"momo.sum":                       []byte("sum"),
	})
	if err == nil {
		t.Fatal("expected the sum publish to fail")
	}
	for _, n := range []string{"20260101000000_a.up.sql", "20260101000000_a.snapshot.json"} {
		if _, statErr := os.Stat(filepath.Join(dir, n)); statErr == nil {
			t.Errorf("%s stayed published after the failure", n)
		}
	}
	if len(leftovers(t, dir)) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers(t, dir))
	}
}

func TestWriteNewFilesRejectsEscapingNames(t *testing.T) {
	for _, n := range []string{"../x.sql", "a/b.sql", ".hidden", ".."} {
		if _, err := writeNewFiles(t.TempDir(), map[string][]byte{n: []byte("x")}); err == nil {
			t.Errorf("%q must be refused", n)
		}
	}
}

// momo migrate hash: the remedy the validate and apply messages name.
func TestMigrateHashRepairsSum(t *testing.T) {
	dir := t.TempDir()
	mdir := filepath.Join(dir, "m")
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(mdir)+"\n")
	writeFile(t, filepath.Join(mdir, "20260101000000_a.up.sql"), "CREATE TABLE a (id int);\n")
	writeFile(t, filepath.Join(mdir, ".DS_Store"), "junk")
	var out, errOut bytes.Buffer
	if code := runMigrate(&out, &errOut, []string{"--config", cfg, "validate"}, noEnv); code != 1 || !strings.Contains(out.String(), "momo migrate hash") {
		t.Fatalf("missing sum: exit %d, out %q", code, out.String())
	}
	out.Reset()
	if code := runMigrate(&out, &errOut, []string{"--config", cfg, "hash"}, noEnv); code != 0 {
		t.Fatalf("hash: exit %d: %s", code, errOut.String())
	}
	if len(leftovers(t, mdir)) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers(t, mdir))
	}
	// validate still has no snapshot to complain about only through sum: filter on sum problems.
	out.Reset()
	runMigrate(&out, &errOut, []string{"--config", cfg, "validate"}, noEnv)
	if strings.Contains(out.String(), "momo.sum") {
		t.Fatalf("validate still reports momo.sum after hash: %s", out.String())
	}
	if strings.Contains(readOrEmpty(t, filepath.Join(mdir, "momo.sum")), ".DS_Store") {
		t.Fatal("momo.sum records a dotfile")
	}
	// An edit makes the sum stale; hash accepts it as the new baseline.
	writeFile(t, filepath.Join(mdir, "20260101000000_a.up.sql"), "CREATE TABLE a (id bigint);\n")
	out.Reset()
	runMigrate(&out, &errOut, []string{"--config", cfg, "validate"}, noEnv)
	if !strings.Contains(out.String(), "momo.sum mismatch") {
		t.Fatalf("an edit must be reported: %s", out.String())
	}
	if code := runMigrate(&out, &errOut, []string{"--config", cfg, "hash"}, noEnv); code != 0 {
		t.Fatalf("second hash: exit %d", code)
	}
	out.Reset()
	runMigrate(&out, &errOut, []string{"--config", cfg, "validate"}, noEnv)
	if strings.Contains(out.String(), "momo.sum") {
		t.Fatalf("validate still reports momo.sum: %s", out.String())
	}
}

func TestMigrateHashUsageErrors(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/none\n")
	for name, args := range map[string][]string{
		"missing directory": {"--config", cfg, "hash"},
		"stray argument":    {"--config", cfg, "hash", "extra"},
	} {
		var out, errOut bytes.Buffer
		if code := runMigrate(&out, &errOut, args, noEnv); code != 2 {
			t.Errorf("%s: exit %d, want 2 (%s)", name, code, errOut.String())
		}
	}
}

// An unreadable migration directory is an execution error (3), not usage.
func TestMigrateValidateUnreadableDirectoryExitsThree(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced here")
	}
	dir := t.TempDir()
	mdir := filepath.Join(dir, "m")
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(mdir)+"\n")
	mkdirAll(t, mdir)
	if err := os.Chmod(mdir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(mdir, 0o750) }) //nolint:gosec // restore so TempDir can clean up
	var out, errOut bytes.Buffer
	if code := runMigrate(&out, &errOut, []string{"--config", cfg, "validate"}, noEnv); code != 3 {
		t.Fatalf("exit %d, want 3: %s", code, errOut.String())
	}
}

func TestMigrateHelpAndStrayArguments(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	const u = "postgres://u:p@127.0.0.1:1/x"
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"baseline -h", []string{"--config", cfg, "baseline", "-h"}, 0},
		{"resolve -h", []string{"--config", cfg, "resolve", "-h"}, 0},
		{"apply -h", []string{"--config", cfg, "apply", "-h"}, 0},
		{"validate stray", []string{"--config", cfg, "validate", "stray"}, 2},
		{"plan stray", []string{"--config", cfg, "plan", "--url", u, "stray"}, 2},
		{"apply stray", []string{"--config", cfg, "apply", "--url", u, "stray"}, 2},
		{"status stray", []string{"--config", cfg, "status", "--url", u, "stray"}, 2},
		{"drift stray", []string{"--config", cfg, "drift", "--url", u, "stray"}, 2},
		{"baseline stray", []string{"--config", cfg, "baseline", "--version", "1", "--url", u, "stray"}, 2},
		{"resolve stray", []string{"--config", cfg, "resolve", "--version", "1", "--applied", "--url", u, "stray"}, 2},
	}
	for _, c := range cases {
		var out, errOut bytes.Buffer
		if got := runMigrate(&out, &errOut, c.args, noEnv); got != c.want {
			t.Errorf("%s: exit %d, want %d (%s)", c.name, got, c.want, errOut.String())
		}
	}
}

// A stray positional that is a pasted URL must not leak its password.
func TestMigrateStrayArgumentIsRedacted(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	var out, errOut bytes.Buffer
	code := runMigrate(&out, &errOut, []string{"--config", cfg, "validate", "postgres://u:topsecret@h/db"}, noEnv)
	if code != 2 || strings.Contains(out.String()+errOut.String(), "topsecret") {
		t.Fatalf("exit %d, output %q %q", code, out.String(), errOut.String())
	}
}
