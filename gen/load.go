// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bufio"
	"errors"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/schema"
)

const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
	packages.NeedDeps | packages.NeedModule

// Load loads the packages matching patterns with type information. Package
// errors become positioned diagnostics; only infrastructure failures (for
// example go list cannot run) are returned as err.
func Load(patterns []string, cfg Config) (*Graph, []Diagnostic, error) {
	if len(patterns) == 0 {
		return nil, nil, errors.New("gen: no package patterns given")
	}
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			return nil, nil, errors.New("gen: empty package pattern")
		}
	}
	pcfg := &packages.Config{Mode: loadMode, Dir: cfg.Dir}
	if len(cfg.Tags) > 0 {
		pcfg.BuildFlags = []string{"-tags=" + strings.Join(cfg.Tags, ",")}
	}
	pkgs, err := packages.Load(pcfg, patterns...)
	if err != nil {
		return nil, nil, fmt.Errorf("gen: load packages: %w", err)
	}

	g := &Graph{
		Models:      []*Model{},
		Projections: []*Model{},
		Schema:      &schema.Schema{},
		Packages:    []*Package{},
		Plugins:     append([]Plugin(nil), cfg.Plugins...),
	}
	var diags []Diagnostic
	var namer naming.Namer = naming.Momo{}
	if cfg.Naming != nil {
		namer = cfg.Naming
	}
	g.Namer = namer
	seen := map[string]bool{}
	for _, p := range pkgs {
		for _, e := range p.Errors {
			d := errorDiagnostic(e)
			// generate is about to rewrite files it generated, so a stale
			// reference in one (a renamed or removed field) must not stop it.
			// Other errors, such as a redeclaration against a hand-written
			// name, are real and stay.
			if staleReference(d.Msg) && isGeneratedFile(d.Pos.Filename, cfg.Dir) {
				continue
			}
			key := d.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			diags = append(diags, d)
		}
		dir := ""
		if len(p.GoFiles) > 0 {
			dir = dirOf(p.GoFiles[0])
		}
		g.Packages = append(g.Packages, &Package{Path: p.PkgPath, Name: p.Name, Dir: dir})
		ms, ps, mdiags := discoverAll(p, namer)
		g.Models = append(g.Models, ms...)
		g.Projections = append(g.Projections, ps...)
		diags = append(diags, mdiags...)
		diags = append(diags, collisionDiagnostics(ms)...)
		diags = append(diags, projectionDiagnostics(ps, cfg)...)
	}
	rdiags, bad := resolveRelations(g.Models)
	diags = append(diags, rdiags...)
	if len(bad) > 0 { // invalid relations mean no output for their package
		kept := g.Models[:0]
		for _, m := range g.Models {
			if !bad[m.PkgPath] {
				kept = append(kept, m)
			}
		}
		g.Models = kept
		keptP := g.Projections[:0]
		for _, m := range g.Projections {
			if !bad[m.PkgPath] {
				keptP = append(keptP, m)
			}
		}
		g.Projections = keptP
	}
	sort.SliceStable(g.Packages, func(i, j int) bool { return g.Packages[i].Path < g.Packages[j].Path })
	sort.SliceStable(g.Models, func(i, j int) bool {
		a, b := g.Models[i], g.Models[j]
		if a.PkgPath != b.PkgPath {
			return a.PkgPath < b.PkgPath
		}
		return a.GoName < b.GoName
	})
	sort.SliceStable(g.Projections, func(i, j int) bool {
		a, b := g.Projections[i], g.Projections[j]
		if a.PkgPath != b.PkgPath {
			return a.PkgPath < b.PkgPath
		}
		return a.GoName < b.GoName
	})
	var sdiags []Diagnostic
	g.Schema, g.viewTables, sdiags = buildSchema(g.Models, cfg)
	diags = append(diags, sdiags...)
	g.Warnings = dbDefaultWarnings(g.Models)
	return g, diags, nil
}

// staleReference reports whether a type error says the code names something
// that no longer exists.
func staleReference(msg string) bool {
	return strings.Contains(msg, "undefined") ||
		strings.Contains(msg, "has no field or method") ||
		strings.Contains(msg, "unknown field")
}

// isGeneratedFile reports whether the file starts with momo's generated
// header. A relative name resolves against dir. Unreadable files are not
// generated.
func isGeneratedFile(name, dir string) bool {
	if name == "" || name == "-" {
		return false
	}
	if !filepath.IsAbs(name) && dir != "" {
		name = filepath.Join(dir, name)
	}
	f, err := os.Open(name) // #nosec G304 -- the path comes from the compiler's own diagnostic for the user's package
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	line, _ := bufio.NewReader(f).ReadString('\n')
	_, ok := ParseHeader(line)
	return ok
}

func dirOf(file string) string {
	if i := strings.LastIndexAny(file, `/\`); i >= 0 {
		return file[:i]
	}
	return "."
}

// errorDiagnostic converts a packages.Error. Pos is "file:line:col",
// "file:line" or "-" / empty; it is parsed from the right so drive-letter
// colons in Windows paths survive.
func errorDiagnostic(e packages.Error) Diagnostic {
	d := Diagnostic{Msg: e.Msg}
	pos := e.Pos
	if pos == "" || pos == "-" {
		d.Pos = token.Position{Filename: "-"}
		return d
	}
	parts := strings.Split(pos, ":")
	var nums []int
	for len(parts) > 1 && len(nums) < 2 {
		n, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			break
		}
		nums = append([]int{n}, nums...)
		parts = parts[:len(parts)-1]
	}
	d.Pos.Filename = strings.Join(parts, ":")
	switch len(nums) {
	case 2:
		d.Pos.Line, d.Pos.Column = nums[0], nums[1]
	case 1:
		d.Pos.Line = nums[0]
	}
	return d
}

// collisionDiagnostics reports, in declaration order, columns produced by two
// fields of one model and model names of one package that differ only in case.
func collisionDiagnostics(models []*Model) []Diagnostic {
	var diags []Diagnostic
	for _, m := range models {
		cols := make([]columnName, 0, len(m.Fields))
		for _, f := range m.Fields {
			cols = append(cols, columnName{Field: f.GoName, Column: f.Column, Pos: f.Pos})
		}
		diags = append(diags, checkColumnCollisions(cols)...)
	}
	return append(diags, checkModelIdentifiers(models)...)
}

// projectionDiagnostics checks that each field type of a projection is one
// the type map knows, as for a model; the error names the struct and the
// field. Column names are positional labels, so repeats are allowed.
func projectionDiagnostics(projections []*Model, cfg Config) []Diagnostic {
	var diags []Diagnostic
	b := &schemaBuilder{namer: cfg.Naming, typeDef: cfg.TypeDefaults, primary: dialect.Postgres}
	if len(cfg.Dialects) > 0 {
		b.primary = cfg.Dialects[0]
	}
	for _, p := range projections {
		for _, f := range p.Fields {
			core, _ := strings.CutPrefix(f.GoType, "*")
			b.diags = nil
			b.columnType(f, core)
			for _, d := range b.diags {
				diags = append(diags, Diagnostic{Pos: d.Pos, Msg: "projection " + p.GoName + ": " + d.Msg})
			}
		}
	}
	return diags
}
