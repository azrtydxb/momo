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
	"slices"
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
// Go-side value, and unsupported for what cannot be converted.
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

type pending struct {
	model, field string
	tag          map[string]string
	relField     *ast.Field
	relName      string
	elem         string
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

// parseTag splits a gorm tag into upper-cased keys and their values.
func parseTag(f *ast.Field) map[string]string {
	out := map[string]string{}
	if f.Tag == nil {
		return out
	}
	raw, err := strconv.Unquote(f.Tag.Value)
	if err != nil {
		return out
	}
	for _, part := range strings.Split(reflect.StructTag(raw).Get("gorm"), ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, ":")
		out[strings.ToUpper(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
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
			tag := parseTag(f)
			if has(tag, "PRIMARYKEY", "PRIMARY_KEY") {
				si.isMod = true
			}
			for _, n := range f.Names {
				if n.Name == "ID" {
					si.isMod = true
				}
			}
		}
	}
}

func (r *reader) build() ([]importer.Model, *importer.Report, error) {
	r.classify()
	var models []importer.Model
	var pend []pending
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
		pend = append(pend, r.fields(&m, si, "", map[string]bool{name: true})...)
		r.finishModel(&m)
		models = append(models, m)
	}
	r.resolveRelations(models, pend)
	return models, r.rep, nil
}

// fields appends the columns of si to m and returns its associations.
func (r *reader) fields(m *importer.Model, si *structInfo, prefix string, seen map[string]bool) []pending {
	var assoc []pending
	for _, f := range si.st.Fields.List {
		tag := parseTag(f)
		if len(f.Names) == 0 {
			assoc = append(assoc, r.embedded(m, si, f, tag, prefix, seen)...)
			continue
		}
		for _, n := range f.Names {
			if !n.IsExported() || has(tag, "-") {
				continue
			}
			if p, ok := r.association(m, si, f, n.Name, tag); ok {
				assoc = append(assoc, p...)
				continue
			}
			r.column(m, f, n, tag, prefix)
		}
	}
	return assoc
}

func (r *reader) embedded(m *importer.Model, si *structInfo, f *ast.Field, tag map[string]string, prefix string, seen map[string]bool) []pending {
	pos := r.pos(f.Pos())
	if r.isGormSel(si, f.Type, "Model") {
		src := pos
		m.Fields = append(m.Fields,
			importer.Field{Name: "ID", Column: prefix + "id", GoType: "uint", SQLType: "bigint", PK: true, Serial: true, Source: src},
			importer.Field{Name: "CreatedAt", Column: prefix + "created_at", GoType: "time.Time", SQLType: "timestamptz", Nullable: true, Source: src},
			importer.Field{Name: "UpdatedAt", Column: prefix + "updated_at", GoType: "time.Time", SQLType: "timestamptz", Nullable: true, Source: src},
			importer.Field{Name: "DeletedAt", Column: prefix + "deleted_at", GoType: "*time.Time", SQLType: "timestamptz", Nullable: true, Index: true, IndexName: "idx_" + m.Table + "_" + prefix + "deleted_at", Source: src},
		)
		r.rep.Add(importer.Change{Kind: importer.RenamedType, Source: src, Model: m.Name, Field: "DeletedAt", Message: "gorm.DeletedAt becomes *time.Time; soft delete is a momo tag, and GORM's automatic soft-delete filter on queries does not carry over"})
		return nil
	}
	id, ok := f.Type.(*ast.Ident)
	if !ok {
		if st, isPtr := f.Type.(*ast.StarExpr); isPtr {
			id, ok = st.X.(*ast.Ident)
		}
	}
	if ok {
		if emb, local := r.structs[id.Name]; local && !seen[id.Name] {
			seen[id.Name] = true
			p := prefix
			if has(tag, "EMBEDDEDPREFIX") {
				p += tag["EMBEDDEDPREFIX"]
			}
			return r.fields(m, emb, p, seen)
		}
	}
	r.rep.Add(importer.Change{Kind: importer.DroppedField, Source: pos, Model: m.Name, Message: "embedded field " + types.ExprString(f.Type) + " is not a struct in this package and was not imported"})
	return nil
}

// association reports a relation field; it returns the pending relation when
// it may carry a foreign key.
func (r *reader) association(m *importer.Model, si *structInfo, f *ast.Field, name string, tag map[string]string) ([]pending, bool) {
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
	if slice {
		return nil, true
	}
	return []pending{{model: m.Name, field: name, tag: tag, relField: f, relName: name, elem: id.Name, pos: pos}}, true
}

var (
	intTypes    = map[string]string{"int8": "smallint", "int16": "smallint", "uint8": "smallint", "uint16": "smallint", "int": "bigint", "int32": "bigint", "int64": "bigint", "uint": "bigint", "uint32": "bigint", "uint64": "bigint"}
	nullTypes   = map[string]string{"sql.NullString": "text", "sql.NullInt64": "bigint", "sql.NullInt32": "bigint", "sql.NullBool": "boolean", "sql.NullFloat64": "numeric", "sql.NullTime": "timestamptz"}
	identOnly   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	sqlKeywords = map[string]bool{"true": true, "false": true, "null": true, "current_timestamp": true, "current_date": true, "current_time": true, "localtimestamp": true, "localtime": true}
)

