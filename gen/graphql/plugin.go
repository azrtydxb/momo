// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"fmt"
	"go/format"
	"path/filepath"

	"github.com/azrtydxb/momo/gen"
)

// Plugin is the gen.Plugin that writes the GraphQL schema, the Go glue for a
// gqlgen server and the gqlgen configuration.
//
// The server is split in three packages, because gqlgen's generated executor
// must not import the package that holds the resolvers: <Dir>/model holds the
// Go types and functions the schema binds to, <Dir>/generated is written by
// gqlgen and <Dir> holds the resolvers.
type Plugin struct {
	// Dir is the output directory, the one that holds gqlgen.yml.
	Dir string
	// Package is the Go package name of the resolvers in Dir; it defaults to
	// the base name of Dir. The gqlgen configuration names the package graph,
	// so a different name needs an edit of gqlgen.yml.
	Package string
	// ModelsPackage is the import path of the package that holds the models.
	// When set, every model must be in it.
	ModelsPackage string
	// ImportPath is the import path of Dir. It defaults to the path derived
	// from the module of the models.
	ImportPath string
}

// Name returns "graphql".
func (Plugin) Name() string { return "graphql" }

// APIVersion returns gen.PluginAPIVersion.
func (Plugin) APIVersion() int { return gen.PluginAPIVersion }

// Generate writes <Dir>/schema.graphqls and <Dir>/gqlgen.yml through w.Text,
// <Dir>/model/momo_gen.go through w.File and <Dir>/resolver.go through
// w.Scaffold, so a resolver file you edited is never overwritten. Run gqlgen
// generate in Dir afterwards to write <Dir>/generated/exec.go.
func (p Plugin) Generate(g *gen.Graph, w *gen.Writer) error {
	sdl, diags := SDL(g)
	if len(diags) > 0 {
		return &gen.ErrDiagnostics{Diagnostics: diags}
	}
	pkg := p.Package
	if pkg == "" {
		pkg = filepath.Base(p.Dir)
	}
	glue, diags := GoSource(g, "model", p.ModelsPackage)
	if len(diags) > 0 {
		return &gen.ErrDiagnostics{Diagnostics: diags}
	}
	ms, _ := analyzeGo(g, p.ModelsPackage)
	importPath := p.ImportPath
	if importPath == "" {
		var err error
		if importPath, err = dirImportPath(g, p.Dir); err != nil {
			return err
		}
	}
	scaffold, err := format.Source(resolverScaffold(ms, pkg, importPath))
	if err != nil {
		return fmt.Errorf("graphql: generator bug: formatting the resolver scaffold: %w", err)
	}
	if err := w.Text(filepath.Join(p.Dir, "schema.graphqls"), sdl); err != nil {
		return err
	}
	if err := w.Text(filepath.Join(p.Dir, "gqlgen.yml"), gqlgenConfig(ms, importPath)); err != nil {
		return err
	}
	if err := w.File(filepath.Join(p.Dir, "model", "momo_gen.go"), glue); err != nil {
		return err
	}
	return w.Scaffold(filepath.Join(p.Dir, "resolver.go"), scaffold)
}

// dirImportPath derives the import path of dir from the module of the first
// model.
func dirImportPath(g *gen.Graph, dir string) (string, error) {
	for _, m := range g.Models {
		if m.ModulePath != "" {
			return gen.ImportPath(m.ModulePath, m.ModuleDir, dir)
		}
	}
	return "", fmt.Errorf("graphql: cannot derive the import path of %s: set ImportPath", dir)
}
