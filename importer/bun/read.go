// SPDX-License-Identifier: Apache-2.0

// Package bun reads Bun models from Go source without running it and turns
// them into importer models plus a report of every behaviour that changes
// when the project moves to Momo.
//
// The reader follows github.com/uptrace/bun v1.3.0 with the PostgreSQL
// dialect (dialect/pgdialect v1.3.0), the combination the fixture module
// internal/compat/bunfixture verifies against a real database.
package bun

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/importer"
)

const (
	bunPath    = "github.com/uptrace/bun"
	schemaPath = "github.com/uptrace/bun/schema"
	// maxIdent is the longest identifier PostgreSQL keeps.
	maxIdent = 63
)

var namer = naming.Bun{}

// Read parses the Go files of dir (test files are skipped) and returns the
// models and the report. A model is a struct that embeds bun.BaseModel.
//
// It reads these bun tag options: the name and column, table and alias on
// bun.BaseModel, pk, autoincrement, notnull, nullzero, unique (bare, named and
// grouped), default, type, soft_delete, scanonly, embed, and rel:belongs-to
// with join, on_delete and on_update. Column types follow pgdialect's mapping
// (int is bigint, int32 is integer, string is varchar, time.Time is
// timestamptz) and a type: option wins. Table names follow Bun's default
// (the plural of the underscored type name) and are taken as written when a
// table: option names them.
//
// Always reported: zero-value-handling for every nullzero field of a value
// type (Bun writes NULL for a zero value, Momo writes the zero value),
// naming for every belongs-to relation (Bun only creates the constraint when
// the table is created with WithForeignKeys or by a migration, Momo always
// declares it), go-default for a BeforeAppendModel hook, renamed-type for a
// named type or bun.NullTime that is imported under another Go type and for a
// soft_delete field (the automatic filter does not carry over), dropped-field
// for fields that do not become columns (scanonly, "-", has-one, has-many and
// m2m relation fields) and for options that only change Bun's runtime,
// dropped-index for unique constraints Momo's tags cannot express, and
// unsupported for what cannot be converted.
//
// Bun creates no relation constraint and no index from a struct tag alone, and
// it saves no relation field on Insert or Update, so no implicit-save change
// is reported.
//
// Limits: only the Go files of dir are read. A NamingStrategy-like override
// (schema.SetTableNameInflector) is detected as a call in those files only.
// Indexes, foreign keys or columns that a migration, an AfterCreateTable hook
// or db.NewCreateIndex adds are not visible to a static reader; the
// round trip in internal/compat/bunfixture compares models created by
// db.NewCreateTable only.
func Read(dir string) ([]importer.Model, *importer.Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("importer/bun: %w", err)
	}
	r := &reader{
		fset:    token.NewFileSet(),
		rep:     &importer.Report{},
		seen:    map[importer.Change]bool{},
		structs: map[string]*structInfo{},
		named:   map[string]*namedInfo{},
		methods: map[string]map[string]token.Pos{},
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(r.fset, filepath.Join(dir, n), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, nil, fmt.Errorf("importer/bun: %w", err)
		}
		r.collect(f)
	}
	return r.build()
}

// fileCtx holds what a declaration needs to know about its file: the import
// path behind each local package name.
type fileCtx struct {
	imports map[string]string
}

func newFileCtx(f *ast.File) *fileCtx {
	fc := &fileCtx{imports: map[string]string{}}
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		name := p[strings.LastIndex(p, "/")+1:]
		if im.Name != nil {
			name = im.Name.Name
		}
		if name == "_" || name == "." {
			continue
		}
		fc.imports[name] = p
	}
	return fc
}

// qualified returns the import path and name of a pkg.Name expression.
func (fc *fileCtx) qualified(e ast.Expr) (pkgPath, name string, ok bool) {
	sel, isSel := e.(*ast.SelectorExpr)
	if !isSel {
		return "", "", false
	}
	id, isID := sel.X.(*ast.Ident)
	if !isID {
		return "", "", false
	}
	p, found := fc.imports[id.Name]
	if !found {
		return "", "", false
	}
	return p, sel.Sel.Name, true
}

type structInfo struct {
	name string
	spec *ast.TypeSpec
	st   *ast.StructType
	file *fileCtx
}

// namedInfo is a declared type that is not a struct.
type namedInfo struct {
	expr ast.Expr
	file *fileCtx
}

type reader struct {
	fset    *token.FileSet
	rep     *importer.Report
	seen    map[importer.Change]bool
	order   []string
	structs map[string]*structInfo
	named   map[string]*namedInfo
	methods map[string]map[string]token.Pos // receiver type -> method name -> position
}

