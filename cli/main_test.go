// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildCustomBinary builds the fixture binary (testdata/custombin), a momo
// built with cli.Main and one plugin named "custom".
func buildCustomBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "custombin")
	if runtime.GOOS == "windows" {
		bin += ".exe" // go build -o adds no suffix, and exec finds none without it
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cli/testdata/custombin") //nolint:gosec // fixed arguments
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the custom binary: %v\n%s", err, out)
	}
	return bin
}

func runBin(t *testing.T, bin, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	cmd := exec.Command(bin, args...) //nolint:gosec // the binary was built by this test
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return code, out.String(), errb.String()
}

// TestCustomBinary builds a binary with cli.Main and a plugin, and checks it
// behaves like `momo generate` with that plugin selectable from momo.yaml.
func TestCustomBinary(t *testing.T) {
	bin := buildCustomBinary(t)
	root := copyModule(t, exampleSrc)
	if err := os.WriteFile(filepath.Join(root, "momo.yaml"), []byte("version: 1\nplugins:\n  custom: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code, out, errs := runBin(t, bin, root, "generate"); code != 0 {
		t.Fatalf("generate: exit %d\n%s%s", code, out, errs)
	}
	got, err := os.ReadFile(filepath.Join(root, "custom_gen.go")) //nolint:gosec // test path
	if err != nil {
		t.Fatalf("the plugin output is missing: %v", err)
	}
	if !strings.Contains(string(got), "CustomPlugin") {
		t.Fatalf("plugin output = %q", got)
	}
	if code, out, errs := runBin(t, bin, root, "generate", "--check"); code != 0 {
		t.Fatalf("generate --check after generate: exit %d\n%s%s", code, out, errs)
	}

	// Stale plugin output is reported like any other generated file.
	drifted := append(append([]byte{}, got...), "\n// drift\n"...)
	if err := os.WriteFile(filepath.Join(root, "custom_gen.go"), drifted, 0o600); err != nil { //nolint:gosec // test path
		t.Fatal(err)
	}
	if code, out, _ := runBin(t, bin, root, "generate", "--check"); code != 1 || !strings.Contains(out, "stale:") {
		t.Fatalf("drifted plugin output: exit %d, stdout %q; want exit 1 and a stale line", code, out)
	}

	if code, out, _ := runBin(t, bin, root, "help"); code != 0 || !strings.Contains(out, "cli.Main") {
		t.Fatalf("help: exit %d, does not describe cli.Main:\n%s", code, out)
	}
}

// TestPluginNotPassedToMain checks that a plugin that is not passed to Run (or
// Main) cannot be enabled from momo.yaml: exit 2 and the Go API hint.
func TestPluginNotPassedToMain(t *testing.T) {
	bin := buildCustomBinary(t) // before chdir: it builds relative to the repository
	root := copyModule(t, exampleSrc)
	if err := os.WriteFile(filepath.Join(root, "momo.yaml"), []byte("version: 1\nplugins:\n  custom: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	var out, errb bytes.Buffer
	if code := Run([]string{"generate"}, &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2\n%s", code, errb.String())
	}
	for _, want := range []string{"custom", "gen.Emit", "cli.Main"} {
		if !strings.Contains(errb.String(), want) {
			t.Fatalf("stderr lacks %q: %q", want, errb.String())
		}
	}
	if _, err := os.Stat(filepath.Join(root, "custom_gen.go")); err == nil {
		t.Fatal("the plugin ran although it was not passed")
	}

	// The custom binary refuses a plugin it was not built with.
	if err := os.WriteFile(filepath.Join(root, "momo.yaml"), []byte("version: 1\nplugins:\n  other: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runBin(t, bin, root, "generate"); code != 2 || !strings.Contains(errs, "gen.Emit") {
		t.Fatalf("plugin not passed to Main: exit %d, stderr %q; want exit 2 and the Go API hint", code, errs)
	}
}
