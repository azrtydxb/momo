// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/azrtydxb/momo/gen"
)

// Plugin is the gen.Plugin that writes the OpenAPI document and, when
// Options.Handlers is set, the net/http handlers.
type Plugin struct {
	// Dir is the output directory.
	Dir string
	// Package is the Go package name of the handlers; it defaults to the base
	// name of Dir.
	Package string
	// Options is passed to Document and Handlers.
	Options Options
}

// Name returns "openapi".
func (Plugin) Name() string { return "openapi" }

// APIVersion returns gen.PluginAPIVersion.
func (Plugin) APIVersion() int { return gen.PluginAPIVersion }

// Generate writes <Dir>/openapi.yaml through w.Text, and, only when
// Options.Handlers is true, <Dir>/handlers_gen.go through w.File. The
// document is written as YAML, which OpenAPI accepts as a JSON equivalent,
// because a JSON file has no place for the generated-file header the writer
// requires.
//
// A model that cannot be served is left out and named in a comment at the top
// of each output; it does not fail the run. An invalid option does, and so does
// a handlers_gen.go left behind by an earlier run with handlers: true, which
// would otherwise keep serving routes the document no longer lists.
func (p Plugin) Generate(g *gen.Graph, w *gen.Writer) error {
	doc, diags := Document(g, p.Options)
	if doc == nil {
		return &gen.ErrDiagnostics{Diagnostics: diags}
	}
	y, err := toYAML(doc, diags)
	if err != nil {
		return err
	}
	if err := w.Text(filepath.Join(p.Dir, "openapi.yaml"), y); err != nil {
		return err
	}
	stale := filepath.Join(p.Dir, "handlers_gen.go")
	if !p.Options.Handlers {
		if _, err := os.Lstat(stale); err == nil {
			return &gen.ErrDiagnostics{Diagnostics: []gen.Diagnostic{{Msg: stale + " is left over from handlers: true; delete it, or set handlers: true"}}}
		}
		return nil
	}
	pkg := p.Package
	if pkg == "" {
		pkg = filepath.Base(p.Dir)
	}
	src, hdiags := Handlers(g, pkg, p.Options)
	if src == nil {
		return &gen.ErrDiagnostics{Diagnostics: hdiags}
	}
	return w.File(stale, src)
}

// toYAML renders the JSON document as YAML under the generated-file header.
// Map keys are written in sorted order, so the output is deterministic.
func toYAML(doc []byte, notes []gen.Diagnostic) ([]byte, error) {
	var v any
	if err := yaml.Unmarshal(doc, &v); err != nil {
		return nil, fmt.Errorf("openapi: document is not valid: %w", err)
	}
	var enc bytes.Buffer
	e := yaml.NewEncoder(&enc)
	e.SetIndent(2)
	if err := e.Encode(v); err != nil {
		return nil, fmt.Errorf("openapi: encode YAML: %w", err)
	}
	if err := e.Close(); err != nil {
		return nil, fmt.Errorf("openapi: encode YAML: %w", err)
	}
	body := enc.Bytes()
	var b strings.Builder
	b.WriteString("# " + strings.TrimPrefix(gen.Header(), "// ") + "\n")
	for _, n := range notes {
		b.WriteString("# Left out: " + strings.ReplaceAll(n.Msg, "\n", " ") + "\n")
	}
	b.Write(body)
	return []byte(b.String()), nil
}
