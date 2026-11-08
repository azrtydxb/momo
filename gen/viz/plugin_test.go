// SPDX-License-Identifier: Apache-2.0

package viz

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

// Catches: the plugin writing outside Out, writing the wrong file for a
// format, accepting an unknown format, or emitting without the header.
func TestVizPlugin(t *testing.T) {
	g := loadExample(t)

	root := t.TempDir()
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	p := Plugin{Format: "dot", Out: filepath.Join(root, "viz")}
	if p.Name() != "viz" || p.APIVersion() != gen.PluginAPIVersion {
		t.Fatalf("name=%q api=%d", p.Name(), p.APIVersion())
	}
	if err := p.Generate(g, w); err != nil {
		t.Fatal(err)
	}
	outs := w.Outputs()
	want := filepath.Join(root, "viz", "schema.dot")
	if len(outs) != 1 || outs[0].Path != want {
		var got []string
		for _, o := range outs {
			got = append(got, o.Path)
		}
		t.Fatalf("outputs = %v, want only %s", got, want)
	}
	firstLine := strings.SplitN(string(outs[0].Data), "\n", 2)[0]
	if firstLine != gen.Header() {
		t.Fatalf("first line = %q", firstLine)
	}

	w = gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	if err := (Plugin{Format: "mermaid", Out: filepath.Join(root, "viz")}).Generate(g, w); err != nil {
		t.Fatal(err)
	}
	if outs = w.Outputs(); len(outs) != 1 || outs[0].Path != filepath.Join(root, "viz", "schema.mmd") {
		t.Fatalf("mermaid outputs = %v", outs)
	}

	svg := Plugin{Format: "svg", Out: filepath.Join(root, "viz")}
	if err := svg.Generate(g, gen.NewWriter(gen.WriterOptions{ModuleRoot: root})); err == nil ||
		!strings.Contains(err.Error(), `unknown viz format "svg"`) {
		t.Fatalf("svg: got %v", err)
	}
}
