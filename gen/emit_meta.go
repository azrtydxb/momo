// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"go/format"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/azrtydxb/momo/schema"
)

// EmitMeta returns the source of momo_gen.go for the models of one package:
// per model a MomoMeta method, the package-level momo.Meta value with its
// scan and bind functions, and the literal schema.Table. models must belong
// to a single package; the result is gofmt-formatted and starts with
// Header().
//
// Structs that embed momo.Projection in the same package (g.Projections) get
// a MomoMeta with no table and every derived index set to -1.
func EmitMeta(models []*Model, g *Graph) ([]byte, []Diagnostic) {
	if len(models) == 0 {
		return nil, nil
	}
	var projs []*Model
	if g != nil {
		for _, p := range g.Projections {
			if p.PkgPath == models[0].PkgPath && p.PkgDir == models[0].PkgDir {
				projs = append(projs, p)
			}
		}
	}
	return emitPackageMeta(models, projs, g)
}

// emitPackageMeta is EmitMeta for the models and projections of one package.
func emitPackageMeta(models, projections []*Model, g *Graph) ([]byte, []Diagnostic) {
	if len(models)+len(projections) == 0 {
		return nil, nil
	}
	first := append(append([]*Model(nil), models...), projections...)[0]
	pkgName, pkgPath := first.PkgName, first.PkgPath
	sorted := append([]*Model(nil), models...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].GoName < sorted[j].GoName })
	sortedP := append([]*Model(nil), projections...)
	sort.SliceStable(sortedP, func(i, j int) bool { return sortedP[i].GoName < sortedP[j].GoName })

	var diags []Diagnostic
	var body strings.Builder
	used := map[string]string{}
	for _, m := range sorted {
		if m.PkgPath != pkgPath {
			diags = append(diags, Diagnostic{Pos: m.Pos, Msg: "model " + m.GoName + " is not in package " + pkgPath})
			continue
		}
		t := tableOf(g, m)
		if t == nil {
			diags = append(diags, Diagnostic{Pos: m.Pos, Msg: "no schema table for model " + m.GoName})
			continue
		}
		lower := lowerFirst(m.GoName)
		for _, v := range []string{lower + "Meta", lower + "Table"} {
			if prev, ok := used[v]; ok {
				diags = append(diags, Diagnostic{Pos: m.Pos, Msg: fmt.Sprintf("generated name %s of %s collides with %s", v, m.GoName, prev)})
			}
			used[v] = m.GoName
		}
		_, gd := idGensOf(m)
		diags = append(diags, gd...)
		emitModelMeta(&body, m, t, lower)
	}
	for _, m := range sortedP {
		if m.PkgPath != pkgPath {
			diags = append(diags, Diagnostic{Pos: m.Pos, Msg: "projection " + m.GoName + " is not in package " + pkgPath})
			continue
		}
		lower := lowerFirst(m.GoName)
		if prev, ok := used[lower+"Meta"]; ok {
			diags = append(diags, Diagnostic{Pos: m.Pos, Msg: fmt.Sprintf("generated name %s of %s collides with %s", lower+"Meta", m.GoName, prev)})
		}
		used[lower+"Meta"] = m.GoName
		emitProjectionMeta(&body, m, lower)
	}
	if len(diags) > 0 {
		return nil, diags
	}

	var out strings.Builder
	out.WriteString(Header() + "\n\n")
	fmt.Fprintf(&out, "package %s\n\n", pkgName)
	if len(sorted) > 0 {
		out.WriteString("import (\n")
		if anyHooks(sorted) {
			out.WriteString("\t\"context\"\n\n")
		}
		out.WriteString("\t\"github.com/azrtydxb/momo\"\n\t\"github.com/azrtydxb/momo/schema\"\n)\n")
	} else {
		out.WriteString("import \"github.com/azrtydxb/momo\"\n")
	}
	out.WriteString(body.String())
	src, err := format.Source([]byte(out.String()))
	if err != nil {
		return nil, []Diagnostic{{Pos: first.Pos, Msg: "generator bug: formatting meta for " + pkgName + ": " + err.Error()}}
	}
	return src, nil
}

