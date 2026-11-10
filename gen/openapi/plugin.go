// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"fmt"
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
func (p Plugin) Generate(g *gen.Graph, w *gen.Writer) error {
	doc, diags := Document(g, p.Options)
	if len(diags) > 0 {
		return &gen.ErrDiagnostics{Diagnostics: diags}
	}
	y, err := toYAML(doc)
	if err != nil {
		return err
	}
	if err := w.Text(filepath.Join(p.Dir, "openapi.yaml"), y); err != nil {
		return err
	}
	if !p.Options.Handlers {
		return nil
	}
	pkg := p.Package
	if pkg == "" {
		pkg = filepath.Base(p.Dir)
	}
	src, diags := Handlers(g, pkg, p.Options)
	if len(diags) > 0 {
		return &gen.ErrDiagnostics{Diagnostics: diags}
	}
	return w.File(filepath.Join(p.Dir, "handlers_gen.go"), src)
}

// toYAML renders the JSON document as YAML under the generated-file header.
// Map keys are written in sorted order, so the output is deterministic.
func toYAML(doc []byte) ([]byte, error) {
	var v any
	if err := yaml.Unmarshal(doc, &v); err != nil {
		return nil, fmt.Errorf("openapi: document is not valid: %w", err)
	}
	body, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("openapi: encode YAML: %w", err)
	}
	var b strings.Builder
	b.WriteString("# " + strings.TrimPrefix(gen.Header(), "// ") + "\n")
	b.Write(body)
	return []byte(b.String()), nil
}
