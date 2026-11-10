// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateAfterFieldRename follows the steps of issue #62: generate,
// rename a field so the generated file is stale, generate again.
func TestGenerateAfterFieldRename(t *testing.T) {
	root := t.TempDir()
	momoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	model := func(field string) string {
		return "package models\n\nimport \"github.com/azrtydxb/momo\"\n\n" +
			"type KbChunk struct {\n\tmomo.Model `momo:\"table:kb_chunks\"`\n" +
			"\tID int64 `momo:\"pk,identity\"`\n\t" + field + " string `momo:\"notnull\"`\n}\n"
	}
	write("go.mod", "module example.com/stalegen\n\ngo 1.26.10\n\n"+
		"require github.com/azrtydxb/momo v0.0.0\n\nreplace github.com/azrtydxb/momo => "+momoRoot+"\n")
	if err := os.Mkdir(filepath.Join(root, "models"), 0o750); err != nil {
		t.Fatal(err)
	}
	modelFile := filepath.Join("models", "kb.go")
	write(modelFile, model("Model2"))
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	if out, err := tidy.CombinedOutput(); err != nil {
		if os.Getenv("MOMO_REQUIRE_INTEGRATION") == "1" || os.Getenv("CI") == "true" {
			t.Fatalf("go mod tidy: %v\n%s", err, out)
		}
		t.Skipf("go mod tidy failed (network or module cache unavailable?): %v\n%s", err, out)
	}

	run := func() {
		t.Helper()
		g, diags, err := Load([]string{"./models"}, Config{Dir: root})
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
	}
	gen := filepath.Join(root, "models", "momo_gen.go")
	run()
	first, err := os.ReadFile(gen) // #nosec G304 -- a path inside the test's temp dir
	if err != nil || !strings.Contains(string(first), "Model2") {
		t.Fatalf("first run did not write code for Model2: %v", err)
	}

	write(modelFile, model("EmbeddingModel"))
	// Without the fix this stops on the stale momo_gen.go.
	run()
	second, err := os.ReadFile(gen) // #nosec G304 -- a path inside the test's temp dir
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(second), "Model2") || !strings.Contains(string(second), "EmbeddingModel") {
		t.Fatalf("regenerated file still refers to the old field:\n%s", second)
	}
}
