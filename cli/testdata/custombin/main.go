// SPDX-License-Identifier: Apache-2.0

// Command custombin is the fixture for the custom-binary test: a momo binary
// built with cli.Main and one generator plugin.
package main

import (
	"path/filepath"

	"github.com/azrtydxb/momo/cli"
	"github.com/azrtydxb/momo/gen"
)

// marker writes custom_gen.go next to the package of the Tenant model.
type marker struct{}

func (marker) Name() string { return "custom" }

func (marker) Generate(g *gen.Graph, w *gen.Writer) error {
	for _, m := range g.Models {
		if m.GoName != "Tenant" {
			continue
		}
		src := gen.Header() + "\n\npackage " + m.PkgName + "\n\n// CustomPlugin marks the output of the custom plugin.\nconst CustomPlugin = true\n"
		return w.File(filepath.Join(m.PkgDir, "custom_gen.go"), []byte(src))
	}
	return nil
}

func main() { cli.Main(marker{}) }
