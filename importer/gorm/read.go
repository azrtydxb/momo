// SPDX-License-Identifier: Apache-2.0

// Package gorm reads GORM models from Go source without running it and turns
// them into importer models plus a report of every behaviour that changes
// when the project moves to Momo.
package gorm

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/importer"
)

const gormPath = "gorm.io/gorm"

var namer = naming.GORM{}

// Read parses the Go files of dir (test files are skipped) and returns the
// models and the report. It reads these gorm tag options: column, type, size,
// precision, scale, primaryKey, not null, unique, index, uniqueIndex, default,
// autoIncrement, autoCreateTime, autoUpdateTime, embedded, embeddedPrefix,
// foreignKey and constraint. It recognises gorm.Model embedding and TableName
// methods that return a string literal.
//
// Always reported: implicit-save for every association field (GORM saves
// associations with their parent), zero-value-handling for every non-pointer
// field with a default (GORM replaces a zero value with the default on create,
// and skips zero fields in struct updates), go-default for a default that is a
// Go-side value, and unsupported for what cannot be converted. Constraint and
// unique names Momo cannot carry, and indexes it cannot express (composite
// ones), are reported as naming and dropped-index changes and do not block the
// import.
func Read(dir string) ([]importer.Model, *importer.Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("importer/gorm: %w", err)
	}
	r := &reader{fset: token.NewFileSet(), rep: &importer.Report{}, structs: map[string]*structInfo{}, tables: map[string]string{}}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(r.fset, filepath.Join(dir, n), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, nil, fmt.Errorf("importer/gorm: %w", err)
		}
		r.collect(f)
	}
	return r.build()
}

type structInfo struct {
	name   string
	spec   *ast.TypeSpec
	st     *ast.StructType
	gormID string // local name of the gorm package in the declaring file
	isMod  bool
}

type reader struct {
	fset    *token.FileSet
	rep     *importer.Report
	order   []string
	structs map[string]*structInfo
	tables  map[string]string // type name -> literal from TableName()
}

// relation is an association field that may carry a foreign key.
type relation struct {
	model, field string
	tag          map[string]string
	elem         string
	many         bool
	pos          string
}

func (r *reader) pos(p token.Pos) string {
	ps := r.fset.Position(p)
	return fmt.Sprintf("%s:%d:%d", filepath.Base(ps.Filename), ps.Line, ps.Column)
}

func (r *reader) collect(f *ast.File) {
	alias := ""
	for _, im := range f.Imports {
		if p, err := strconv.Unquote(im.Path.Value); err == nil && p == gormPath {
			alias = "gorm"
			if im.Name != nil {
				alias = im.Name.Name
			}
		}
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			for _, s := range d.Specs {
				ts, ok := s.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				r.structs[ts.Name.Name] = &structInfo{name: ts.Name.Name, spec: ts, st: st, gormID: alias}
				r.order = append(r.order, ts.Name.Name)
			}
		case *ast.FuncDecl:
			if d.Name.Name != "TableName" || d.Recv == nil || len(d.Recv.List) != 1 || d.Body == nil || len(d.Body.List) != 1 {
				continue
			}
			ret, ok := d.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			lit, ok := ret.Results[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			r.tables[recvName(d.Recv.List[0].Type)] = v
		}
	}
}

func recvName(e ast.Expr) string {
	if s, ok := e.(*ast.StarExpr); ok {
		e = s.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// splitTag splits a gorm tag at semicolons; a backslash escapes a semicolon.
func splitTag(s string) []string {
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == ';':
			cur.WriteByte(';')
			i++
		case s[i] == ';':
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(s[i])
		}
	}
	return append(parts, cur.String())
}

