// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"go/token"
	"go/types"
	"reflect"

	"golang.org/x/tools/go/packages"

	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/schema"
)

// flattener turns embedded structs, prefixed structs and mixins into the
// flat field list of the containing model. It works from type information
// only, so structs from other packages flatten the same way as local ones.
type flattener struct {
	fset  *token.FileSet
	namer naming.Namer
	diags []Diagnostic
	// used records the named struct types flattened into some model; such a
	// type is a mixin, not a stray non-model carrying momo tags.
	used map[*types.Named]bool
}

func newFlattener(pkg *packages.Package, namer naming.Namer) *flattener {
	return &flattener{fset: pkg.Fset, namer: namer, used: map[*types.Named]bool{}}
}

func (fl *flattener) diag(pos token.Pos, msg string) {
	fl.diags = append(fl.diags, Diagnostic{Pos: fl.fset.Position(pos), Msg: msg})
}

// flatten is the plan's Flatten: it returns the persisted fields of st in
// declaration order. path is the index path of st within the model, and
// prefix is prepended to every column name. A struct that contains itself
// by value cannot compile, but seen still guards the recursion so that a
// future pointer-aware change cannot loop.
func (fl *flattener) flatten(m *Model, st *types.Struct, path []int, prefix string, seen map[*types.Named]bool) []*Field {
	var out []*Field
	for i := 0; i < st.NumFields(); i++ {
		v := st.Field(i)
		ft := fl.parseTag(v, st.Tag(i))
		if ft.Skip {
			continue
		}
		p := appendPath(path, i)
		if v.Anonymous() || ft.Embedded || ft.Prefix != "" {
			out = append(out, fl.embedField(m, v, ft, p, prefix, seen)...)
			continue
		}
		if !v.Exported() {
			if raw, ok := reflect.StructTag(st.Tag(i)).Lookup("momo"); ok && raw != "" && raw != "-" {
				fl.diag(v.Pos(), "momo tag on unexported field "+v.Name())
			}
			continue
		}
		if ft.BelongsTo || ft.HasOne || ft.HasMany || ft.ManyToMany || ft.Rel != "" {
			continue // a relation, handled in Task 13
		}
		out = append(out, fl.newField(v, p, prefix, ft))
	}
	return out
}

// embedField flattens one embedded or `embedded`-tagged field v whose index
// path is p. It returns nil, with a diagnostic where the declaration is
// wrong, when nothing can be flattened.
func (fl *flattener) embedField(m *Model, v *types.Var, ft tags.Tags, p []int, prefix string, seen map[*types.Named]bool) []*Field {
	t := types.Unalias(v.Type())
	ptr := false
	if pt, ok := t.(*types.Pointer); ok {
		ptr = true
		t = types.Unalias(pt.Elem())
	}
	named, _ := t.(*types.Named)
	if named != nil && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == momoPkgPath && named.Obj().Name() == momoModelName {
		fl.diag(v.Pos(), "momo.Model embedded inside embedded struct "+v.Name()+"; embed it in the model itself")
		return nil
	}
	if ptr {
		fl.diag(v.Pos(), "embedded pointer field "+v.Name()+" is not supported; embed the struct by value")
		return nil
	}
	st, _ := t.Underlying().(*types.Struct)
	if st == nil {
		if ft.Embedded {
			fl.diag(v.Pos(), "field "+v.Name()+" is tagged embedded but is not a struct")
		}
		return nil // an embedded non-struct type is not flattened
	}
	if ft.Prefix != "" && !ft.Embedded {
		fl.diag(v.Pos(), "prefix on field "+v.Name()+" requires the embedded option")
		return nil
	}
	if named != nil {
		if seen[named] {
			fl.diag(v.Pos(), "embedded cycle through "+named.Obj().Name())
			return nil
		}
		seen[named] = true
		defer delete(seen, named)
		fl.used[named] = true
	}
	return fl.flatten(m, st, p, prefix+ft.Prefix, seen)
}

// parseTag parses the momo tag of a type-level field, positioning problems
// at the field.
func (fl *flattener) parseTag(v *types.Var, tag string) tags.Tags {
	raw, ok := reflect.StructTag(tag).Lookup("momo")
	if !ok {
		return tags.Tags{}
	}
	t, probs := tags.Parse(raw)
	for _, pr := range probs {
		fl.diag(v.Pos(), "field "+v.Name()+": "+pr.Msg)
	}
	return t
}

// newField builds the Field for v. prefix is the accumulated embedded prefix
// and is prepended to the column name; the Go name is unchanged.
func (fl *flattener) newField(v *types.Var, path []int, prefix string, ft tags.Tags) *Field {
	col := ft.Column
	if col == "" {
		col = fl.namer.ColumnName(v.Name())
	}
	t := v.Type()
	_, nullable := t.Underlying().(*types.Pointer)
	access := Writable
	switch {
	case ft.ReadOnly:
		access = ReadOnly
	case ft.CreateOnly:
		access = CreateOnly
	case ft.Immutable:
		access = Immutable
	}
	return &Field{
		GoName:   v.Name(),
		Path:     path,
		Column:   prefix + col,
		GoType:   types.TypeString(t, func(p *types.Package) string { return p.Name() }),
		Nullable: nullable,
		Tags:     ft,
		Pos:      fl.fset.Position(v.Pos()),
		Access:   access,
		Base:     prefix,
	}
}

func appendPath(path []int, i int) []int {
	out := make([]int, len(path), len(path)+1)
	copy(out, path)
	return append(out, i)
}

// mixinIndexes returns the indexes contributed by the `index` and `unique`
// tags of flattened fields (those whose path is deeper than one). Names that
// are not explicit are <table>_<column>_idx.
func mixinIndexes(m *Model) []schema.Index {
	var out []schema.Index
	for _, f := range m.Fields {
		if len(f.Path) < 2 || (!f.Tags.Index && !f.Tags.Unique) {
			continue
		}
		name := m.Table + "_" + f.Column + "_idx"
		unique := f.Tags.Unique
		switch {
		case unique && f.Tags.UniqueName != "":
			name = f.Tags.UniqueName
		case !unique && f.Tags.IndexName != "":
			name = f.Tags.IndexName
		}
		out = append(out, schema.Index{
			Name:   name,
			Unique: unique,
			Parts:  []schema.IndexPart{{Column: f.Column}},
		})
	}
	return out
}