// tableOf finds the schema table of a model in the graph.
func tableOf(g *Graph, m *Model) *schema.Table {
	if g == nil || g.Schema == nil {
		return nil
	}
	for i := range g.Schema.Tables {
		t := &g.Schema.Tables[i]
		if t.Name == m.Table && t.Schema == m.Schema {
			return t
		}
	}
	return nil
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

func emitModelMeta(b *strings.Builder, m *Model, t *schema.Table, lower string) {
	w := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	w("\nfunc (*%s) MomoMeta() *momo.Meta[%s] { return &%sMeta }\n\n", m.GoName, m.GoName, lower)
	w("var %sMeta = momo.Meta[%s]{\n", lower, m.GoName)
	w("Table: %s,\n", strconv.Quote(m.Table))
	if m.Schema != "" {
		w("Schema: %s,\n", strconv.Quote(m.Schema))
	}
	w("Columns: []momo.ColumnMeta{\n")
	for _, f := range m.Fields {
		if fl := flagExpr(f); fl != "0" {
			w("{Name: %s, Flags: %s},\n", strconv.Quote(f.Column), fl)
		} else {
			w("{Name: %s},\n", strconv.Quote(f.Column))
		}
	}
	w("},\n")
	sels := make([]string, len(m.Fields))
	for i, f := range m.Fields {
		sels[i], _ = fieldVar(m, f)
	}
	w("Ptr: func(m *%s, i int) any {\n", m.GoName)
	writeSwitch(b, sels, "&m.")
	w("},\n")
	w("Value: func(m *%s, i int) any {\n", m.GoName)
	writeSwitch(b, sels, "m.")
	w("},\n")
	w("TableSchema: &%sTable,\n", lower)
	w("Identity: %d, Version: %d, SoftDelete: %d, CreatedAt: %d, UpdatedAt: %d,\n",
		indexOf(m, func(f *Field) bool { return f.Tags.Identity || f.Tags.Serial }),
		indexOf(m, func(f *Field) bool { return f.Tags.Version }),
		indexOf(m, func(f *Field) bool { return f.Tags.SoftDelete != "" }),
		indexOf(m, func(f *Field) bool { return f.Tags.CreatedAt }),
		indexOf(m, func(f *Field) bool { return f.Tags.UpdatedAt }))
	// The timestamp kind is the zero value, so it is omitted.
	if i := indexOf(m, func(f *Field) bool { return f.Tags.SoftDelete != "" }); i >= 0 {
		switch m.Fields[i].Tags.SoftDelete {
		case "unix":
			w("SoftDeleteKind: momo.SoftDeleteUnix,\n")
		case "bool":
			w("SoftDeleteKind: momo.SoftDeleteBool,\n")
		}
	}
	var pk []string
	for i, f := range m.Fields {
		if f.Tags.PK {
			pk = append(pk, strconv.Itoa(i))
		}
	}
	w("PK: []int{%s},\n", strings.Join(pk, ", "))
	if gens, _ := idGensOf(m); len(gens) > 0 {
		w("IDGens: []momo.IDGen{")
		for i, g := range gens {
			if i > 0 {
				w(", ")
			}
			w("{Column: %d, Name: %s}", g.Column, strconv.Quote(g.Name))
		}
		w("},\n")
	}
	if m.IsView {
		w("View: true,\n")
	}
	w("%s", hooksLiteral(m.GoName, m.Hooks))
	w("}\n\n")
	w("var %sTable = %s\n", lower, literal(reflect.ValueOf(*t)))
}

// emitProjectionMeta writes the MomoMeta of a projection: the columns in
// field order, scan and bind functions, and -1 for every derived index.
func emitProjectionMeta(b *strings.Builder, m *Model, lower string) {
	w := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	w("\nfunc (*%s) MomoMeta() *momo.Meta[%s] { return &%sMeta }\n\n", m.GoName, m.GoName, lower)
	w("var %sMeta = momo.Meta[%s]{\n", lower, m.GoName)
	w("Columns: []momo.ColumnMeta{\n")
	for _, f := range m.Fields {
		w("{Name: %s},\n", strconv.Quote(f.Column))
	}
	w("},\n")
	sels := make([]string, len(m.Fields))
	for i, f := range m.Fields {
		sels[i], _ = fieldVar(m, f)
	}
	w("Ptr: func(m *%s, i int) any {\n", m.GoName)
	writeSwitch(b, sels, "&m.")
	w("},\n")
	w("Value: func(m *%s, i int) any {\n", m.GoName)
	writeSwitch(b, sels, "m.")
	w("},\n")
	w("Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,\n")
	w("}\n")
}

// writeSwitch writes the body of a Ptr or Value function: one case per
// column and nil past the end.
func writeSwitch(b *strings.Builder, sels []string, prefix string) {
	if len(sels) == 0 {
		b.WriteString("return nil\n")
		return
	}
	b.WriteString("switch i {\n")
	for i, s := range sels {
		fmt.Fprintf(b, "case %d:\nreturn %s%s\n", i, prefix, s)
	}
	b.WriteString("}\nreturn nil\n")
}

func indexOf(m *Model, on func(*Field) bool) int {
	for i, f := range m.Fields {
		if on(f) {
			return i
		}
	}
	return -1
}

// literal renders v as a Go composite literal. Zero fields are omitted, so
// the output stays close to what a person would write. Only the kinds a
// schema.Table is made of are supported.
func literal(v reflect.Value) string { return lit(v, false) }

// lit renders v; elide drops the type of a struct element of a slice.
func lit(v reflect.Value, elide bool) string {
	switch v.Kind() {
	case reflect.String:
		return strconv.Quote(v.String())
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Pointer:
		return "&" + lit(v.Elem(), elide)
	case reflect.Slice:
		var sb strings.Builder
		sb.WriteString(v.Type().String() + "{")
		simple := v.Type().Elem().Kind() == reflect.String
		for i := 0; i < v.Len(); i++ {
			if simple {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(lit(v.Index(i), true))
				continue
			}
			sb.WriteString("\n" + lit(v.Index(i), true) + ",")
		}
		if !simple && v.Len() > 0 {
			sb.WriteString("\n")
		}
		return sb.String() + "}"
	case reflect.Struct:
		var sb strings.Builder
		if !elide {
			sb.WriteString(v.Type().String())
		}
		sb.WriteString("{\n")
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).IsZero() {
				continue
			}
			sb.WriteString(v.Type().Field(i).Name + ": " + lit(v.Field(i), false) + ",\n")
		}
		return sb.String() + "}"
	}
	panic("gen: unsupported kind in schema literal: " + v.Kind().String())
}