func (r *reader) pos(p token.Pos) string {
	ps := r.fset.Position(p)
	return fmt.Sprintf("%s:%d:%d", filepath.Base(ps.Filename), ps.Line, ps.Column)
}

// add records c once: a struct embedded into several models would otherwise
// report the same problem for each of them.
func (r *reader) add(c importer.Change) {
	if r.seen[c] {
		return
	}
	r.seen[c] = true
	r.rep.Add(c)
}

func (r *reader) unsupported(pos, model, field, msg string) {
	r.add(importer.Change{Kind: importer.Unsupported, Source: pos, Model: model, Field: field, Message: msg})
}

func (r *reader) collect(f *ast.File) {
	fc := newFileCtx(f)
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			for _, s := range d.Specs {
				ts, ok := s.(*ast.TypeSpec)
				if !ok || ts.TypeParams != nil {
					continue
				}
				if st, ok := ts.Type.(*ast.StructType); ok {
					if _, dup := r.structs[ts.Name.Name]; !dup {
						r.order = append(r.order, ts.Name.Name)
					}
					r.structs[ts.Name.Name] = &structInfo{name: ts.Name.Name, spec: ts, st: st, file: fc}
					continue
				}
				r.named[ts.Name.Name] = &namedInfo{expr: ts.Type, file: fc}
			}
		case *ast.FuncDecl:
			if d.Recv == nil || len(d.Recv.List) != 1 {
				continue
			}
			recv := recvName(d.Recv.List[0].Type)
			if recv == "" {
				continue
			}
			if r.methods[recv] == nil {
				r.methods[recv] = map[string]token.Pos{}
			}
			if _, dup := r.methods[recv][d.Name.Name]; !dup {
				r.methods[recv][d.Name.Name] = d.Pos()
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetTableNameInflector" {
			r.add(importer.Change{Kind: importer.Naming, Source: r.pos(call.Pos()),
				Message: "SetTableNameInflector replaces Bun's table name pluralisation here; the importer assumes the default rules, so check the table names of the imported models. Only calls in the files of the read directory are detected"})
		}
		return true
	})
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

// isBaseModel reports whether e is bun.BaseModel (or schema.BaseModel).
func (fc *fileCtx) isBaseModel(e ast.Expr) bool {
	p, name, ok := fc.qualified(e)
	return ok && name == "BaseModel" && (p == bunPath || p == schemaPath)
}

func (r *reader) baseModelField(si *structInfo) *ast.Field {
	for _, f := range si.st.Fields.List {
		if len(f.Names) == 0 && si.file.isBaseModel(f.Type) {
			return f
		}
	}
	return nil
}

// bunTag returns the value of the bun key of f's struct tag. ok is false (and
// the problem reported) when the tag literal cannot be read.
func (r *reader) bunTag(f *ast.Field, model string) (string, bool) {
	if f.Tag == nil {
		return "", true
	}
	raw, err := strconv.Unquote(f.Tag.Value)
	if err != nil {
		r.unsupported(r.pos(f.Tag.Pos()), model, "", "the struct tag "+f.Tag.Value+" cannot be read: "+err.Error())
		return "", false
	}
	return reflect.StructTag(raw).Get("bun"), true
}

type pendingRel struct {
	name  string
	field *ast.Field
	tag   tag
	pos   string
}

type modelBuild struct {
	m        importer.Model
	byColumn map[string]int // column -> index in m.Fields
	uniq     map[string][]int
	rels     []pendingRel
}

func (r *reader) build() ([]importer.Model, *importer.Report, error) {
	var builds []*modelBuild
	byName := map[string]*modelBuild{}
	for _, name := range r.order {
		si := r.structs[name]
		if r.baseModelField(si) == nil {
			continue
		}
		mb := r.model(si)
		builds = append(builds, mb)
		byName[name] = mb
	}
	for _, mb := range builds {
		for _, p := range mb.rels {
			r.belongsTo(mb, byName, p)
		}
	}
	models := make([]importer.Model, 0, len(builds))
	for _, mb := range builds {
		models = append(models, mb.m)
	}
	return models, r.rep, nil
}

// checkValue rejects text a momo tag cannot carry (the emitter would refuse it
// later with a less specific message).
func (r *reader) checkValue(pos, model, field, what, v string) bool {
	if strings.ContainsAny(v, ",`\n\r") {
		r.unsupported(pos, model, field, what+" "+strconv.Quote(v)+" contains a comma, backtick or newline, which a momo tag cannot carry; set it by hand in the imported model")
		return false
	}
	return true
}

var knownTableOptions = map[string]bool{"table": true, "alias": true, "select": true}

