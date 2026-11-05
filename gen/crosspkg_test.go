// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const (
	fixturesModule = "github.com/azrtydxb/momo/gen/testdata/fixtures"
	crossModule    = "example.com/cross"
)

func TestGenerateCrossPackage(t *testing.T) {
	src, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	// Only the two cross-package fixtures: the rest of the tree holds
	// deliberately broken packages that would fail the final build.
	for _, pkg := range []string{"crossa", "crossb"} {
		if err := copyTree(filepath.Join(src, pkg), filepath.Join(root, pkg)); err != nil {
			t.Fatal(err)
		}
	}
	momoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	gomod := "module " + crossModule + "\n\ngo 1.26\n\n" +
		"require github.com/azrtydxb/momo v0.0.0\n\nreplace github.com/azrtydxb/momo => " + momoRoot + "\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	// go.mod lacks go.sum entries until tidy runs, and Load needs them.
	goRun(t, root, "mod", "tidy")
	g, diags, err := Load([]string{"./crossa", "./crossb"}, Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	w := NewWriter(WriterOptions{ModuleRoot: root})
	if err := Emit(g, w); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	goRun(t, root, "build", "./...")
}

func goRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %v: %v\n%s", args, err, out)
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		// The temporary module must not share a path prefix with the momo
		// module it replaces, or Go reports an ambiguous import.
		b = bytes.ReplaceAll(b, []byte(fixturesModule), []byte(crossModule))
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
}