// parseTag returns the gorm tag of f: upper-cased keys, the last value of
// each, and every value of each key in order. A tag that cannot be unquoted is
// reported.
func (r *reader) parseTag(f *ast.Field, model string) (map[string]string, map[string][]string) {
	last, all := map[string]string{}, map[string][]string{}
	if f.Tag == nil {
		return last, all
	}
	raw, err := strconv.Unquote(f.Tag.Value)
	if err != nil {
		r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: r.pos(f.Tag.Pos()), Model: model, Message: "the struct tag " + f.Tag.Value + " cannot be read: " + err.Error()})
		return last, all
	}
	for _, part := range splitTag(reflect.StructTag(raw).Get("gorm")) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, ":")
		k = strings.ToUpper(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		last[k] = v
		all[k] = append(all[k], v)
	}
	return last, all
}

func has(tag map[string]string, keys ...string) bool {
	for _, k := range keys {
		if _, ok := tag[k]; ok {
			return true
		}
	}
	return false
}

func (r *reader) isGormSel(si *structInfo, e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && si.gormID != "" && id.Name == si.gormID
}

// classify decides which structs are models: those that embed gorm.Model or
// have a primary key (a primaryKey tag or a field named ID).
func (r *reader) classify() {
	for _, si := range r.structs {
		for _, f := range si.st.Fields.List {
			if len(f.Names) == 0 && r.isGormSel(si, f.Type, "Model") {
				si.isMod = true
			}
			if f.Tag != nil {
				if raw, err := strconv.Unquote(f.Tag.Value); err == nil {
					for _, part := range splitTag(reflect.StructTag(raw).Get("gorm")) {
						k, _, _ := strings.Cut(strings.TrimSpace(part), ":")
						if k = strings.ToUpper(strings.TrimSpace(k)); k == "PRIMARYKEY" || k == "PRIMARY_KEY" {
							si.isMod = true
						}
					}
				}
			}
			for _, n := range f.Names {
				if n.Name == "ID" {
					si.isMod = true
				}
			}
		}
	}
}

// hasExplicitPK reports whether si, or a struct it embeds, declares its key
// with gorm.Model or a primaryKey tag, which then wins over a field named ID.
func (r *reader) hasExplicitPK(si *structInfo, seen map[string]bool) bool {
	for _, f := range si.st.Fields.List {
		if len(f.Names) == 0 && r.isGormSel(si, f.Type, "Model") {
			return true
		}
		if f.Tag != nil {
			if raw, err := strconv.Unquote(f.Tag.Value); err == nil {
				for _, part := range splitTag(reflect.StructTag(raw).Get("gorm")) {
					k, _, _ := strings.Cut(strings.TrimSpace(part), ":")
					if k = strings.ToUpper(strings.TrimSpace(k)); k == "PRIMARYKEY" || k == "PRIMARY_KEY" {
						return true
					}
				}
			}
		}
		if len(f.Names) == 0 {
			if emb := r.localStruct(f.Type); emb != nil && !seen[emb.name] {
				seen[emb.name] = true
				ok := r.hasExplicitPK(emb, seen)
				delete(seen, emb.name)
				if ok {
					return true
				}
			}
		}
	}
	return false
}

// localStruct returns the struct declared in the project that t names, through
// at most one pointer.
func (r *reader) localStruct(t ast.Expr) *structInfo {
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return r.structs[id.Name]
	}
	return nil
}

type modelState struct {
	explicitPK   bool
	explicitAuto map[string]bool // field name -> autoIncrement:true was written
}

func (r *reader) build() ([]importer.Model, *importer.Report, error) {
	r.classify()
	var models []importer.Model
	var rels []relation
	for _, name := range r.order {
		si := r.structs[name]
		if !si.isMod {
			continue
		}
		m := importer.Model{Name: name, Source: r.pos(si.spec.Pos())}
		if t, ok := r.tables[name]; ok {
			m.Table = t
		} else {
			m.Table = namer.TableName(name)
		}
		st := &modelState{explicitPK: r.hasExplicitPK(si, map[string]bool{name: true}), explicitAuto: map[string]bool{}}
		rels = append(rels, r.fields(&m, st, si, "", map[string]bool{name: true})...)
		r.finishModel(&m, st)
		models = append(models, m)
	}
	r.resolveRelations(models, rels)
	return models, r.rep, nil
}

