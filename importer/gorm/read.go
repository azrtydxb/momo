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
	"sort"
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
// Limit: a NamingStrategy is only detected as a composite literal key in the
// Go files of dir. One configured in another package, or assigned after the
// fact, is not seen; the importer assumes GORM's default naming then.
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
			if d.Name.Name != "TableName" || d.Recv == nil || len(d.Recv.List) != 1 {
				continue
			}
			recv := recvName(d.Recv.List[0].Type)
			if v, ok := literalReturn(d); ok {
				r.tables[recv] = v
				continue
			}
			r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: r.pos(d.Pos()), Model: recv,
				Message: "TableName does not return a string literal, so the table name cannot be read; the importer would guess the default plural name"})
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "NamingStrategy" {
			r.rep.Add(importer.Change{Kind: importer.Naming, Source: r.pos(kv.Pos()),
				Message: "a gorm NamingStrategy is configured here (for example SingularTable or TablePrefix); the importer assumes GORM's default naming, so check the table and column names of the imported models. Only literals in the files of the read directory are detected: a NamingStrategy set in another package is not"})
		}
		return true
	})
}

// literalReturn returns the string literal d returns, when its body is a
// single return of one.
func literalReturn(d *ast.FuncDecl) (string, bool) {
	if d.Body == nil || len(d.Body.List) != 1 {
		return "", false
	}
	ret, ok := d.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return "", false
	}
	lit, ok := ret.Results[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	return v, err == nil
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
		if !knownKeys[k] {
			r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: r.pos(f.Pos()), Model: model, Field: fieldNames(f),
				Message: "the gorm tag option " + strings.ToLower(k) + " is not imported (check, comment, references and the like); declare it in Momo's companion file or convert the field by hand"})
			continue
		}
		last[k] = v
		all[k] = append(all[k], v)
	}
	return last, all
}

// knownKeys are the gorm tag options the reader handles or deliberately
// reports; any other option is reported as unsupported.
var knownKeys = map[string]bool{
	"COLUMN": true, "TYPE": true, "SIZE": true, "PRECISION": true, "SCALE": true,
	"PRIMARYKEY": true, "PRIMARY_KEY": true, "NOT NULL": true, "NOTNULL": true,
	"UNIQUE": true, "INDEX": true, "UNIQUEINDEX": true, "DEFAULT": true,
	"AUTOINCREMENT": true, "AUTOCREATETIME": true, "AUTOUPDATETIME": true,
	"EMBEDDED": true, "EMBEDDEDPREFIX": true, "FOREIGNKEY": true, "CONSTRAINT": true,
	"MANY2MANY": true, "POLYMORPHIC": true, "SERIALIZER": true,
	"-": true, "->": true, "<-": true,
}

func fieldNames(f *ast.Field) string {
	var names []string
	for _, n := range f.Names {
		names = append(names, n.Name)
	}
	return strings.Join(names, ", ")
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
				if n.Name == "ID" && isKeyType(f.Type) {
					si.isMod = true
				}
			}
		}
	}
}

