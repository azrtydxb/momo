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
)

// discoverModels finds the models of one loaded package. It reports
// positioned diagnostics and never guesses: a type is a model only when it is
// a non-generic, non-alias struct that embeds momo.Model by value.
func discoverModels(pkg *packages.Package, namer naming.Namer) ([]*Model, []Diagnostic) {
	if pkg.TypesInfo == nil || pkg.Types == nil {
		return nil, nil
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
	var models []*Model
	var diags []Diagnostic
	for _, d := range all {
		if d.nonModel != nil && !fl.used[d.nonModel] {
			d.checkNonModel()
		}
		diags = append(diags, d.diags...)
		if d.model != nil {
			models = append(models, d.model)
		}
	}
	return models, dedupeDiagnostics(diags)
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
	marker := -1
	idx := 0
	for _, f := range d.st.Fields.List {
		n := len(f.Names)
		if n == 0 {
			n = 1
			switch d.markerKind(f.Type) {
			case markerValue:
				if marker < 0 {
					marker = idx
				}
			case markerPointer:
				d.diag(f.Pos(), "momo.Model must be embedded by value in "+d.ts.Name.Name)
				return nil
			}
		}
		idx += n
	}
	if marker < 0 {
		d.nonModel = named
		return nil
	}
	if named.TypeParams().Len() > 0 {
		d.diag(d.ts.Pos(), "generic type "+d.ts.Name.Name+" cannot be a model")
		return nil
	}
	// Embedding Model twice makes the type checker drop the duplicate field,
	// so the counts differ and this guard skips the type (the compiler
	// diagnostic is already reported); it also prevents index panics.
	if astFieldCount(d.st) != strct.NumFields() {
		return nil // redeclared fields: the type checker already reported it
	}
	return d.build(obj, named, strct, marker)
}

type markerKindT int

const (
	markerNone markerKindT = iota
	markerValue
	markerPointer
)

// markerKind reports whether a field type expression is momo.Model.
func (d *discovery) markerKind(e ast.Expr) markerKindT {
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
	if n.Obj().Pkg().Path() == momoPkgPath && n.Obj().Name() == momoModelName {
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

func (d *discovery) build(obj *types.TypeName, named *types.Named, strct *types.Struct, marker int) *Model {
	pos := d.pkg.Fset.Position(obj.Pos())
	m := &Model{
		GoName:  obj.Name(),
		PkgPath: d.pkg.PkgPath,
		PkgName: d.pkg.Name,
		PkgDir:  dirOf(pos.Filename),
		File:    pos.Filename,
		Pos:     pos,
	}
	mt := tags.Tags{}
	idx := 0
	var fields []*Field
	for _, f := range d.st.Fields.List {
		if len(f.Names) == 0 {
			if idx == marker {
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
			if ft.Skip || ft.BelongsTo || ft.HasOne || ft.HasMany || ft.ManyToMany || ft.Rel != "" {
				continue // skipped, or a relation handled in Task 13
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
	m.Schema = mt.Schema
	m.Table = mt.Table
	if m.Table == "" {
		m.Table = d.namer.TableName(m.GoName)
	}
	var idiags []Diagnostic
	m.Indexes, idiags = mixinIndexes(m)
	d.diags = append(d.diags, idiags...)
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