// fields appends the columns of si to m and returns its associations.
func (r *reader) fields(m *importer.Model, st *modelState, si *structInfo, prefix string, stack map[string]bool) []relation {
	var rels []relation
	for _, f := range si.st.Fields.List {
		tag, all := r.parseTag(f, m.Name)
		if len(f.Names) == 0 {
			rels = append(rels, r.embedded(m, st, si, f, tag, prefix, stack)...)
			continue
		}
		for _, n := range f.Names {
			if !n.IsExported() {
				continue
			}
			pos := r.pos(n.Pos())
			if v, skip := tag["-"]; skip {
				msg := "gorm:\"-\" hides the field from GORM entirely; it has no column and is not imported"
				if v == "migration" {
					msg = "gorm:\"-:migration\" keeps the field out of the table; it has no column and is not imported"
				}
				r.rep.Add(importer.Change{Kind: importer.DroppedField, Source: pos, Model: m.Name, Field: n.Name, Message: msg})
				continue
			}
			if has(tag, "SERIALIZER") {
				r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: pos, Model: m.Name, Field: n.Name, Message: "serializer:" + tag["SERIALIZER"] + " converts the value in Go; convert the field by hand with a Momo scanner type"})
				continue
			}
			for _, key := range []string{"->", "<-"} {
				if has(tag, key) {
					r.rep.Add(importer.Change{Kind: importer.DroppedField, Source: pos, Model: m.Name, Field: n.Name, Message: "the permission tag " + key + ":" + tag[key] + " is not mapped; use the read_only or create_only tag options"})
				}
			}
			if has(tag, "EMBEDDED") {
				if emb := r.localStruct(f.Type); emb != nil && !stack[emb.name] {
					stack[emb.name] = true
					rels = append(rels, r.fields(m, st, emb, prefix+tag["EMBEDDEDPREFIX"], stack)...)
					delete(stack, emb.name)
					continue
				}
			}
			if rel, ok := r.association(m, f, n.Name, tag); ok {
				rels = append(rels, rel...)
				continue
			}
			r.column(m, st, si, f, n, tag, all, prefix)
		}
	}
	return rels
}

func (r *reader) embedded(m *importer.Model, st *modelState, si *structInfo, f *ast.Field, tag map[string]string, prefix string, stack map[string]bool) []relation {
	pos := r.pos(f.Pos())
	if r.isGormSel(si, f.Type, "Model") {
		m.Fields = append(m.Fields,
			importer.Field{Name: "ID", Column: prefix + "id", GoType: "uint", SQLType: "bigint", PK: true, Serial: true, Source: pos},
			importer.Field{Name: "CreatedAt", Column: prefix + "created_at", GoType: "time.Time", SQLType: "timestamptz", Nullable: true, Source: pos},
			importer.Field{Name: "UpdatedAt", Column: prefix + "updated_at", GoType: "time.Time", SQLType: "timestamptz", Nullable: true, Source: pos},
			importer.Field{Name: "DeletedAt", Column: prefix + "deleted_at", GoType: "*time.Time", SQLType: "timestamptz", Nullable: true, Index: true, IndexName: "idx_" + m.Table + "_" + prefix + "deleted_at", Source: pos},
		)
		r.rep.Add(importer.Change{Kind: importer.RenamedType, Source: pos, Model: m.Name, Field: "DeletedAt", Message: "gorm.DeletedAt becomes *time.Time; soft delete is a momo tag, and GORM's automatic soft-delete filter on queries does not carry over"})
		return nil
	}
	if emb := r.localStruct(f.Type); emb != nil {
		if stack[emb.name] {
			r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: pos, Model: m.Name, Message: "the struct " + emb.name + " embeds itself, directly or through another embedded struct"})
			return nil
		}
		stack[emb.name] = true
		defer delete(stack, emb.name)
		return r.fields(m, st, emb, prefix+tag["EMBEDDEDPREFIX"], stack)
	}
	r.rep.Add(importer.Change{Kind: importer.DroppedField, Source: pos, Model: m.Name, Message: "embedded field " + types.ExprString(f.Type) + " is not a struct in this package and was not imported"})
	return nil
}