func (r *reader) model(si *structInfo) *modelBuild {
	mb := &modelBuild{
		m:        importer.Model{Name: si.name, Source: r.pos(si.spec.Pos()), Table: namer.TableName(si.name)},
		byColumn: map[string]int{},
		uniq:     map[string][]int{},
	}
	r.baseModelTag(mb, si)
	r.hooks(mb)
	for _, c := range r.candidates(si, map[string]bool{si.name: true}) {
		r.field(mb, c)
	}
	r.uniques(mb)
	seenName, seenCol := map[string]bool{}, map[string]bool{}
	for _, f := range mb.m.Fields {
		if seenName[f.Name] || seenCol[f.Column] {
			r.unsupported(f.Source, si.name, f.Name, "the field name "+f.Name+" or the column "+f.Column+" is declared twice in "+si.name+" (for example by two embedded structs)")
		}
		seenName[f.Name], seenCol[f.Column] = true, true
	}
	return mb
}

// baseModelTag reads the table name from the tag of the embedded bun.BaseModel.
func (r *reader) baseModelTag(mb *modelBuild, si *structInfo) {
	bf := r.baseModelField(si)
	pos := r.pos(bf.Pos())
	raw, ok := r.bunTag(bf, si.name)
	if !ok {
		return
	}
	t := parseTag(raw)
	if knownTableOptions[t.name] {
		r.add(importer.Change{Kind: importer.Naming, Source: pos, Model: si.name,
			Message: "the BaseModel tag name " + strconv.Quote(t.name) + " is also an option name; Bun uses it as the table name (and warns), write table:" + t.name + " if that is not intended"})
	}
	for _, k := range sortedKeys(t.opts) {
		if !knownTableOptions[k] {
			r.unsupported(pos, si.name, "", "the BaseModel option "+k+" is not known to Bun v1.3.0 (Bun ignores it with a warning)")
		}
	}
	table := mb.m.Table
	if t.name != "" {
		table = t.name
	}
	if v, ok := t.option("table"); ok {
		table = v
	}
	if t.has("select") {
		r.unsupported(pos, si.name, "", "the select: option reads the model from another table or view; declare a Momo view model by hand")
	}
	if strings.Contains(table, ".") {
		r.unsupported(pos, si.name, "", "the table name "+strconv.Quote(table)+" is schema-qualified; the importer does not read schemas")
		return
	}
	if !r.checkValue(pos, si.name, "", "the table name", table) {
		return
	}
	mb.m.Table = table
}

// hookReport describes what a model hook changes when the model moves to Momo.
var hookReport = []struct {
	name string
	kind importer.Kind
	msg  string
}{
	{"BeforeAppendModel", importer.GoDefault, "Bun calls BeforeAppendModel before it writes the model (typically to set created_at and updated_at in Go); Momo does not call it, so reproduce the effect with a created_at or updated_at tag or a database default"},
	{"BeforeScanRow", importer.ImplicitSave, "Bun runs BeforeScanRow implicitly for every scanned row; Momo does not call it"},
	{"AfterScanRow", importer.ImplicitSave, "Bun runs AfterScanRow implicitly for every scanned row; Momo does not call it"},
	{"BeforeCreateTable", importer.Unsupported, "BeforeCreateTable can change the CREATE TABLE statement (extra foreign keys, partitioning), which a static reader cannot see; check the imported schema by hand"},
	{"AfterCreateTable", importer.Unsupported, "AfterCreateTable usually adds indexes or constraints after the table is created, which a static reader cannot see; check the imported schema by hand"},
}

func (r *reader) hooks(mb *modelBuild) {
	for _, h := range hookReport {
		if p, ok := r.methods[mb.m.Name][h.name]; ok {
			r.add(importer.Change{Kind: h.kind, Source: r.pos(p), Model: mb.m.Name, Message: h.msg})
		}
	}
}

// cand is one struct field in the form Bun's Table sees it after embedding.
type cand struct {
	field    *ast.Field
	name     *ast.Ident
	tag      tag
	sql      string
	idx      []int
	owner    *structInfo
	prefix   string
	embedded bool
}

