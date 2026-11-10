// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rebaseCLI(dir string) (*cli, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return &cli{out: &out, errOut: &errOut, cwd: dir}, &out, &errOut
}

func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// Catches: a rebase that removes some branch files, fails on one it cannot
// remove, and leaves a half-rebased tree with the new migration not written.
func TestApplyRebaseChecksEveryPathBeforeChangingAnything(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "20260102000000_a.up.sql"), "SELECT 1;\n")
	writeFile(t, filepath.Join(dir, "20260102000000_a.snapshot.json"), "{}")
	// A path the rebase wants to remove that is a directory, not a file.
	mkdirAll(t, filepath.Join(dir, "20260102000000_a.down.sql"))

	l, _, errOut := rebaseCLI(dir)
	code := l.applyRebase(dir,
		[]string{"20260102000000_a.up.sql", "20260102000000_a.snapshot.json", "20260102000000_a.down.sql"},
		map[string][]byte{"20260103000000_b.up.sql": []byte("SELECT 2;\n")})
	if code != exitFindings || !strings.Contains(errOut.String(), "nothing was changed") {
		t.Fatalf("exit %d, want %d with 'nothing was changed': %s", code, exitFindings, errOut.String())
	}
	for _, f := range []string{"20260102000000_a.up.sql", "20260102000000_a.snapshot.json"} {
		if !pathExists(filepath.Join(dir, f)) {
			t.Errorf("%s was removed although the rebase was refused", f)
		}
	}
	if pathExists(filepath.Join(dir, "20260103000000_b.up.sql")) {
		t.Error("the new migration was written although the rebase was refused")
	}
}

func TestApplyRebaseRemovesThenWrites(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "20260102000000_a.up.sql"), "SELECT 1;\n")
	l, out, errOut := rebaseCLI(dir)
	code := l.applyRebase(dir, []string{"20260102000000_a.up.sql"}, map[string][]byte{"20260103000000_b.up.sql": []byte("SELECT 2;\n")})
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if pathExists(filepath.Join(dir, "20260102000000_a.up.sql")) || !pathExists(filepath.Join(dir, "20260103000000_b.up.sql")) {
		t.Fatalf("old file must be gone and the new one present; output %s", out.String())
	}
}

// A write that fails after the removals says how to recover.
func TestApplyRebasePrintsRecoveryStepsOnAPartialFailure(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "20260102000000_a.up.sql"), "SELECT 1;\n")
	writeFile(t, filepath.Join(dir, "20260103000000_b.up.sql"), "already here\n")
	l, _, errOut := rebaseCLI(dir)
	code := l.applyRebase(dir, []string{"20260102000000_a.up.sql"}, map[string][]byte{"20260103000000_b.up.sql": []byte("SELECT 2;\n")})
	if code == exitOK {
		t.Fatal("writing over an existing file must fail")
	}
	msg := errOut.String()
	if !strings.Contains(msg, "half rebased") || !strings.Contains(msg, "git checkout --") || !strings.Contains(msg, "rebase again") {
		t.Fatalf("the failure must print the recovery steps: %s", msg)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "20260103000000_b.up.sql")); string(got) != "already here\n" {
		t.Fatal("an existing file was overwritten")
	}
}
