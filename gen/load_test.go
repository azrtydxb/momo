// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixturesDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadPackagesDiagnostics(t *testing.T) {
	dir := fixturesDir(t)
	g, diags, err := Load([]string{"./brokenpkg"}, Config{Dir: dir})
	if err != nil {
		t.Fatalf("infrastructure error: %v", err)
	}
	if g == nil {
		t.Fatal("graph is nil")
	}
	if len(diags) == 0 {
		t.Fatal("no diagnostics for a syntax error")
	}
	d := diags[0].String()
	if !strings.Contains(d, "broken.go:8:") {
		t.Fatalf("diagnostic lacks file:line:col of line 8: %s", d)
	}
	if diags[0].Pos.Line != 8 || diags[0].Pos.Column == 0 || !strings.HasSuffix(diags[0].Pos.Filename, "broken.go") {
		t.Fatalf("bad position: %+v", diags[0].Pos)
	}
}

// A real syntax error cannot be committed as a fixture (the format gate
// refuses unparsable Go), so the test writes one into a temporary module.
func TestLoadSyntaxErrorPositioned(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/syn\n\ngo 1.26\n")
	write("syn.go", "package syn\n\ntype Thing struct {\n\tID int64\n\tName = \"x\"\n}\n")
	_, diags, err := Load([]string{"."}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 {
		t.Fatal("no diagnostics for a syntax error")
	}
	if d := diags[0]; d.Pos.Line != 5 || d.Pos.Column == 0 || !strings.HasSuffix(d.Pos.Filename, "syn.go") {
		t.Fatalf("want syn.go:5:col, got %s", d)
	}
}

func TestLoadTypeErrorPositioned(t *testing.T) {
	_, diags, err := Load([]string{"./typeerrpkg"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 {
		t.Fatal("no diagnostics for a type error")
	}
	if diags[0].Pos.Line != 3 || diags[0].Pos.Column == 0 {
		t.Fatalf("want line 3 with column, got %s", diags[0])
	}
}

func TestLoadMissingPackage(t *testing.T) {
	g, diags, err := Load([]string{"./doesnotexist"}, Config{Dir: fixturesDir(t)})
	if err == nil && len(diags) == 0 {
		t.Fatalf("missing package silently succeeded: graph=%v", g)
	}
}

func TestLoadEmptyPatterns(t *testing.T) {
	_, _, err := Load(nil, Config{Dir: fixturesDir(t)})
	if err == nil {
		t.Fatal("empty pattern list must be an error")
	}
	_, _, err = Load([]string{""}, Config{Dir: fixturesDir(t)})
	if err == nil {
		t.Fatal("blank pattern must be an error")
	}
}

func TestDiagnosticString(t *testing.T) {
	var d Diagnostic
	d.Pos.Filename, d.Pos.Line, d.Pos.Column, d.Msg = "a.go", 3, 7, "boom"
	if got := d.String(); got != "a.go:3:7: boom" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadCopiesPlugins(t *testing.T) {
	g, _, err := Load([]string{"./typeerrpkg"}, Config{Dir: fixturesDir(t), Plugins: []Plugin{nopPlugin{}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Plugins) != 1 {
		t.Fatalf("plugins not copied: %v", g.Plugins)
	}
}

// writeModule writes files into a fresh module and returns its directory.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A rename leaves the previously generated file calling a field that is
// gone. Load must not stop on that file, since generate rewrites it (#62).
func TestLoadIgnoresErrorsInGeneratedFile(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod":      "module example.com/stale\n\ngo 1.26\n",
		"model.go":    "package stale\n\ntype Chunk struct {\n\tID             int64\n\tEmbeddingModel string\n}\n",
		"momo_gen.go": Header() + "\n\npackage stale\n\nfunc (m *Chunk) Old() string { return m.Model2 }\n",
	})
	_, diags, err := Load([]string{"."}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("errors in a generated file must not surface: %v", diags)
	}
}

// A type error in a hand-written file still fails, next to a stale
// generated file.
func TestLoadKeepsErrorsInHandwrittenFile(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod":      "module example.com/stale2\n\ngo 1.26\n",
		"model.go":    "package stale2\n\ntype Chunk struct{ ID int64 }\n\nvar bad int = \"x\"\n",
		"momo_gen.go": Header() + "\n\npackage stale2\n\nfunc (m *Chunk) Old() string { return m.Model2 }\n",
	})
	_, diags, err := Load([]string{"."}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || !strings.HasSuffix(diags[0].Pos.Filename, "model.go") {
		t.Fatalf("want exactly the model.go error, got %v", diags)
	}
}

type nopPlugin struct{}

func (nopPlugin) Name() string                   { return "nop" }
func (nopPlugin) Generate(*Graph, *Writer) error { return nil }