// candidates returns the fields of si the way Bun's Table.processFields does:
// anonymous and embed: structs are flattened in place, a top-level name wins
// over an embedded one, and an ambiguous embedded name is left out.
func (r *reader) candidates(si *structInfo, stack map[string]bool) []cand {
	var direct, flat []cand
	names := map[string]bool{}
	k := 0
	for _, f := range si.st.Fields.List {
		raw, ok := r.bunTag(f, si.name)
		count := max(1, len(f.Names))
		if !ok {
			k += count
			continue
		}
		if raw == "-" {
			for _, id := range f.Names {
				names[id.Name] = true
				if id.IsExported() {
					r.add(importer.Change{Kind: importer.DroppedField, Source: r.pos(id.Pos()), Model: si.name, Field: id.Name,
						Message: `bun:"-" hides the field from Bun entirely; it has no column and is not imported`})
				}
			}
			if len(f.Names) == 0 {
				names[types.ExprString(f.Type)] = true
			}
			k += count
			continue
		}
		tg := parseTag(raw)
		if len(f.Names) == 0 {
			if si.file.isBaseModel(f.Type) {
				k++
				continue
			}
			flat = append(flat, r.embed(si, f, tg, "", stack, k, true)...)
			k++
			continue
		}
		for _, id := range f.Names {
			idx := k
			k++
			if !id.IsExported() {
				continue
			}
			if prefix, ok := tg.option("embed"); ok {
				flat = append(flat, r.embed(si, f, tg, prefix, stack, idx, false)...)
				continue
			}
			sql := namer.ColumnName(id.Name)
			if tg.name != "" && tg.name != sql {
				if knownFieldOptions[tg.name] {
					r.add(importer.Change{Kind: importer.Naming, Source: r.pos(id.Pos()), Model: si.name, Field: id.Name,
						Message: "the tag name " + strconv.Quote(tg.name) + " is also an option name; Bun uses it as the column name (and warns), write column:" + tg.name + " if that is not intended"})
				}
				sql = tg.name
			}
			if v, ok := tg.option("column"); ok {
				sql = v
			}
			direct = append(direct, cand{field: f, name: id, tag: tg, sql: sql, idx: []int{idx}, owner: si})
			names[sql] = true
		}
	}
	ambName, ambTag := map[string]int{}, map[string]int{}
	for n := range names {
		ambName[n]++
		ambTag[n]++
	}
	for _, c := range flat {
		ambName[c.sql]++
		if !c.tag.zero() {
			ambTag[c.sql]++
		}
	}
	out := direct
	for _, c := range flat {
		if ambName[c.sql] > 1 && (c.tag.zero() || ambTag[c.sql] != 1) {
			r.add(importer.Change{Kind: importer.DroppedField, Source: r.pos(c.name.Pos()), Model: si.name, Field: c.name.Name,
				Message: "the embedded field is ambiguous (another field maps to " + strconv.Quote(c.sql) + "); Bun leaves it out, so it is not imported"})
			continue
		}
		out = append(out, c)
	}
	if len(flat) > 0 {
		slices.SortStableFunc(out, func(a, b cand) int { return slices.Compare(a.idx, b.idx) })
	}
	return out
}

// embed flattens the struct that field f embeds (anonymously or with an
// embed: prefix) into the candidates of its parent.
func (r *reader) embed(si *structInfo, f *ast.Field, tg tag, prefix string, stack map[string]bool, idx int, anonymous bool) []cand {
	pos := r.pos(f.Pos())
	label := types.ExprString(f.Type)
	if anonymous && (tg.has("inherit") || tg.has("extend")) {
		r.unsupported(pos, si.name, label, "inherit and extend take the table name of the embedded struct; set the table with table: on bun.BaseModel by hand")
	}
	t := f.Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	var sub *structInfo
	if id, ok := t.(*ast.Ident); ok {
		sub = r.structs[id.Name]
		if sub == nil && r.named[id.Name] != nil && anonymous {
			return nil // Bun ignores an embedded type that is not a struct
		}
	}
	if sub == nil {
		r.unsupported(pos, si.name, label, "the embedded type "+label+" is not a struct declared in the read directory, so its columns cannot be read")
		return nil
	}
	if stack[sub.name] {
		r.unsupported(pos, si.name, label, "the struct "+sub.name+" embeds itself, directly or through another embedded struct")
		return nil
	}
	stack[sub.name] = true
	defer delete(stack, sub.name)
	children := r.candidates(sub, stack)
	for i := range children {
		c := &children[i]
		c.embedded = true
		c.prefix = prefix
		c.sql = prefix + c.sql
		c.idx = append([]int{idx}, c.idx...)
	}
	return children
}

// knownFieldOptions are the field options Bun v1.3.0 knows.
var knownFieldOptions = map[string]bool{
	"column": true, "alt": true, "type": true, "array": true, "hstore": true, "composite": true,
	"multirange": true, "json_use_number": true, "msgpack": true, "notnull": true, "nullzero": true,
	"default": true, "unique": true, "soft_delete": true, "scanonly": true, "skipupdate": true,
	"pk": true, "autoincrement": true, "rel": true, "join": true, "join_on": true, "on_update": true,
	"on_delete": true, "m2m": true, "polymorphic": true, "identity": true,
}

