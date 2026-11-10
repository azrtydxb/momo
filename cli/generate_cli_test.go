// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFromNestedPackageDir(t *testing.T) {
	root := copyModule(t, exampleSrc)
	if err := os.WriteFile(filepath.Join(root, "momo.yaml"), []byte("version: 1\nplugins:\n  nosuch: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	chdir(t, filepath.Join(root, "billing"))
	var out, errb bytes.Buffer
	if code := Run([]string{"generate"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "nosuch") {
		t.Fatalf("module-root momo.yaml not found from package dir: exit %d, %q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "gen.Emit") {
		t.Fatalf("unknown plugin error does not point to the Go API: %q", errb.String())
	}
}

// TestGoGenerateDirective runs what `go generate` runs: the command from the
// directive in the package directory, with the directive's arguments.
func TestGoGenerateDirective(t *testing.T) {
	root := copyModule(t, exampleSrc)
	for _, pkg := range []string{"", "billing"} {
		src, err := os.ReadFile(filepath.Join(root, pkg, "models.go")) //nolint:gosec // test path
		if err != nil {
			t.Fatal(err)
		}
		const prefix = "//go:generate go run github.com/azrtydxb/momo/cmd/momo "
		var args []string
		for _, l := range strings.Split(string(src), "\n") {
			if rest, ok := strings.CutPrefix(l, prefix); ok {
				args = strings.Fields(rest)
			}
		}
		if len(args) == 0 {
			t.Fatalf("%q: no go:generate directive", pkg)
		}
		chdir(t, filepath.Join(root, pkg))
		var out, errb bytes.Buffer
		if code := Run(args, &out, &errb); code != 0 {
			t.Fatalf("%q: go generate args %v: exit %d\n%s", pkg, args, code, errb.String())
		}
		want := filepath.Join(root, "document", "momo_gen.go")
		if pkg == "billing" {
			want = filepath.Join(root, "billing", "invoice", "momo_gen.go")
		}
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("output missing: %v", err)
		}
		out.Reset()
		if code := Run(append(args, "--check"), &out, &errb); code != 0 {
			t.Fatalf("%q: check after generate: exit %d\n%s%s", pkg, code, out.String(), errb.String())
		}
	}
}

func TestGenerateMissingPattern(t *testing.T) {
	root := copyModule(t, exampleSrc)
	chdir(t, root)
	var out, errb bytes.Buffer
	if code := Run([]string{"generate", "./nosuchdir/..."}, &out, &errb); code != 2 {
		t.Fatalf("exit %d want 2", code)
	}
	if want := "momo: pattern \"./nosuchdir/...\" matches no package directory\n"; errb.String() != want {
		t.Fatalf("stderr %q want %q", errb.String(), want)
	}
}

func TestGenerateConfigFlagAndPatternBase(t *testing.T) {
	root := copyModule(t, exampleSrc)
	chdir(t, root)
	if err := os.MkdirAll("conf", 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("conf/momo.yaml", []byte("version: 1\npackages: [\"./nonexistent\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"-c", "--config"} {
		var out, errb bytes.Buffer
		// CLI patterns resolve against cwd, not against conf/.
		if code := Run([]string{"generate", flag, "conf/momo.yaml", "./billing"}, &out, &errb); code != 0 {
			t.Fatalf("%s: exit %d: %s", flag, code, errb.String())
		}
		// Patterns from the file resolve against the config directory.
		errb.Reset()
		if code := Run([]string{"generate", flag, "conf/momo.yaml"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "./nonexistent") {
			t.Fatalf("%s: config pattern: exit %d: %s", flag, code, errb.String())
		}
	}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{"generate", "-h"}, {"generate", "--help"}, {"help", "generate"}} {
		var out, errb bytes.Buffer
		if code := Run(args, &out, &errb); code != 0 {
			t.Fatalf("%v: exit %d", args, code)
		}
		if !strings.Contains(out.String(), "Usage: momo generate") || errb.Len() != 0 {
			t.Fatalf("%v: stdout %q stderr %q", args, out.String(), errb.String())
		}
	}
	var out, errb bytes.Buffer
	if code := Run([]string{"help", "nosuch"}, &out, &errb); code != 2 {
		t.Fatalf("help nosuch: exit %d", code)
	}
}
