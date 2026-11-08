// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/config"
	"github.com/azrtydxb/momo/gen/viz"
	"github.com/azrtydxb/momo/schema"
)

const vizUsage = `Usage: momo viz [--format mermaid|dot] [--snapshot <file>] [-c momo.yaml] [--out <file>] [patterns...]

Draws an entity-relationship diagram. By default it reads the model packages
matching patterns (or "packages" from momo.yaml, then ./...). With --snapshot
it reads a migration snapshot file instead and draws its foreign keys.

--format picks Mermaid (the default) or Graphviz DOT. Without --out the
diagram goes to stdout.

Exit codes: 0 ok, 1 model diagnostics, 2 usage, configuration or input error.
`

// runViz implements `momo viz`.
func runViz(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "-h" || a == "-help" || a == "--help" {
			_, _ = fmt.Fprint(stdout, vizUsage)
			return exitOK
		}
	}
	fs := flag.NewFlagSet("viz", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var cfgPath, format, snapshot, out string
	fs.StringVar(&cfgPath, "c", "", "path to momo.yaml")
	fs.StringVar(&cfgPath, "config", "", "path to momo.yaml")
	fs.StringVar(&format, "format", "mermaid", "mermaid or dot")
	fs.StringVar(&snapshot, "snapshot", "", "migration snapshot file")
	fs.StringVar(&out, "out", "", "output file (default stdout)")
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(stdout, vizUsage)
			return exitOK
		}
		code := fail(stderr, err)
		_, _ = fmt.Fprintf(stderr, "\n%s", vizUsage)
		return code
	}
	if format != "mermaid" && format != "dot" {
		return fail(stderr, fmt.Errorf("unknown viz format %q (want mermaid or dot)", format))
	}

	var tables []schema.Table
	var edges []viz.Edge
	if snapshot != "" {
		s, err := readSnapshot(snapshot)
		if err != nil {
			return fail(stderr, err)
		}
		tables, edges = viz.TablesFromSchema(s), viz.EdgesFromSchema(s)
	} else {
		g, code := loadVizGraph(fs.Args(), cfgPath, stderr)
		if g == nil {
			return code
		}
		tables, edges = viz.Tables(g), viz.EdgesFromGraph(g)
	}

	_, data, err := viz.Render(format, tables, edges)
	if err != nil {
		return fail(stderr, err)
	}
	if out == "" {
		if _, err := stdout.Write(data); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	}
	if err := os.WriteFile(out, data, 0o644); err != nil { // #nosec G306 -- a diagram is meant to be committed and read, like generated source
		return fail(stderr, err)
	}
	return exitOK
}

func readSnapshot(path string) (*schema.Schema, error) {
	f, err := os.Open(path) // #nosec G304 -- the snapshot path is the user's own command line argument
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	s, err := schema.Load(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return s, nil
}

// loadVizGraph loads the models the way generate does. It returns a nil graph
// and the exit code when loading fails.
func loadVizGraph(patterns []string, cfgPath string, stderr io.Writer) (*gen.Graph, int) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fail(stderr, err)
	}
	file, cfgDir, err := loadConfigFile(cfgPath, cwd)
	if err != nil {
		return nil, fail(stderr, err)
	}
	base := cwd
	if len(patterns) == 0 && len(file.Packages) > 0 {
		patterns, base = file.Packages, cfgDir
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	if err := checkPatterns(patterns, base); err != nil {
		return nil, fail(stderr, err)
	}
	cfg, err := config.ToConfig(file, base)
	if err != nil {
		return nil, fail(stderr, err)
	}
	g, diags, err := gen.Load(patterns, cfg)
	if err != nil {
		return nil, fail(stderr, err)
	}
	if len(diags) > 0 {
		printDiagnostics(stderr, diags, cwd)
		return nil, exitFindings
	}
	return g, exitOK
}
