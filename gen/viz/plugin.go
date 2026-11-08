// SPDX-License-Identifier: Apache-2.0

package viz

import (
	"fmt"
	"path/filepath"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/schema"
)

// Plugin is the gen.Plugin that writes an ER diagram of the models.
type Plugin struct {
	// Format is "mermaid" (schema.mmd) or "dot" (schema.dot).
	Format string
	// Out is the output directory.
	Out string
}

// Name returns "viz".
func (Plugin) Name() string { return "viz" }

// APIVersion returns gen.PluginAPIVersion.
func (Plugin) APIVersion() int { return gen.PluginAPIVersion }

// Generate writes <Out>/schema.mmd or <Out>/schema.dot through w.Text.
func (p Plugin) Generate(g *gen.Graph, w *gen.Writer) error {
	file, data, err := Render(p.Format, Tables(g), EdgesFromGraph(g))
	if err != nil {
		return err
	}
	return w.Text(filepath.Join(p.Out, file), data)
}

// Render renders tables and edges in format ("mermaid" or "dot") and returns
// the conventional file name with the content. Another format is an error.
func Render(format string, tables []schema.Table, edges []Edge) (file string, data []byte, err error) {
	switch format {
	case "mermaid":
		return "schema.mmd", Mermaid(tables, edges), nil
	case "dot":
		return "schema.dot", DOT(tables, edges), nil
	}
	return "", nil, fmt.Errorf("unknown viz format %q", format)
}
