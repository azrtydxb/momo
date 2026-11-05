// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/testdata/plugins/fixture"
)

func runPlugins(root string, plugins ...gen.Plugin) ([]gen.Output, error) {
	g := &gen.Graph{Plugins: plugins}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	if err := gen.Emit(g, w); err != nil {
		return nil, err
	}
	return w.Outputs(), nil
}

type badNamePlugin struct{}

func (badNamePlugin) Name() string                           { return "Bad-Name" }
func (badNamePlugin) Generate(*gen.Graph, *gen.Writer) error { return nil }

type v2Plugin struct{ badNamePlugin }

func (v2Plugin) Name() string    { return "v2" }
func (v2Plugin) APIVersion() int { return 2 }

func TestGeneratorPlugin(t *testing.T) {
	root := t.TempDir()
	p := fixture.Plugin{Root: root}

	first, err := runPlugins(root, p)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runPlugins(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Path != second[0].Path || !bytes.Equal(first[0].Data, second[0].Data) {
		t.Fatalf("plugin output is not deterministic: %v vs %v", first, second)
	}

	hand := filepath.Join(root, "handwritten", "hand.go")
	if err := os.MkdirAll(filepath.Dir(hand), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hand, []byte("package handwritten\n// mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runPlugins(root, fixture.Plugin{Root: root, Overwrite: true}); !errors.Is(err, gen.ErrHandwritten) {
		t.Fatalf("handwritten target: want ErrHandwritten, got %v", err)
	}
	b, err := os.ReadFile(hand)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "package handwritten\n// mine\n" {
		t.Fatal("handwritten file was modified")
	}

	if _, err := runPlugins(root, badNamePlugin{}); err == nil || !strings.Contains(err.Error(), "invalid plugin name") {
		t.Fatalf("bad name: got %v", err)
	}
	if _, err := runPlugins(root, v2Plugin{}); err == nil || !strings.Contains(err.Error(), "targets API v2") {
		t.Fatalf("api version: got %v", err)
	}
}

func TestPluginsUseWriter(t *testing.T) {
	forbidden := []string{"os.WriteFile(", "os.Create(", "os.OpenFile(", "os.Rename("}
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || filepath.Base(p) == "writer.go" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, f := range forbidden {
			if bytes.Contains(b, []byte(f)) {
				t.Errorf("%s calls %s; generator code must write through gen.Writer", p, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
