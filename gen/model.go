// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/gen/tags"
)

const (
	momoPkgPath   = "github.com/azrtydxb/momo"
	momoModelName = "Model"
	// momoProjectionName is the marker type of a projection.
	momoProjectionName = "Projection"
)

// discoverModels finds the models of one loaded package. It reports
// positioned diagnostics and never guesses: a type is a model only when it is
// a non-generic, non-alias struct that embeds momo.Model by value.
func discoverModels(pkg *packages.Package, namer naming.Namer) ([]*Model, []Diagnostic) {
	ms, _, diags := discoverAll(pkg, namer)
	return ms, diags
}

// discoverAll is discoverModels that also returns the projections: structs
// that embed momo.Projection by value.
func discoverAll(pkg *packages.Package, namer naming.Namer) ([]*Model, []*Model, []Diagnostic) {
	if pkg.TypesInfo == nil || pkg.Types == nil {
		return nil, nil, nil
	}
	fl := newFlattener(pkg, namer)
	var all []*discovery
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok || ts.Assign.IsValid() {
					continue
				}
				d := &discovery{pkg: pkg, namer: namer, ts: ts, st: st, fl: fl}
				d.model = d.run()
				all = append(all, d)
			}
		}
	}
	// A struct without the marker that carries momo tags is only a mistake
	// when no model flattens it as a mixin or embedded struct, so the check
	// waits until every model of the package has been built.
	var models, projections []*Model
	var diags []Diagnostic
	companionBad := false
	for _, d := range all {
		if d.nonModel != nil && !fl.used[d.nonModel] {
			d.checkNonModel()
		}
		diags = append(diags, d.diags...)
		companionBad = companionBad || d.companionBad
		if d.model != nil && d.model.Projection {
			projections = append(projections, d.model)
		} else if d.model != nil {
			models = append(models, d.model)
		}
	}
	if companionBad {
		models, projections = nil, nil // an invalid companion means no output for the package
	}
	return models, projections, dedupeDiagnostics(diags)
}

type discovery struct {
	pkg   *packages.Package
	namer naming.Namer
	ts    *ast.TypeSpec
	st    *ast.StructType
	fl    *flattener
	diags []Diagnostic
	model *Model
	// nonModel is set for a struct without the marker, whose momo tags are
	// checked once mixin use is known.
	nonModel *types.Named
	// companionBad is set when this model's MomoConfig has diagnostics.
	companionBad bool
}

func (d *discovery) diag(pos token.Pos, msg string) {
	d.diags = append(d.diags, Diagnostic{Pos: d.pkg.Fset.Position(pos), Msg: msg})
}

func (d *discovery) run() *Model {
	obj, _ := d.pkg.TypesInfo.Defs[d.ts.Name].(*types.TypeName)
	if obj == nil {
		return nil
	}
	named, _ := obj.Type().(*types.Named)
	if named == nil {
		return nil
	}
	strct, _ := named.Underlying().(*types.Struct)
	if strct == nil {
		return nil
	}
	marker, projMarker := -1, -1
	idx := 0
	for _, f := range d.st.Fields.List {
		n := len(f.Names)
		if n == 0 {
			n = 1
			switch d.markerKind(f.Type, momoModelName) {
			case markerValue:
				if marker < 0 {
					marker = idx
				}
			case markerPointer:
				d.diag(f.Pos(), "momo.Model must be embedded by value in "+d.ts.Name.Name)
				return nil
			}
			switch d.markerKind(f.Type, momoProjectionName) {
			case markerValue:
				if projMarker < 0 {
					projMarker = idx
				}
			case markerPointer:
				d.diag(f.Pos(), "momo.Projection must be embedded by value in "+d.ts.Name.Name)
				return nil
			}
		}
		idx += n
	}
	if marker >= 0 && projMarker >= 0 {
		d.diag(d.ts.Pos(), d.ts.Name.Name+" embeds both momo.Model and momo.Projection")
		return nil
	}
	projection := marker < 0 && projMarker >= 0
	if projection {
		marker = projMarker
	}
	if marker < 0 {
		d.nonModel = named
		return nil
	}
	if named.TypeParams().Len() > 0 {
		what := "a model"
		if projection {
			what = "a projection"
		}
		d.diag(d.ts.Pos(), "generic type "+d.ts.Name.Name+" cannot be "+what)
		return nil
	}
	// Embedding Model twice makes the type checker drop the duplicate field,
	// so the counts differ and this guard skips the type (the compiler
	// diagnostic is already reported); it also prevents index panics.
	if astFieldCount(d.st) != strct.NumFields() {
		return nil // redeclared fields: the type checker already reported it
	}
	return d.build(obj, named, strct, marker, projection)
}

type markerKindT int

const (
	markerNone markerKindT = iota
	markerValue
	markerPointer
)