// association reports a relation field; it returns the pending relation.
func (r *reader) association(m *importer.Model, f *ast.Field, name string, tag map[string]string) ([]relation, bool) {
	t := f.Type
	slice := false
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	if a, ok := t.(*ast.ArrayType); ok && a.Len == nil {
		slice = true
		t = a.Elt
		if s, ok := t.(*ast.StarExpr); ok {
			t = s.X
		}
	}
	id, ok := t.(*ast.Ident)
	if !ok {
		return nil, false
	}
	target, local := r.structs[id.Name]
	if !local || !target.isMod {
		return nil, false
	}
	pos := r.pos(f.Pos())
	if has(tag, "MANY2MANY", "POLYMORPHIC") {
		r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: pos, Model: m.Name, Field: name, Message: "many2many and polymorphic associations are not imported"})
		return nil, true
	}
	r.rep.Add(importer.Change{Kind: importer.ImplicitSave, Source: pos, Model: m.Name, Field: name,
		Message: "GORM saves the association " + name + " together with " + m.Name + " on Create and Save; Momo writes only the model you pass, so save related rows explicitly"})
	return []relation{{model: m.Name, field: name, tag: tag, elem: id.Name, many: slice, pos: pos}}, true
}

var (
	intTypes    = map[string]string{"int8": "smallint", "int16": "smallint", "uint8": "smallint", "uint16": "smallint", "int": "bigint", "int32": "bigint", "int64": "bigint", "uint": "bigint", "uint32": "bigint", "uint64": "bigint"}
	nullTypes   = map[string]string{"sql.NullString": "text", "sql.NullInt64": "bigint", "sql.NullInt32": "bigint", "sql.NullBool": "boolean", "sql.NullFloat64": "numeric", "sql.NullTime": "timestamptz"}
	identOnly   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	sqlKeywords = map[string]bool{"true": true, "false": true, "null": true, "current_timestamp": true, "current_date": true, "current_time": true, "localtimestamp": true, "localtime": true}
)

// atoi reads a numeric tag option, reporting a value that is not a number.
func (r *reader) atoi(m *importer.Model, field, pos string, tag map[string]string, key string) int {
	v, ok := tag[key]
	if !ok || v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: pos, Model: m.Name, Field: field, Message: strings.ToLower(key) + ":" + v + " is not a non-negative integer"})
		return 0
	}
	return n
}