// isKeyType reports whether t can be a key: a named scalar or a package type
// such as uuid.UUID, not a pointer, slice, map or inline struct.
func isKeyType(t ast.Expr) bool {
	switch t.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		return true
	}
	return false
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
		p := prefix + tag["EMBEDDEDPREFIX"]
		m.Fields = append(m.Fields,
			importer.Field{Name: "ID", Column: p + "id", GoType: "uint", SQLType: "bigint", PK: true, Serial: true, Source: pos},
			importer.Field{Name: "CreatedAt", Column: p + "created_at", GoType: "time.Time", SQLType: "timestamptz", Nullable: true, Source: pos},
			importer.Field{Name: "UpdatedAt", Column: p + "updated_at", GoType: "time.Time", SQLType: "timestamptz", Nullable: true, Source: pos},
			importer.Field{Name: "DeletedAt", Column: p + "deleted_at", GoType: "*time.Time", SQLType: "timestamptz", Nullable: true, Index: true, IndexName: "idx_" + m.Table + "_" + p + "deleted_at", Source: pos},
		)
		for _, name := range []string{"CreatedAt", "UpdatedAt"} {
			r.rep.Add(importer.Change{Kind: importer.GoDefault, Source: pos, Model: m.Name, Field: name, Message: "GORM sets " + name + " (from gorm.Model) in Go on create or update; Momo needs a created_at or updated_at tag or a database default"})
		}
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
	// intBits is the size gorm gives an integer field; an unsigned field is
	// one bit wider in gorm.io/driver/postgres DataTypeOf (size++ for Uint), so
	// uint16 is an integer and uint8 a smallint. A sql.Null* type takes the
	// type of its first field (NullInt32 holds an int32).
	intBits = map[string]int{
		"int8": 8, "int16": 16, "int32": 32, "int64": 64, "int": 64,
		"uint8": 9, "uint16": 17, "uint32": 33, "uint64": 65, "uint": 65,
		"sql.NullInt16": 16, "sql.NullInt32": 32, "sql.NullInt64": 64, "sql.NullByte": 9,
	}
	// kinds maps a Go type to the gorm data type it gets; a sql.Null* type
	// takes the kind of its first field.
	kinds = map[string]string{
		"bool": "bool", "sql.NullBool": "bool",
		"float32": "float", "float64": "float", "sql.NullFloat64": "float",
		"string": "string", "sql.NullString": "string",
		"time.Time": "time", "sql.NullTime": "time",
		"[]byte": "bytes",
	}
	identOnly   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	sqlKeywords = map[string]bool{"true": true, "false": true, "null": true, "current_timestamp": true, "current_date": true, "current_time": true, "localtimestamp": true, "localtime": true}
)

// maxVarchar is the longest varchar PostgreSQL accepts; gorm writes text above it.
const maxVarchar = 10485760

