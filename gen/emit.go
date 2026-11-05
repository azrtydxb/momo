// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ErrDiagnostics is returned by Emit when an emitter reports diagnostics.
// Emit buffers nothing in that case.
type ErrDiagnostics struct {
	Diagnostics []Diagnostic
}

// Error lists one Diagnostic.String line per diagnostic.
func (e *ErrDiagnostics) Error() string {
	lines := make([]string, len(e.Diagnostics))
	for i, d := range e.Diagnostics {
		lines[i] = d.String()
	}
	return "gen: generation failed with diagnostics:\n" + strings.Join(lines, "\n")
}

// pending is one generated file waiting for the diagnostics check.
type pending struct {
	path string
	src  []byte
}

// Emit runs the built-in emitters for every model of g and then the plugins
// of g.Plugins in order, buffering the files in w. Models are grouped by
// package directory (sorted); per package it emits the model-package
// momo_gen.go and, per model, the momo_gen.go of the model's subpackage.
// EmitDescriptors already embeds the EmitLookup output (ColumnByName and
// ColumnNames) in that file, so the lookup is not emitted a second time. If any emitter reports diagnostics they are returned as
// *ErrDiagnostics and nothing is buffered.
//
// After a non-nil error the Writer may hold partial output (files buffered
// before a plugin failed); callers must not Commit it.
//
// Output is byte-deterministic: emitters iterate slices only and the
// grouping below sorts every key, so map iteration order never reaches a file.
func Emit(g *Graph, w *Writer) error {
	for _, p := range g.Plugins {
		if err := validatePlugin(p); err != nil {
			return err
		}
	}
	byPkg := map[string][]*Model{}
	var keys []string
	for _, m := range g.Models {
		key := m.PkgDir + "\x00" + m.PkgPath
		if _, ok := byPkg[key]; !ok {
			keys = append(keys, key)
		}
		byPkg[key] = append(byPkg[key], m)
	}
	sort.Strings(keys)

	var files []pending
	var diags []Diagnostic
	for _, key := range keys {
		models := byPkg[key]
		sort.SliceStable(models, func(i, j int) bool { return models[i].GoName < models[j].GoName })
		dir := models[0].PkgDir

		src, d := EmitMeta(models, g)
		diags = append(diags, d...)
		if len(d) == 0 {
			files = append(files, pending{filepath.Join(dir, "momo_gen.go"), src})
		}
		subOwner := map[string]*Model{}
		for _, m := range models {
			if prev, dup := subOwner[m.SubPkg]; dup {
				diags = append(diags, Diagnostic{Pos: m.Pos, Msg: fmt.Sprintf(
					"models %s and %s of package %s share subpackage %s and would overwrite each other's momo_gen.go",
					prev.GoName, m.GoName, m.PkgName, m.SubPkg)})
				continue
			}
			subOwner[m.SubPkg] = m
			sub := filepath.Join(dir, m.SubPkg, "momo_gen.go")
			desc, d := EmitDescriptors(m, g)
			diags = append(diags, d...)
			if len(d) == 0 {
				files = append(files, pending{sub, desc})
			}
		}
	}
	if len(diags) > 0 {
		return &ErrDiagnostics{Diagnostics: diags}
	}
	for _, f := range files {
		if err := w.File(f.path, f.src); err != nil {
			return err
		}
	}
	for _, p := range g.Plugins {
		if err := p.Generate(g, w); err != nil {
			return fmt.Errorf("plugin %s: %w", p.Name(), err)
		}
	}
	return nil
}