func (r *reader) column(m *importer.Model, st *modelState, si *structInfo, f *ast.Field, n *ast.Ident, tag map[string]string, all map[string][]string, prefix string) {
	pos := r.pos(n.Pos())
	goType := types.ExprString(f.Type)
	base := strings.TrimPrefix(goType, "*")
	fld := importer.Field{Name: n.Name, Column: prefix + namer.ColumnName(n.Name), GoType: goType, Source: pos}
	if c := tag["COLUMN"]; c != "" {
		fld.Column = prefix + c
	}
	unsupported := func(msg string) {
		r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: pos, Model: m.Name, Field: n.Name, Message: msg})
	}
	if si.gormID != "" && base == si.gormID+".DeletedAt" {
		fld.GoType, base = "*time.Time", "time.Time"
		fld.Index = true
		r.rep.Add(importer.Change{Kind: importer.RenamedType, Source: pos, Model: m.Name, Field: n.Name, Message: "gorm.DeletedAt becomes *time.Time; the automatic soft-delete filter does not carry over"})
	}
	size := r.atoi(m, n.Name, pos, tag, "SIZE")
	prec := r.atoi(m, n.Name, pos, tag, "PRECISION")
	scale := r.atoi(m, n.Name, pos, tag, "SCALE")
	fld.PK = has(tag, "PRIMARYKEY", "PRIMARY_KEY")
	if !fld.PK && n.Name == "ID" && prefix == "" && !st.explicitPK {
		fld.PK = true
	}
	switch {
	case tag["TYPE"] != "":
		fld.SQLType = tag["TYPE"]
	case base == "bool":
		fld.SQLType = "boolean"
	case intTypes[base] != "":
		fld.SQLType = intTypes[base]
	case base == "float32" || base == "float64":
		fld.SQLType = "numeric"
		if prec > 0 {
			fld.SQLType = fmt.Sprintf("numeric(%d,%d)", prec, scale)
		}
	case base == "string":
		fld.SQLType = "text"
		if size > 0 && size <= 65535 {
			fld.SQLType = fmt.Sprintf("varchar(%d)", size)
		}
	case base == "time.Time":
		fld.SQLType = "timestamptz"
	case base == "[]byte":
		fld.SQLType = "bytea"
	case nullTypes[base] != "":
		fld.SQLType = nullTypes[base]
	default:
		unsupported("the type " + goType + " has no known column type; add a type: tag option or convert the field by hand")
		return
	}
	if q := qualifier(fld.GoType); q != "" && q != "time" && q != "sql" && q != "json" {
		unsupported("the type " + goType + " comes from package " + q + ", which the importer cannot place in a model file")
		return
	}
	_, isInt := intTypes[base]
	_, hasDefault := tag["DEFAULT"]
	auto := tag["AUTOINCREMENT"]
	if isInt && !hasDefault && auto != "false" && (fld.PK || auto == "true") {
		fld.Serial = true
		st.explicitAuto[n.Name] = auto == "true"
	}
	fld.Nullable = !(has(tag, "NOTNULL", "NOT NULL") || fld.PK)
	r.applyDefault(m, &fld, tag, base, pos)
	r.applyIndexes(&fld, tag, all, m)
	if has(tag, "AUTOCREATETIME", "AUTOUPDATETIME") {
		r.rep.Add(importer.Change{Kind: importer.GoDefault, Source: pos, Model: m.Name, Field: n.Name, Message: "GORM sets this timestamp in Go on create or update; Momo needs a created_at or updated_at tag or a database default"})
	}
	m.Fields = append(m.Fields, fld)
}

func qualifier(goType string) string {
	t := strings.TrimLeft(goType, "*[]")
	if i := strings.Index(t, "."); i > 0 {
		return t[:i]
	}
	return ""
}

func (r *reader) applyDefault(m *importer.Model, fld *importer.Field, tag map[string]string, base, pos string) {
	v, ok := tag["DEFAULT"]
	if !ok {
		return
	}
	if !strings.HasPrefix(fld.GoType, "*") {
		r.rep.Add(importer.Change{Kind: importer.ZeroValueHandling, Source: pos, Model: m.Name, Field: fld.Name,
			Message: "GORM treats the zero value of " + fld.Name + " as unset and applies the default " + v + " on Create, and skips it in struct Updates; Momo writes the zero value, so use a pointer type to leave it unset"})
	}
	lower := strings.ToLower(v)
	skipParse := strings.Contains(v, "(") && strings.Contains(v, ")") || lower == "null" || v == ""
	switch {
	case !skipParse && base == "string":
		trimmed := strings.Trim(strings.Trim(v, "'"), `"`)
		fld.Default = "'" + strings.ReplaceAll(trimmed, "'", "''") + "'"
		fld.DefaultKind = "db"
	case identOnly.MatchString(v) && !sqlKeywords[lower]:
		fld.DefaultKind = "go"
		r.rep.Add(importer.Change{Kind: importer.GoDefault, Source: pos, Model: m.Name, Field: fld.Name, Message: "the default " + v + " is not a SQL literal; GORM applies it in Go and Momo does not"})
	default:
		fld.Default = v
		fld.DefaultKind = "db"
	}
}