func (r *reader) field(mb *modelBuild, c cand) {
	pos := r.pos(c.name.Pos())
	if c.tag.has("rel") || c.tag.has("m2m") {
		r.relation(mb, c, pos)
		return
	}
	if c.tag.has("scanonly") {
		r.add(importer.Change{Kind: importer.DroppedField, Source: pos, Model: mb.m.Name, Field: c.name.Name,
			Message: "scanonly: Bun scans the column from query results but never writes it and creates no column for it; the field is not imported"})
		return
	}
	r.column(mb, c, pos)
}

// relation handles a field with a rel or m2m option.
func (r *reader) relation(mb *modelBuild, c cand, pos string) {
	name := c.name.Name
	rel, hasRel := c.tag.option("rel")
	switch {
	case hasRel && rel == "belongs-to":
		mb.rels = append(mb.rels, pendingRel{name: name, field: c.field, tag: c.tag, pos: pos})
	case hasRel && (rel == "has-one" || rel == "has-many"):
		r.add(importer.Change{Kind: importer.DroppedField, Source: pos, Model: mb.m.Name, Field: name,
			Message: rel + " relation field: Bun creates no constraint for it and Momo declares a relation on the model that holds the foreign key, so the field is not imported; the key column on the other model is imported as a plain column (declare belongs-to there if it should be a foreign key)"})
	case hasRel:
		r.unsupported(pos, mb.m.Name, name, "unknown relation rel:"+rel+" (Bun panics on it)")
	default:
		r.add(importer.Change{Kind: importer.DroppedField, Source: pos, Model: mb.m.Name, Field: name,
			Message: "m2m relation field: it has no column of its own and Momo has no equivalent field; the junction table is imported as the model that holds its two keys"})
	}
}

// basicSQL is pgdialect's column type for a Go kind (schema.sqlTypes mapped
// through pgdialect.sqlType in v1.3.0).
var basicSQL = map[string]string{
	"bool": "boolean", "int": "bigint", "int8": "smallint", "int16": "smallint", "int32": "integer",
	"int64": "bigint", "uint": "bigint", "uint8": "smallint", "uint16": "smallint", "uint32": "integer",
	"uint64": "bigint", "byte": "smallint", "rune": "integer", "float32": "real",
	"float64": "double precision", "string": "varchar",
}

// colType is what the reader makes of a field's Go type.
type colType struct {
	goType  string // the type written in the model file; empty when none can be
	sql     string // pgdialect's column type; empty when Bun derives none
	reason  string // why goType or sql is missing
	renamed string // set when the Go type changes on import
}

var extTypes = map[string]colType{
	"time.Time":                {goType: "time.Time", sql: "timestamptz"},
	"database/sql.NullString":  {goType: "sql.NullString", sql: "varchar"},
	"database/sql.NullInt64":   {goType: "sql.NullInt64", sql: "bigint"},
	"database/sql.NullBool":    {goType: "sql.NullBool", sql: "boolean"},
	"database/sql.NullFloat64": {goType: "sql.NullFloat64", sql: "double precision"},
	"database/sql.NullTime":    {goType: "sql.NullTime", sql: "timestamptz"},
	"database/sql.NullInt32":   {goType: "sql.NullInt32", reason: "Bun v1.3.0 maps sql.NullInt32 to jsonb (a struct it does not know), which is not a nullable integer; add type:integer or use *int32"},
	"database/sql.NullInt16":   {goType: "sql.NullInt16", reason: "Bun v1.3.0 maps sql.NullInt16 to jsonb (a struct it does not know), which is not a nullable integer; add type:smallint or use *int16"},
	"database/sql.NullByte":    {goType: "sql.NullByte", reason: "Bun v1.3.0 maps sql.NullByte to jsonb (a struct it does not know); add a type: option or use *uint8"},
	"encoding/json.RawMessage": {goType: "json.RawMessage", sql: "jsonb"},
}

const jsonReason = "Bun stores a struct, map or slice as jsonb through encoding/json; Momo needs a types.JSON field, which the importer cannot place, so convert the field by hand"

// classify maps the Go type e to a column type.
func (r *reader) classify(fc *fileCtx, e ast.Expr, depth int) colType {
	if depth > 8 {
		return colType{reason: "the type is declared through too many layers to follow"}
	}
	switch t := e.(type) {
	case *ast.Ident:
		return r.classifyIdent(t, depth)
	case *ast.SelectorExpr:
		p, name, ok := fc.qualified(t)
		if !ok {
			return colType{reason: "the type " + types.ExprString(t) + " comes from a package the importer cannot resolve"}
		}
		if ct, ok := extTypes[p+"."+name]; ok {
			return ct
		}
		if (p == bunPath || p == schemaPath) && name == "NullTime" {
			return colType{goType: "*time.Time", sql: "timestamptz", renamed: "bun.NullTime"}
		}
		return colType{reason: "the type " + types.ExprString(t) + " comes from package " + p + ", which the importer cannot place in a model file; convert the field by hand"}
	case *ast.ArrayType:
		if id, ok := t.Elt.(*ast.Ident); ok && t.Len == nil && (id.Name == "byte" || id.Name == "uint8") {
			return colType{goType: "[]byte", sql: "bytea"}
		}
		return colType{reason: jsonReason}
	case *ast.MapType, *ast.StructType:
		return colType{reason: jsonReason}
	}
	return colType{reason: "the type " + types.ExprString(e) + " has no column type the importer knows"}
}

