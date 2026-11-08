// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	gomod := "module " + crossModule + "\n\ngo 1.26.10\n\n" +
		"require github.com/azrtydxb/momo v0.0.0\n\nreplace github.com/azrtydxb/momo => " + momoRoot + "\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	// go.mod lacks go.sum entries until tidy runs, and Load needs them.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	if out, err := tidy.CombinedOutput(); err != nil {
		if os.Getenv("MOMO_REQUIRE_INTEGRATION") == "1" || os.Getenv("CI") == "true" {
			t.Fatalf("go mod tidy: %v\n%s", err, out)
		}
		t.Skipf("go mod tidy failed (network or module cache unavailable?): %v\n%s", err, out)
	}
	g, diags, err := Load([]string{"./crossa", "./crossb"}, Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	w := NewWriter(WriterOptions{ModuleRoot: root})
	if err := Emit(g, w); err != nil {
		t.Fatal(err)
	}
	for _, o := range w.Outputs() {
		src := string(o.Data)
		inCrossa := strings.Contains(src, `"`+crossModule+`/crossa"`)
		inCrossb := strings.Contains(src, `"`+crossModule+`/crossb`)
		rel, _ := filepath.Rel(root, o.Path)
		switch {
		case strings.HasPrefix(rel, "crossb"+string(filepath.Separator)+"item"):
			if !inCrossa {
				t.Errorf("%s does not import crossa", rel)
			}
		case strings.HasPrefix(rel, "crossa"):
			if inCrossb {
				t.Errorf("%s imports crossb: import cycle", rel)
			}
		}
		if rel == filepath.Join("crossa", "momo_gen.go") || rel == filepath.Join("crossb", "momo_gen.go") {
			if strings.Contains(src, crossModule) {
				t.Errorf("model-package metadata %s imports a fixture package", rel)
			}
		}
	}
	if _, err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	goRun(t, root, "build", "./...")
	goRun(t, root, "vet", "./...")
}

// TestEmitImportsUseImportPath proves descriptor imports of model packages
// are built through ImportPath from the module path and directory.
func TestEmitImportsUseImportPath(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./crossa", "./crossb"}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	var item, owner *Model
	for _, m := range g.Models {
		switch m.GoName {
		case "Item":
			item = m
		case "Owner":
			owner = m
		}
	}
	if item == nil || owner == nil || owner.ModulePath == "" {
		t.Fatal("models or module info missing")
	}
	want := owner.ModulePath + "/gen/testdata/fixtures/crossa"
	src, d := EmitDescriptors(item, g)
	if len(d) != 0 || !strings.Contains(string(src), `"`+want+`"`) {
		t.Fatalf("Item descriptors do not import %s: %v\n%s", want, d, src)
	}
	// A different module path must show up in the import: the path is
	// derived, not copied from the loader's PkgPath.
	owner.ModulePath = "example.com/renamed"
	src, _ = EmitDescriptors(item, g)
	if !strings.Contains(string(src), `"example.com/renamed/gen/testdata/fixtures/crossa"`) {
		t.Fatalf("import not built through ImportPath:\n%s", src)
	}
	// A package directory outside its module is reported, not emitted.
	owner.ModuleDir = filepath.Join(dir, "crossb")
	if _, d = EmitDescriptors(item, g); len(d) == 0 || !strings.Contains(d[0].Msg, "is outside module") {
		t.Fatalf("want outside-module diagnostic, got %v", d)
	}
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