// markerKind reports whether a field type expression is the momo marker
// type of that name (Model or Projection).
func (d *discovery) markerKind(e ast.Expr, name string) markerKindT {
	t := d.pkg.TypesInfo.TypeOf(e)
	if t == nil {
		return markerNone
	}
	kind := markerValue
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
		kind = markerPointer
	}
	n, ok := t.(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return markerNone
	}
	if n.Obj().Pkg().Path() == momoPkgPath && n.Obj().Name() == name {
		return kind
	}
	return markerNone
}

// checkNonModel reports momo tags on a struct that is not a model.
func (d *discovery) checkNonModel() {
	for _, f := range d.st.Fields.List {
		if f.Tag == nil {
			continue
		}
		if raw, ok := tagValue(f.Tag); ok && raw != "" {
			d.diag(f.Tag.Pos(), "momo tags on non-model type "+d.ts.Name.Name)
			return
		}
	}
}

// tagValue returns the `momo` entry of a tag literal.
func tagValue(lit *ast.BasicLit) (string, bool) {
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return reflect.StructTag(s).Lookup("momo")
}

// parseTags parses the momo tag of one ast.Field, once, however many names
// it declares. label names the field in diagnostics.
func (d *discovery) parseTags(lit *ast.BasicLit, label string) tags.Tags {
	if lit == nil {
		return tags.Tags{}
	}
	raw, ok := tagValue(lit)
	if !ok {
		if _, err := strconv.Unquote(lit.Value); err != nil {
			d.diag(lit.Pos(), label+": malformed struct tag literal")
		}
		return tags.Tags{}
	}
	t, probs := tags.Parse(raw)
	for _, p := range probs {
		d.diag(lit.Pos(), label+": "+p.Msg)
	}
	return t
}

func fieldLabel(f *ast.Field) string {
	if len(f.Names) == 0 {
		return "embedded field"
	}
	var names []string
	for _, n := range f.Names {
		names = append(names, n.Name)
	}
	return "field " + strings.Join(names, ", ")
}

func (d *discovery) build(obj *types.TypeName, named *types.Named, strct *types.Struct, marker int, projection bool) *Model {
	pos := d.pkg.Fset.Position(obj.Pos())
	m := &Model{
		GoName:  obj.Name(),
		PkgPath: d.pkg.PkgPath,
		PkgName: d.pkg.Name,
		PkgDir:  dirOf(pos.Filename),
		File:    pos.Filename,
		Pos:     pos,
		SubPkg:  SubPkgName(obj.Name(), d.pkg.Name),
		named:   named,

		Projection: projection,
	}
	if mod := d.pkg.Module; mod != nil {
		m.ModulePath, m.ModuleDir = mod.Path, mod.Dir
	}
	mt := tags.Tags{}
	idx := 0
	var fields []*Field
	for _, f := range d.st.Fields.List {
		if len(f.Names) == 0 {
			if idx == marker && !projection {
				mt = d.parseTags(f.Tag, "momo.Model marker")
			}
			if idx != marker {
				v := strct.Field(idx)
				et := d.parseTags(f.Tag, fieldLabel(f))
				if !et.Skip {
					fields = append(fields, d.fl.embedField(m, v, et, []int{idx}, "", map[*types.Named]bool{named: true})...)
				}
			}
			idx++
			continue
		}
		ft := d.parseTags(f.Tag, fieldLabel(f))
		for _, name := range f.Names {
			i := idx
			idx++
			if !name.IsExported() {
				if f.Tag != nil {
					if raw, ok := tagValue(f.Tag); ok && raw != "" && raw != "-" {
						d.diag(f.Tag.Pos(), "momo tag on unexported field "+name.Name)
					}
				}
				continue
			}
			if ft.Skip {
				continue
			}
			if isRelationTag(ft) {
				if projection {
					d.diag(f.Pos(), "relation field "+name.Name+" is not allowed on projection "+m.GoName)
					continue
				}
				m.Relations = append(m.Relations, newRelation(strct.Field(i), ft, d.pkg.Fset))
				continue
			}
			if ft.Embedded || ft.Prefix != "" {
				fields = append(fields, d.fl.embedField(m, strct.Field(i), ft, []int{i}, "", map[*types.Named]bool{named: true})...)
				continue
			}
			fields = append(fields, d.fl.newField(strct.Field(i), []int{i}, "", ft))
		}
	}
	d.diags = append(d.diags, d.fl.diags...)
	d.fl.diags = nil
	m.Fields = fields
	if projection {
		return m // no table, indexes or companion
	}
	m.Schema = mt.Schema
	m.Table = mt.Table
	if m.Table == "" {
		m.Table = d.namer.TableName(m.GoName)
	}
	var idiags []Diagnostic
	m.Indexes, idiags = mixinIndexes(m)
	d.diags = append(d.diags, idiags...)
	var cdiags []Diagnostic
	m.Config, cdiags = ReadCompanion(d.pkg, m)
	d.diags = append(d.diags, cdiags...)
	d.companionBad = d.companionBad || len(cdiags) > 0
	return m
}

func astFieldCount(st *ast.StructType) int {
	n := 0
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			n++
		} else {
			n += len(f.Names)
		}
	}
	return n
}