// applyIndexes reads unique, index and uniqueIndex. A field can carry several
// index tags; Momo's tags name one index per field, so the first is kept and
// the others are reported as dropped.
func (r *reader) applyIndexes(fld *importer.Field, tag map[string]string, all map[string][]string, m *importer.Model) {
	if has(tag, "UNIQUE") && tag["UNIQUE"] != "false" {
		fld.Unique = true // a UNIQUE constraint, which GORM names uni_<table>_<column>
		fld.UniqueName = "uni_" + m.Table + "_" + fld.Column
	}
	kept := false
	for _, key := range []string{"INDEX", "UNIQUEINDEX"} {
		for _, v := range all[key] {
			name, opts, _ := strings.Cut(v, ",")
			if name == "" {
				name = "idx_" + m.Table + "_" + fld.Column
			}
			unique := key == "UNIQUEINDEX"
			for _, o := range strings.Split(opts, ",") {
				switch o = strings.TrimSpace(o); {
				case o == "":
				case o == "unique":
					unique = true
				case strings.HasPrefix(o, "priority:") || strings.HasPrefix(o, "sort:") || strings.HasPrefix(o, "length:") || o == "composite" || strings.HasPrefix(o, "composite:"):
					// ordering inside a composite index; composites are reported in finishModel
				default:
					r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: fld.Source, Model: m.Name, Field: fld.Name, Message: "index option " + o + " is not imported"})
				}
			}
			if kept {
				r.rep.Add(importer.Change{Kind: importer.DroppedIndex, Source: fld.Source, Model: m.Name, Field: fld.Name, Message: "a field carries several index tags; only the first is kept, index " + name + " is dropped"})
				continue
			}
			kept = true
			if unique {
				fld.UniqueIndex, fld.Unique = true, true
				fld.UniqueName = name
			} else {
				fld.Index, fld.IndexName = true, name
			}
		}
	}
	if fld.Index && fld.IndexName == "" {
		fld.IndexName = "idx_" + m.Table + "_" + fld.Column
	}
}

// finishModel settles the key columns and finds composite indexes.
func (r *reader) finishModel(m *importer.Model, st *modelState) {
	pks := 0
	for _, f := range m.Fields {
		if f.PK {
			pks++
		}
	}
	for i := range m.Fields {
		f := &m.Fields[i]
		if pks > 1 && f.Serial && !st.explicitAuto[f.Name] {
			f.Serial = false // a composite key does not auto-increment unless asked to
		}
	}
	// Two fields that share an index name make one composite index. Momo's
	// tags cannot express it, and two tags with one name would collide, so the
	// index is dropped from every field and reported.
	users := map[string][]int{}
	for i, f := range m.Fields {
		for _, name := range []string{ifx(f.Index, f.IndexName), ifx(f.UniqueIndex, f.UniqueName)} {
			if name != "" {
				users[name] = append(users[name], i)
			}
		}
	}
	for name, idx := range users {
		if len(idx) < 2 {
			continue
		}
		var names []string
		for _, i := range idx {
			f := &m.Fields[i]
			names = append(names, f.Name)
			if f.IndexName == name {
				f.Index, f.IndexName = false, ""
			}
			if f.UniqueName == name {
				f.UniqueIndex, f.UniqueName = false, ""
				f.Unique = false
			}
		}
		r.rep.Add(importer.Change{Kind: importer.DroppedIndex, Source: m.Fields[idx[0]].Source, Model: m.Name, Field: m.Fields[idx[0]].Name,
			Message: "index " + name + " spans " + strings.Join(names, ", ") + "; composite indexes are not imported, declare it in the companion file"})
	}
}

func ifx(ok bool, s string) string {
	if ok {
		return s
	}
	return ""
}

