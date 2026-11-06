// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create symlinks here: %v", err)
		}
		t.Fatal(err)
	}
}

// A symlink to a directory is refused with a clear message; a symlink to a
// file is followed and hashed as that file.
func TestWalkSymlinks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "m")
	other := filepath.Join(root, "elsewhere")
	for _, d := range []string{dir, other} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, body string) {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "20260101000000_a.up.sql"), "CREATE TABLE a (id int);\n")
	write(filepath.Join(other, "20250101000000_b.up.sql"), "CREATE TABLE b (id int);\n")
	symlinkOrSkip(t, filepath.Join(other, "20250101000000_b.up.sql"), filepath.Join(dir, "20250101000000_b.up.sql"))

	sum, err := ComputeSum(os.DirFS(dir))
	if err != nil || len(sum.Files) != 2 {
		t.Fatalf("a file symlink must be followed: %v %v", sum, err)
	}
	files, err := Discover(os.DirFS(dir))
	if err != nil || len(files) != 2 || string(files[0].Body) != "CREATE TABLE b (id int);\n" {
		t.Fatalf("Discover must read through a file symlink: %v %v", files, err)
	}

	symlinkOrSkip(t, other, filepath.Join(dir, "linkdir"))
	want := "migrations: linkdir is a symlink to a directory; not supported"
	if _, err := ComputeSum(os.DirFS(dir)); err == nil || err.Error() != want {
		t.Fatalf("ComputeSum: got %v, want %q", err, want)
	}
	if _, err := Discover(os.DirFS(dir)); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Discover: got %v, want %q", err, want)
	}
}