func (r *reader) classifyIdent(id *ast.Ident, depth int) colType {
	if sql, ok := basicSQL[id.Name]; ok {
		return colType{goType: id.Name, sql: sql}
	}
	if r.structs[id.Name] != nil {
		return colType{reason: "the struct " + id.Name + " is stored by Bun as jsonb through encoding/json (it has no rel: option); " + jsonReason}
	}
	ni := r.named[id.Name]
	if ni == nil {
		return colType{reason: "the type " + id.Name + " is not declared in the read directory, so its column type is unknown"}
	}
	for _, m := range []string{"Value", "Scan", "AppendQuery"} {
		if _, ok := r.methods[id.Name][m]; ok {
			return colType{reason: "the type " + id.Name + " has a " + m + " method, so Bun stores it through that method; convert the field by hand"}
		}
	}
	if _, ok := ni.expr.(*ast.SelectorExpr); ok {
		return colType{reason: "the type " + id.Name + " is defined over " + types.ExprString(ni.expr) + "; Bun maps a defined type by its kind, not by the type underneath, so its column type is not what the importer can derive; add a type: option and convert the field by hand"}
	}
	inner := r.classify(ni.file, ni.expr, depth+1)
	if inner.goType != "" && inner.renamed == "" {
		inner.renamed = id.Name
	}
	return inner
}