func (r *reader) column(m *importer.Model, f *ast.Field, n *ast.Ident, tag map[string]string, prefix string) {
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
	if base == "gorm.DeletedAt" {
		fld.GoType, base = "*time.Time", "time.Time"
		fld.Index = true
		r.rep.Add(importer.Change{Kind: importer.RenamedType, Source: pos, Model: m.Name, Field: n.Name, Message: "gorm.DeletedAt becomes *time.Time; the automatic soft-delete filter does not carry over"})
	}
	size, _ := strconv.Atoi(tag["SIZE"])
	prec, _ := strconv.Atoi(tag["PRECISION"])
	scale, _ := strconv.Atoi(tag["SCALE"])
	fld.PK = has(tag, "PRIMARYKEY", "PRIMARY_KEY")
	if !fld.PK && n.Name == "ID" && prefix == "" && !modelHasExplicitPK(m, tag) {
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
	if q := qualifier(goType); q != "" && q != "time" && q != "sql" && q != "json" && base != "gorm.DeletedAt" {
		unsupported("the type " + goType + " comes from package " + q + ", which the importer cannot place in a model file")
		return
	}
	_, isInt := intTypes[base]
	_, hasDefault := tag["DEFAULT"]
	if fld.PK && isInt && !hasDefault && tag["AUTOINCREMENT"] != "false" {
		fld.Serial = true
	}
	notNull := has(tag, "NOTNULL", "NOT NULL") || fld.PK
	fld.Nullable = !notNull
	r.applyDefault(m, &fld, tag, base, pos)
	r.applyIndexes(&fld, tag, m)
	if has(tag, "AUTOCREATETIME", "AUTOUPDATETIME") {
		r.rep.Add(importer.Change{Kind: importer.GoDefault, Source: pos, Model: m.Name, Field: n.Name, Message: "GORM sets this timestamp in Go on create or update; Momo needs a created_at or updated_at tag or a database default"})
	}
	m.Fields = append(m.Fields, fld)
}

func modelHasExplicitPK(m *importer.Model, _ map[string]string) bool {
	return slices.ContainsFunc(m.Fields, func(f importer.Field) bool { return f.PK })
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

func (r *reader) applyIndexes(fld *importer.Field, tag map[string]string, m *importer.Model) {
	if has(tag, "UNIQUE") && tag["UNIQUE"] != "false" {
		fld.Unique = true // GORM emits a column-level UNIQUE; PostgreSQL names it <table>_<column>_key
	}
	for _, key := range []string{"INDEX", "UNIQUEINDEX"} {
		v, ok := tag[key]
		if !ok {
			continue
		}
		name, opts, _ := strings.Cut(v, ",")
		if name == "" {
			name = "idx_" + m.Table + "_" + fld.Column
		}
		unique := key == "UNIQUEINDEX"
		for _, o := range strings.Split(opts, ",") {
			o = strings.TrimSpace(o)
			switch {
			case o == "":
			case o == "unique":
				unique = true
			default:
				r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: fld.Source, Model: m.Name, Field: fld.Name, Message: "index option " + o + " is not imported"})
			}
		}
		if unique {
			fld.UniqueIndex, fld.Unique = true, true
			fld.UniqueName = name
		} else {
			fld.Index, fld.IndexName = true, name
		}
	}
	if fld.Index && fld.IndexName == "" {
		fld.IndexName = "idx_" + m.Table + "_" + fld.Column
	}
}

// finishModel reports composite indexes: two fields that share an index name.
func (r *reader) finishModel(m *importer.Model) {
	seen := map[string]string{}
	for _, f := range m.Fields {
		for _, name := range []string{ifx(f.Index, f.IndexName), ifx(f.UniqueIndex, f.UniqueName)} {
			if name == "" {
				continue
			}
			if prev, dup := seen[name]; dup && prev != f.Name {
				r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: f.Source, Model: m.Name, Field: f.Name, Message: "index " + name + " spans " + prev + " and " + f.Name + "; composite indexes are not imported, declare them in the companion file"})
			}
			seen[name] = f.Name
		}
	}
}

func ifx(ok bool, s string) string {
	if ok {
		return s
	}
	return ""
}

// resolveRelations turns belongs-to associations into foreign keys.
func (r *reader) resolveRelations(models []importer.Model, pend []pending) {
	byName := map[string]*importer.Model{}
	for i := range models {
		byName[models[i].Name] = &models[i]
	}
	for _, p := range pend {
		m := byName[p.model]
		target := byName[p.elem]
		if m == nil || target == nil {
			continue
		}
		fkName := p.tag["FOREIGNKEY"]
		if fkName == "" {
			fkName = p.relName + "ID"
		}
		var local *importer.Field
		for i := range m.Fields {
			if m.Fields[i].Name == fkName {
				local = &m.Fields[i]
			}
		}
		if local == nil {
			continue // has-one: the key lives on the other model
		}
		var refCols []string
		for _, f := range target.Fields {
			if f.PK {
				refCols = append(refCols, f.Column)
			}
		}
		if len(refCols) != 1 {
			r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: p.pos, Model: m.Name, Field: p.relName, Message: "the referenced model " + target.Name + " needs exactly one primary key column"})
			continue
		}
		fk := importer.ForeignKey{
			Name: "fk_" + m.Table + "_" + namer.ColumnName(p.relName), Field: p.relName, RefModel: target.Name,
			FKField: local.Name, Columns: []string{local.Column}, RefTable: target.Table, RefColumns: refCols,
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
		m.ForeignKeys = append(m.ForeignKeys, fk)
	}
}