// intColumn is the PostgreSQL integer type for a gorm size: up to 16 bits a
// smallint, up to 32 an integer, otherwise a bigint. The same thresholds
// choose smallserial, serial and bigserial for an auto-incrementing key.
func intColumn(bits int) string {
	switch {
	case bits <= 16:
		return "smallint"
	case bits <= 32:
		return "integer"
	}
	return "bigint"
}

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
	bits := intBits[base]
	if bits != 0 && size > 0 {
		// gorm takes the size tag as the bit width of an integer, one more for an unsigned type
		bits = size
		if strings.HasPrefix(base, "uint") {
			bits++
		}
	}
	kind := kinds[base]
	switch {
	case tag["TYPE"] != "":
		fld.SQLType = tag["TYPE"]
	case bits != 0:
		fld.SQLType = intColumn(bits)
	case kind == "bool":
		fld.SQLType = "boolean"
	case kind == "float":
		fld.SQLType = "numeric"
		switch {
		case prec > 0 && scale > 0:
			fld.SQLType = fmt.Sprintf("numeric(%d,%d)", prec, scale)
		case prec > 0:
			fld.SQLType = fmt.Sprintf("numeric(%d)", prec)
		}
	case kind == "string":
		fld.SQLType = "text"
		if size > 0 && size <= maxVarchar {
			fld.SQLType = fmt.Sprintf("varchar(%d)", size)
		}
	case kind == "time":
		fld.SQLType = "timestamptz"
		if prec > 0 {
			fld.SQLType = fmt.Sprintf("timestamptz(%d)", prec)
		}
	case kind == "bytes":
		fld.SQLType = "bytea"
	default:
		unsupported("the type " + goType + " has no known column type; add a type: tag option or convert the field by hand")
		return
	}
	if q := qualifier(fld.GoType); q != "" && q != "time" && q != "sql" && q != "json" {
		unsupported("the type " + goType + " comes from package " + q + ", which the importer cannot place in a model file")
		return
	}
	isInt := intBits[base] != 0
	_, hasDefault := tag["DEFAULT"]
	autoSet := has(tag, "AUTOINCREMENT")
	// gorm gives a bare key the value of its own name, and any value but
	// false is true: a bare autoIncrement switches it on.
	autoOn := autoSet && !strings.EqualFold(tag["AUTOINCREMENT"], "false")
	if isInt && !hasDefault && (autoOn || fld.PK && !autoSet) {
		fld.Serial = true
		st.explicitAuto[n.Name] = autoOn
		if tag["TYPE"] != "" {
			r.rep.Add(importer.Change{Kind: importer.Naming, Source: pos, Model: m.Name, Field: n.Name,
				Message: "type:" + tag["TYPE"] + " on an auto-increment column: gorm replaces the type with smallserial, serial or bigserial by size; Momo keeps type:" + tag["TYPE"] + " with a serial sequence"})
		}
	}
	fld.Nullable = !(has(tag, "NOTNULL", "NOT NULL") || fld.PK)
	if !fld.Nullable && strings.HasPrefix(fld.GoType, "*") && !fld.PK {
		unsupported("a pointer type with not null: Momo reads nullability from the Go type, so the column would become nullable; change the field to " + strings.TrimPrefix(fld.GoType, "*"))
		return
	}
	r.applyDefault(m, &fld, tag, base, pos)
	r.applyIndexes(&fld, tag, all, m)
	for _, auto := range [][2]string{{"CreatedAt", "AUTOCREATETIME"}, {"UpdatedAt", "AUTOUPDATETIME"}} {
		name, key := auto[0], auto[1]
		if n.Name == name && !has(tag, key) && (kind == "time" || bits != 0) {
			r.rep.Add(importer.Change{Kind: importer.GoDefault, Source: pos, Model: m.Name, Field: n.Name, Message: "GORM sets a field named " + name + " in Go on create or update without any tag; Momo needs a created_at or updated_at tag or a database default"})
		}
	}
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
			composite := false
			for _, o := range strings.Split(opts, ",") {
				switch o = strings.TrimSpace(o); {
				case o == "":
				case o == "unique":
					unique = true
				case strings.HasPrefix(o, "priority:"):
					// the order of a field inside a composite index
				case o == "composite" || strings.HasPrefix(o, "composite:"):
					composite = true
				default:
					r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: fld.Source, Model: m.Name, Field: fld.Name, Message: "index option " + o + " is not imported"})
				}
			}
			if composite {
				r.rep.Add(importer.Change{Kind: importer.DroppedIndex, Source: fld.Source, Model: m.Name, Field: fld.Name, Message: "index " + name + " is composite (composite option); composite indexes are not imported, declare it in the companion file"})
				continue
			}
			if kept {
				r.rep.Add(importer.Change{Kind: importer.DroppedIndex, Source: fld.Source, Model: m.Name, Field: fld.Name, Message: "a field carries several index tags; only the first is kept, index " + name + " is dropped"})
				continue
			}
			kept = true
			if unique {
				if fld.Unique && !fld.UniqueIndex {
					r.rep.Add(importer.Change{Kind: importer.DroppedIndex, Source: fld.Source, Model: m.Name, Field: fld.Name, Message: "the field has both unique and a unique index; the unique constraint " + fld.UniqueName + " is dropped, the unique index " + name + " stays"})
				}
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
	names := make([]string, 0, len(users))
	for name := range users {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		idx := users[name]
		if len(idx) < 2 {
			continue
		}
		var spans []string
		for _, i := range idx {
			f := &m.Fields[i]
			spans = append(spans, f.Name)
			if f.IndexName == name {
				f.Index, f.IndexName = false, ""
			}
			if f.UniqueName == name {
				f.UniqueIndex, f.UniqueName = false, ""
				f.Unique = false
			}
		}
		r.rep.Add(importer.Change{Kind: importer.DroppedIndex, Source: m.Fields[idx[0]].Source, Model: m.Name, Field: m.Fields[idx[0]].Name,
			Message: "index " + name + " spans " + strings.Join(spans, ", ") + "; composite indexes are not imported, declare it in the companion file"})
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
		case "":
		default:
			r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: p.pos, Model: p.model, Field: p.field, Message: "the constraint option " + strings.TrimSpace(part) + " is not imported; only OnDelete and OnUpdate are"})
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