// column turns a plain field into a Field of the model.
func (r *reader) column(mb *modelBuild, c cand, pos string) {
	m := &mb.m
	t := c.tag
	fname := c.name.Name
	ok := true
	bad := func(msg string) {
		r.unsupported(pos, m.Name, fname, msg)
		ok = false
	}
	for _, k := range sortedKeys(t.opts) {
		switch k {
		case "column", "notnull", "nullzero", "default", "unique", "soft_delete", "pk", "autoincrement", "type":
		case "alt", "skipupdate", "json_use_number":
			r.add(importer.Change{Kind: importer.DroppedField, Source: pos, Model: m.Name, Field: fname,
				Message: "the option " + k + " only changes how Bun reads or writes the value at run time and is not imported; the column stays"})
		case "join", "join_on", "on_update", "on_delete", "polymorphic":
			r.add(importer.Change{Kind: importer.DroppedField, Source: pos, Model: m.Name, Field: fname,
				Message: "the option " + k + " only has a meaning together with rel: and does nothing here"})
		case "array", "hstore", "composite", "multirange", "msgpack", "identity":
			bad("the option " + k + " changes the column type or the stored encoding and is not imported; convert the field by hand")
		default:
			bad("the option " + k + " is not known to Bun v1.3.0 (Bun ignores it with a warning)")
		}
	}
	if !ok {
		return
	}
	userType, _ := t.option("type")
	e := c.field.Type
	ptr := false
	if s, isStar := e.(*ast.StarExpr); isStar {
		ptr, e = true, s.X
	}
	ct := r.classify(c.owner.file, e, 0)
	if ct.goType != "" && ptr && !strings.HasPrefix(ct.goType, "*") {
		ct.goType = "*" + ct.goType
	}
	sqlType := userType
	if sqlType == "" {
		sqlType = ct.sql
	}
	if ct.goType == "" || sqlType == "" {
		reason := ct.reason
		if reason == "" {
			reason = "the type has no column type the importer knows"
		}
		r.unsupported(pos, m.Name, fname, reason)
		return
	}
	if ct.renamed != "" {
		r.add(importer.Change{Kind: importer.RenamedType, Source: pos, Model: m.Name, Field: fname,
			Message: ct.renamed + " is imported as " + ct.goType + "; check the conversions in the code that uses the field"})
	}
	fld := importer.Field{Name: fname, Column: c.sql, GoType: ct.goType, SQLType: sqlType, Source: pos}
	fld.PK = t.has("pk")
	auto := t.has("autoincrement")
	fld.Nullable = !(t.has("notnull") || fld.PK || auto)
	if !fld.Nullable && strings.HasPrefix(fld.GoType, "*") && !fld.PK {
		r.unsupported(pos, m.Name, fname, "a pointer type with notnull: Momo reads nullability from the Go type, so the column would become nullable; change the field to "+strings.TrimPrefix(fld.GoType, "*"))
		return
	}
	// pgdialect turns an integer column with autoincrement into a serial type by
	// comparing its upper-case type name, so a lower-case type: option or a
	// non-integer type keeps the plain type and no sequence.
	if auto {
		discovered := userType
		if discovered == "" {
			discovered = strings.ToUpper(ct.sql)
		}
		switch discovered {
		case "SMALLINT", "INTEGER", "BIGINT":
			fld.Serial = true
			fld.SQLType = strings.ToLower(discovered)
		default:
			r.add(importer.Change{Kind: importer.Naming, Source: pos, Model: m.Name, Field: fname,
				Message: "autoincrement has no effect on the column type " + sqlType + " under pgdialect v1.3.0 (it only turns the upper-case types SMALLINT, INTEGER and BIGINT into serial types); Bun creates the column without a sequence and so does the import"})
		}
	}
	if t.has("nullzero") && !strings.HasPrefix(fld.GoType, "*") && !strings.HasPrefix(fld.GoType, "sql.Null") {
		r.add(importer.Change{Kind: importer.ZeroValueHandling, Source: pos, Model: m.Name, Field: fname,
			Message: "nullzero: Bun writes NULL (or the column default) when " + fname + " holds its zero value; Momo writes the zero value, so use a pointer type or a sql.Null type to store NULL"})
	}
	if t.has("soft_delete") {
		r.add(importer.Change{Kind: importer.RenamedType, Source: pos, Model: m.Name, Field: fname,
			Message: "soft_delete: Bun adds the filter " + c.sql + " IS NULL to queries and turns Delete into an update; the column is imported as a plain field and the automatic filter does not carry over"})
	}
	if v, has := t.option("default"); has && v != "" {
		fld.Default, fld.DefaultKind = v, "db"
	}
	if !r.checkValue(pos, m.Name, fname, "the column name", fld.Column) ||
		!r.checkValue(pos, m.Name, fname, "the column type", fld.SQLType) ||
		!r.checkValue(pos, m.Name, fname, "the default", fld.Default) {
		return
	}
	idx := len(m.Fields)
	m.Fields = append(m.Fields, fld)
	mb.byColumn[fld.Column] = idx
	if vs, has := t.opts["unique"]; has {
		names := vs
		if len(vs) == 1 {
			names = strings.Split(vs[0], ",")
		}
		for _, un := range names {
			if un != "" && c.embedded && c.prefix != "" {
				un = c.prefix + un
			}
			mb.uniq[un] = append(mb.uniq[un], idx)
		}
	}
}

// uniques turns Bun's unique groups into field-level unique constraints. A
// bare unique is one constraint per field, named by PostgreSQL
// <table>_<column>_key; unique:name is one constraint across every field that
// names it. Momo's tags carry one single-column unique per field, so a group
// across fields, and a second constraint on one field, are reported.
func (r *reader) uniques(mb *modelBuild) {
	m := &mb.m
	keys := sortedKeys(mb.uniq)
	set := func(i int, name string) {
		f := &m.Fields[i]
		if f.Unique {
			r.add(importer.Change{Kind: importer.DroppedIndex, Source: f.Source, Model: m.Name, Field: f.Name,
				Message: "the field is in several unique constraints; Momo's tags carry one, so the constraint " + strconv.Quote(name) + " is dropped"})
			return
		}
		f.Unique = true
		if name == "" {
			name = m.Table + "_" + f.Column + "_key"
			if len(name) > maxIdent {
				r.add(importer.Change{Kind: importer.Naming, Source: f.Source, Model: m.Name, Field: f.Name,
					Message: "PostgreSQL shortens the generated unique constraint name " + name + "; Momo derives its own name for it"})
				name = ""
			}
		}
		f.UniqueName = name
	}
	for _, key := range keys {
		idxs := slices.Compact(slices.Clone(mb.uniq[key]))
		if key == "" {
			for _, i := range idxs {
				set(i, "")
			}
			continue
		}
		if len(idxs) > 1 {
			var spans []string
			for _, i := range idxs {
				spans = append(spans, m.Fields[i].Name)
			}
			r.add(importer.Change{Kind: importer.DroppedIndex, Source: m.Fields[idxs[0]].Source, Model: m.Name, Field: m.Fields[idxs[0]].Name,
				Message: "the unique constraint " + key + " spans " + strings.Join(spans, ", ") + "; composite unique constraints are not imported, declare it in the companion file"})
			continue
		}
		set(idxs[0], key)
	}
}

