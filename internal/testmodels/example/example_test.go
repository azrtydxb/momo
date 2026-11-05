// SPDX-License-Identifier: Apache-2.0

package example

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/internal/golden"
)

func TestGenerateCompanion(t *testing.T) {
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: repo})
	if err := gen.Emit(g, w); err != nil {
		t.Fatal(err)
	}
	outs := w.Outputs()
	if len(outs) < 5 {
		t.Fatalf("expected metadata and descriptor files, got %d", len(outs))
	}
	for _, o := range outs {
		ok, err := golden.Diff(o.Path, o.Data, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%s is stale; run `go run ./cmd/momo generate ./internal/testmodels/example/...`", o.Path)
		}
	}
	build := exec.Command("go", "build", "./internal/testmodels/example/...")
	build.Dir = repo
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
}