// resolveRelations turns associations into foreign keys. A belongs-to field
// carries the key on its own model; a has-one or has-many field carries it on
// the child, where the key becomes a belongs-to relation field named after the
// parent so the constraint survives.
func (r *reader) resolveRelations(models []importer.Model, rels []relation) {
	byName := map[string]*importer.Model{}
	for i := range models {
		byName[models[i].Name] = &models[i]
	}
	for _, p := range rels {
		owner, other := byName[p.model], byName[p.elem]
		if owner == nil || other == nil {
			continue
		}
		fkName := p.tag["FOREIGNKEY"]
		switch {
		case !p.many && findField(owner, fkOrDefault(fkName, p.field)) != nil:
			r.addFK(owner, other, owner.Name+"."+p.field, p, fkOrDefault(fkName, p.field), p.field, false)
		default:
			// has-one / has-many: the key is a field of the child
			child, parent := other, owner
			if fkName == "" {
				fkName = parent.Name + "ID"
			}
			if findField(child, fkName) == nil {
				r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: p.pos, Model: owner.Name, Field: p.field,
					Message: "the foreign key field " + fkName + " is not a field of " + child.Name + "; GORM's constraint for this association is not imported"})
				continue
			}
			r.addFK(child, parent, owner.Name+"."+p.field, p, fkName, p.field, true)
		}
	}
}

func fkOrDefault(tag, rel string) string {
	if tag != "" {
		return tag
	}
	return rel + "ID"
}

func findField(m *importer.Model, name string) *importer.Field {
	for i := range m.Fields {
		if m.Fields[i].Name == name {
			return &m.Fields[i]
		}
	}
	return nil
}

// addFK adds the key held by fkField of from, referencing to.
func (r *reader) addFK(from, to *importer.Model, who string, p relation, fkField, relName string, onChild bool) {
	local := findField(from, fkField)
	var refCols []string
	for _, f := range to.Fields {
		if f.PK {
			refCols = append(refCols, f.Column)
		}
	}
	if len(refCols) != 1 {
		r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: p.pos, Model: p.model, Field: p.field, Message: "the referenced model " + to.Name + " needs exactly one primary key column"})
		return
	}
	// GORM names the constraint after the model that declares the association.
	declTable := from.Table
	field := relName
	if onChild {
		declTable = to.Table
		field = to.Name // the relation field synthesized on the child
		if findField(from, field) != nil || hasRelation(from, field) {
			r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: p.pos, Model: p.model, Field: p.field,
				Message: "the constraint for " + who + " needs a relation field named " + field + " on " + from.Name + ", which already exists; GORM's constraint is dropped"})
			return
		}
		r.rep.Add(importer.Change{Kind: importer.Naming, Source: p.pos, Model: from.Name, Field: field,
			Message: "GORM's constraint for " + who + " lives on " + from.Table + "; the import adds a belongs-to field " + field + " to " + from.Name + " to keep it"})
	}
	if hasRelation(from, field) {
		return // the same key reached through both ends of the association
	}
	fk := importer.ForeignKey{
		Name: "fk_" + declTable + "_" + namer.ColumnName(p.field), Field: field, RefModel: to.Name,
		FKField: local.Name, Columns: []string{local.Column}, RefTable: to.Table, RefColumns: refCols,
	}
	for _, part := range strings.Split(p.tag["CONSTRAINT"], ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), ":")
		switch strings.ToLower(k) {
		case "ondelete":
			fk.OnDelete = strings.ToUpper(v)
		case "onupdate":
			fk.OnUpdate = strings.ToUpper(v)
		}
	}
	r.rep.Add(importer.Change{Kind: importer.Naming, Source: p.pos, Model: from.Name, Field: field,
		Message: "foreign key " + fk.Name + " is named " + from.Table + "_" + local.Column + "_fkey by Momo"})
	from.ForeignKeys = append(from.ForeignKeys, fk)
}

func hasRelation(m *importer.Model, field string) bool {
	for _, fk := range m.ForeignKeys {
		if fk.Field == field {
			return true
		}
	}
	return false
}