// fkRule maps a Bun on_delete or on_update value to the importer's action.
func fkRule(t tag, key string) (action string, problem string) {
	vs, ok := t.opts[key]
	if !ok {
		return "", ""
	}
	if len(vs) > 1 {
		return "", key + " must be a single value (Bun panics on it)"
	}
	rule := strings.ToUpper(strings.TrimSpace(vs[0]))
	switch rule {
	case "CASCADE", "RESTRICT", "SET NULL", "SET DEFAULT":
		return rule, ""
	case "NO ACTION":
		return "", ""
	}
	return "", key + ":" + vs[0] + " is not a foreign key action"
}

// belongsTo resolves a rel:belongs-to field to a foreign key the way Bun's
// Table.belongsToRelation finds its columns.
func (r *reader) belongsTo(mb *modelBuild, byName map[string]*modelBuild, p pendingRel) {
	m := &mb.m
	fail := func(msg string) { r.unsupported(p.pos, m.Name, p.name, msg) }
	e := p.field.Type
	if s, ok := e.(*ast.StarExpr); ok {
		e = s.X
	}
	id, ok := e.(*ast.Ident)
	if !ok {
		fail("the belongs-to field must have a model type or a pointer to one, got " + types.ExprString(p.field.Type))
		return
	}
	tb := byName[id.Name]
	if tb == nil {
		fail("the belongs-to target " + id.Name + " is not a struct with bun.BaseModel in the read directory")
		return
	}
	var pks []int
	for i, f := range tb.m.Fields {
		if f.PK {
			pks = append(pks, i)
		}
	}
	var base, ref []string
	if join, has := p.tag.opts["join"]; has {
		parts := join
		if len(join) == 1 {
			parts = strings.Split(join[0], ",")
		}
		for _, s := range parts {
			kv := strings.Split(strings.TrimSpace(s), "=")
			if len(kv) != 2 {
				fail("the join option " + strconv.Quote(strings.Join(join, ",")) + " cannot be parsed (Bun panics on it)")
				return
			}
			base, ref = append(base, kv[0]), append(ref, kv[1])
		}
	} else {
		if len(pks) == 0 {
			fail("the belongs-to target " + tb.m.Name + " has no primary key (Bun panics on it)")
			return
		}
		prefix := namer.ColumnName(p.name) + "_"
		for _, i := range pks {
			col := tb.m.Fields[i].Column
			ref = append(ref, col)
			switch {
			case hasCol(mb.byColumn, prefix+col):
				base = append(base, prefix+col)
			case hasCol(mb.byColumn, col):
				base = append(base, col)
			default:
				fail("the key column " + prefix + col + " (or " + col + ") is not a column of " + m.Name + "; use join:column=" + col + " (Bun panics on it)")
				return
			}
		}
	}
	if len(base) != 1 || len(pks) != 1 {
		fail("composite belongs-to relations are not imported; Momo's belongs_to tag carries one key column")
		return
	}
	li, lok := mb.byColumn[base[0]]
	ri, rok := tb.byColumn[ref[0]]
	if !lok || !rok {
		fail("the join columns " + base[0] + "=" + ref[0] + " are not both columns of " + m.Name + " and " + tb.m.Name + " (Bun panics on it)")
		return
	}
	if !tb.m.Fields[ri].PK {
		fail("the relation references " + tb.m.Name + "." + ref[0] + ", which is not the primary key; Momo's belongs_to references the primary key")
		return
	}
	onDelete, bad := fkRule(p.tag, "on_delete")
	if bad != "" {
		fail(bad)
		return
	}
	onUpdate, bad := fkRule(p.tag, "on_update")
	if bad != "" {
		fail(bad)
		return
	}
	local := m.Fields[li]
	m.ForeignKeys = append(m.ForeignKeys, importer.ForeignKey{
		Name: m.Table + "_" + local.Column + "_fkey", Field: p.name, RefModel: tb.m.Name, FKField: local.Name,
		Columns: []string{local.Column}, RefTable: tb.m.Table, RefColumns: []string{tb.m.Fields[ri].Column},
		OnDelete: onDelete, OnUpdate: onUpdate,
	})
	r.add(importer.Change{Kind: importer.Naming, Source: p.pos, Model: m.Name, Field: p.name,
		Message: "Bun creates a foreign key for this relation only when the table is created with WithForeignKeys() (and then only if " + local.Column + " is not part of the primary key, or " + m.Name + " is a many-to-many junction table) or by a migration; the import always declares " + m.Table + "_" + local.Column + "_fkey, so a database without it gets the constraint on the next migrate diff"})
}

func hasCol(m map[string]int, k string) bool {
	_, ok := m[k]
	return ok
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
