// SPDX-License-Identifier: Apache-2.0

// Package ent reads an Ent schema package from Go source without running it
// and turns it into importer models plus a report of every behaviour that
// changes when the project moves to Momo.
//
// The reader is static (go/parser and go/ast): the schema package is never
// compiled or executed, which is why it only understands the builder chains
// Ent schemas are written with and reports everything else as unsupported.
package ent

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/importer"
)

const (
	entPath     = "entgo.io/ent"
	fieldPath   = "entgo.io/ent/schema/field"
	edgePath    = "entgo.io/ent/schema/edge"
	indexPath   = "entgo.io/ent/schema/index"
	mixinPath   = "entgo.io/ent/schema/mixin"
	entsqlPath  = "entgo.io/ent/dialect/entsql"
	dialectPath = "entgo.io/ent/dialect"
)

var namer = naming.Ent{}

// Read parses the Go files of dir, an Ent schema package (test files are
// skipped), and returns the models and the report. The model of a schema type
// has the table Ent creates for it (entsql.Annotation Table, Config Table, or
// the snake-cased plural of the type name), an id column (an identity bigint,
// Ent's default key), one column per field and the foreign key columns Ent
// derives from the edges.
//
// It reads these field constructors: String, Text, Bool, Int, Int8, Int16,
// Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64, Float, Float32, Time,
// Bytes and Enum, with Optional, Nillable, Unique, Immutable, Default,
// DefaultFunc, UpdateDefault, StorageKey, SchemaType (the postgres entry),
// Values, entsql.Default and entsql.DefaultExpr annotations, and the
// validators (MaxLen, MinLen, NotEmpty, Match, Validate, Min, Max, Range,
// Positive, Negative, NonNegative), plus Comment, Sensitive, StructTag and
// Deprecated, which do not touch the schema. Edges are edge.To and edge.From
// with Ref, Unique, Required, Field, StorageKey (Column and Symbol) and the
// entsql.OnDelete annotation. Indexes are index.Fields and index.Edges with
// Unique and StorageKey. Mixins are mixin.Time, mixin.CreateTime,
// mixin.UpdateTime and local mixin types of the package.
//
// Annotations from packages other than entsql (entgql, entproto and the like)
// do not change the SQL schema Ent creates and are ignored.
//
// Always reported: implicit-save for every edge (Ent writes an edge only when
// its setter is called), zero-value-handling for every non-pointer field with a
// literal default (Ent applies the default when the setter was not called, and
// Momo writes the zero value; the database default is kept), go-default for
// defaults and update defaults that are Go functions, go-side-rule for
// validators, immutable fields, hooks and policies, naming for foreign key
// names Momo derives itself, dropped-index for indexes Momo's tags cannot
// express, and unsupported for everything the reader does not handle (a
// many-to-many edge, JSON and UUID fields, an unknown builder method).
func Read(dir string) ([]importer.Model, *importer.Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("importer/ent: %w", err)
	}
	r := &reader{fset: token.NewFileSet(), rep: &importer.Report{}, types: map[string]*typeInfo{}, methods: map[string]map[string]method{}}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(r.fset, filepath.Join(dir, n), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, nil, fmt.Errorf("importer/ent: %w", err)
		}
		r.collect(f)
	}
	return r.build()
}

// fileInfo maps the package names a file uses to their import paths.
type fileInfo struct{ imports map[string]string }

// pkg returns the import path e names, or "".
func (f *fileInfo) pkg(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return f.imports[id.Name]
	}
	return ""
}

// is reports whether e is path.name.
func (f *fileInfo) is(e ast.Expr, importPath, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name && f.pkg(sel.X) == importPath
}

type method struct {
	decl *ast.FuncDecl
	file *fileInfo
}

type typeInfo struct {
	name   string
	spec   *ast.TypeSpec
	file   *fileInfo
	entity bool // embeds ent.Schema
	mixin  bool // embeds mixin.Schema
	view   bool // embeds ent.View
}

type reader struct {
	fset    *token.FileSet
	rep     *importer.Report
	order   []string
	types   map[string]*typeInfo
	methods map[string]map[string]method // type name -> method name -> declaration
	fks     []*fk
}

func (r *reader) pos(p token.Pos) string {
	ps := r.fset.Position(p)
	return fmt.Sprintf("%s:%d:%d", filepath.Base(ps.Filename), ps.Line, ps.Column)
}

func (r *reader) unsupported(p token.Pos, model, field, msg string) {
	r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: r.pos(p), Model: model, Field: field, Message: msg})
}

// unsupportedAt is unsupported for a position already rendered.
func (r *reader) unsupportedAt(src, model, field, msg string) {
	r.rep.Add(importer.Change{Kind: importer.Unsupported, Source: src, Model: model, Field: field, Message: msg})
}

func (r *reader) collect(f *ast.File) {
	fi := &fileInfo{imports: map[string]string{}}
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		if im.Name != nil {
			name = im.Name.Name
		}
		if name != "_" && name != "." {
			fi.imports[name] = p
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
				ti := &typeInfo{name: ts.Name.Name, spec: ts, file: fi}
				for _, fld := range st.Fields.List {
					if len(fld.Names) != 0 {
						continue
					}
					ti.entity = ti.entity || fi.is(fld.Type, entPath, "Schema")
					ti.mixin = ti.mixin || fi.is(fld.Type, mixinPath, "Schema")
					ti.view = ti.view || fi.is(fld.Type, entPath, "View")
				}
				if ti.entity || ti.mixin || ti.view {
					r.types[ti.name] = ti
					r.order = append(r.order, ti.name)
				}
			}
		case *ast.FuncDecl:
			if d.Recv == nil || len(d.Recv.List) != 1 {
				continue
			}
			recv := recvName(d.Recv.List[0].Type)
			if r.methods[recv] == nil {
				r.methods[recv] = map[string]method{}
			}
			r.methods[recv][d.Name.Name] = method{decl: d, file: fi}
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

// elem is one element of a returned slice literal, with the file it is in.
type elem struct {
	expr ast.Expr
	file *fileInfo
}

// sliceElems returns the elements of the slice literal the method name of
// type ti returns. A method that does more than return a literal is reported.
func (r *reader) sliceElems(ti *typeInfo, name string) []elem {
	m, ok := r.methods[ti.name][name]
	if !ok {
		return nil
	}
	if m.decl.Body != nil && len(m.decl.Body.List) == 1 {
		if ret, ok := m.decl.Body.List[0].(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
			if cl, ok := ret.Results[0].(*ast.CompositeLit); ok {
				if _, isSlice := cl.Type.(*ast.ArrayType); isSlice {
					out := make([]elem, 0, len(cl.Elts))
					for _, e := range cl.Elts {
						out = append(out, elem{expr: e, file: m.file})
					}
					return out
				}
			}
		}
	}
	r.unsupported(m.decl.Pos(), ti.name, "", name+" does not just return a slice literal, so its contents cannot be read without running it; convert the "+strings.ToLower(name)+" by hand")
	return nil
}

// call is one method call of a builder chain.
type call struct {
	name string
	args []ast.Expr
	pos  token.Pos
}

// chain is a builder chain: pkg.Ctor(args).M1(...).M2(...).
type chain struct {
	pkg   string // import path of the constructor's package
	ctor  string
	args  []ast.Expr
	calls []call
	pos   token.Pos
}

// unwind turns a call expression into its chain. It fails when the chain does
// not start at a package-level constructor.
func unwind(fi *fileInfo, e ast.Expr) (*chain, bool) {
	c := &chain{pos: e.Pos()}
	for {
		ce, ok := e.(*ast.CallExpr)
		if !ok {
			return nil, false
		}
		sel, ok := ce.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil, false
		}
		if p := fi.pkg(sel.X); p != "" {
			c.pkg, c.ctor, c.args = p, sel.Sel.Name, ce.Args
			for i, j := 0, len(c.calls)-1; i < j; i, j = i+1, j-1 {
				c.calls[i], c.calls[j] = c.calls[j], c.calls[i]
			}
			return c, true
		}
		c.calls = append(c.calls, call{name: sel.Sel.Name, args: ce.Args, pos: sel.Sel.Pos()})
		e = sel.X
	}
}

func strLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	return v, err == nil
}

// pascal turns an Ent field or edge name into a Go field name.
func pascal(s string) string {
	var sb strings.Builder
	for _, part := range strings.Split(s, "_") {
		switch {
		case part == "":
		case strings.EqualFold(part, "id"):
			sb.WriteString("ID")
		default:
			sb.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return sb.String()
}

// fieldType is the Go and PostgreSQL type of one Ent field constructor, as
// dialect/sql/schema/postgres.go maps it: the string types are varchar
// without a length (MaxLen and Size are not applied), Text is text, the
// integer types follow the Go size (an unsigned type one size up), and an
// enum is a varchar.
type fieldType struct{ goType, sqlType string }

var fieldTypes = map[string]fieldType{
	"String": {"string", "varchar"}, "Text": {"string", "text"}, "Enum": {"string", "varchar"},
	"Bool": {"bool", "boolean"},
	"Int":  {"int", "bigint"}, "Int8": {"int8", "smallint"}, "Int16": {"int16", "smallint"},
	"Int32": {"int32", "integer"}, "Int64": {"int64", "bigint"},
	"Uint": {"uint", "bigint"}, "Uint8": {"uint8", "smallint"}, "Uint16": {"uint16", "integer"},
	"Uint32": {"uint32", "bigint"}, "Uint64": {"uint64", "bigint"},
	"Float": {"float64", "double precision"}, "Float32": {"float32", "real"},
	"Time": {"time.Time", "timestamptz"}, "Bytes": {"[]byte", "bytea"},
}

// validators run in Go inside Ent's builders.
var validators = map[string]bool{
	"MaxLen": true, "MinLen": true, "NotEmpty": true, "Match": true, "Validate": true,
	"Min": true, "Max": true, "Range": true, "Positive": true, "Negative": true,
	"NonNegative": true, "NonPositive": true,
}

// metadata methods do not change the schema or the generated Go behaviour
// the importer can see.
var metadata = map[string]bool{"Comment": true, "Sensitive": true, "StructTag": true, "Deprecated": true}

// fieldDef is one parsed Ent field.
type fieldDef struct {
	name, column, pos string
	ft                fieldType
	optional          bool
	nillable          bool
	unique            bool
	def, defKind      string // defKind is "db" or ""
	claimed           bool   // the field is the key of an edge
	ok                bool
}

// edgeDef is one parsed Ent edge.
type edgeDef struct {
	owner    *entity
	name     string
	target   string
	inverse  bool
	ref      string
	unique   bool
	required bool
	field    string
	column   string
	symbol   string
	onDelete string
	pos      token.Pos
	file     *fileInfo
	ok       bool
}

// indexDef is one parsed Ent index.
type indexDef struct {
	pos     token.Pos
	fields  []string
	edges   []string
	unique  bool
	storage string
}

type entity struct {
	ti      *typeInfo
	table   string
	fields  []*fieldDef
	edges   []*edgeDef
	indexes []*indexDef
}

func (e *entity) field(name string) *fieldDef {
	for _, f := range e.fields {
		if f.name == name {
			return f
		}
	}
	return nil
}

func (e *entity) edge(name string) *edgeDef {
	for _, ed := range e.edges {
		if ed.name == name {
			return ed
		}
	}
	return nil
}

// build reads every entity, resolves the edges to foreign keys and builds the
// models.
func (r *reader) build() ([]importer.Model, *importer.Report, error) {
	var ents []*entity
	byName := map[string]*entity{}
	for _, name := range r.order {
		ti := r.types[name]
		if ti.view {
			r.unsupported(ti.spec.Pos(), name, "", "an ent.View has no table; it is not imported")
			continue
		}
		if !ti.entity {
			continue
		}
		e := &entity{ti: ti}
		ents = append(ents, e)
		byName[name] = e
	}
	for _, e := range ents {
		r.readEntity(e)
	}
	fks := r.resolve(ents, byName)
	r.fks = fks
	var models []importer.Model
	for _, e := range ents {
		models = append(models, r.model(e, fks))
	}
	return models, r.rep, nil
}

// contributions are the types whose Fields, Edges and Indexes make up e: its
// mixins in order, then the type itself.
func (r *reader) contributions(e *entity) []*typeInfo {
	var out []*typeInfo
	for _, el := range r.sliceElems(e.ti, "Mixin") {
		cl, ok := el.expr.(*ast.CompositeLit)
		if !ok {
			r.unsupported(el.expr.Pos(), e.ti.name, "", "a mixin that is not a composite literal cannot be read")
			continue
		}
		switch t := cl.Type.(type) {
		case *ast.Ident:
			if mt := r.types[t.Name]; mt != nil && mt.mixin {
				out = append(out, mt)
				continue
			}
		case *ast.SelectorExpr:
			if el.file.pkg(t.X) == mixinPath && (t.Sel.Name == "Time" || t.Sel.Name == "CreateTime" || t.Sel.Name == "UpdateTime") {
				r.timeMixin(e, t.Sel.Name, el.expr.Pos())
				continue
			}
		}
		r.unsupported(el.expr.Pos(), e.ti.name, "", "the mixin "+exprName(cl.Type)+" is not a mixin type of this package or mixin.Time, mixin.CreateTime or mixin.UpdateTime")
	}
	return append(out, e.ti)
}

func exprName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprName(t.X) + "." + t.Sel.Name
	}
	return "expression"
}

// timeMixin adds the fields of mixin.Time, mixin.CreateTime or
// mixin.UpdateTime. They are time fields with a time.Now default, set in Go.
func (r *reader) timeMixin(e *entity, kind string, p token.Pos) {
	add := func(name, why string) {
		e.fields = append(e.fields, &fieldDef{name: name, column: name, pos: r.pos(p), ft: fieldTypes["Time"], ok: true})
		r.rep.Add(importer.Change{Kind: importer.GoDefault, Source: r.pos(p), Model: e.ti.name, Field: pascal(name),
			Message: "mixin." + kind + " sets " + name + " in Go (" + why + "); Momo needs a created_at or updated_at tag or a database default"})
	}
	if kind == "Time" || kind == "CreateTime" {
		add("created_at", "default time.Now when the row is created")
		r.rep.Add(importer.Change{Kind: importer.GoSideRule, Source: r.pos(p), Model: e.ti.name, Field: "CreatedAt",
			Message: "mixin." + kind + " makes created_at immutable in Ent's builders; Momo does not stop updates"})
	}
	if kind == "Time" || kind == "UpdateTime" {
		add("updated_at", "default and update default time.Now")
	}
}

// readEntity reads the table name, fields, edges and indexes of e.
func (r *reader) readEntity(e *entity) {
	name := e.ti.name
	e.table = namer.TableName(name)
	if m, ok := r.methods[name]["Config"]; ok {
		r.config(e, m)
	}
	r.annotations(e)
	for _, hook := range []string{"Hooks", "Policy", "Interceptors"} {
		if m, ok := r.methods[name][hook]; ok {
			r.rep.Add(importer.Change{Kind: importer.GoSideRule, Source: r.pos(m.decl.Pos()), Model: name,
				Message: hook + " runs in Go inside Ent's builders; it does not carry over to Momo"})
		}
	}
	for _, ti := range r.contributions(e) {
		for _, el := range r.sliceElems(ti, "Fields") {
			r.readField(e, el)
		}
		for _, el := range r.sliceElems(ti, "Edges") {
			r.readEdge(e, el)
		}
		for _, el := range r.sliceElems(ti, "Indexes") {
			r.readIndex(e, el)
		}
	}
}

// config reads Table from a Config method returning ent.Config{Table: "x"}.
func (r *reader) config(e *entity, m method) {
	if m.decl.Body != nil && len(m.decl.Body.List) == 1 {
		if ret, ok := m.decl.Body.List[0].(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
			if cl, ok := ret.Results[0].(*ast.CompositeLit); ok {
				for _, el := range cl.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if k, isID := kv.Key.(*ast.Ident); isID && k.Name == "Table" {
						if v, ok := strLit(kv.Value); ok {
							e.table = v
							return
						}
					}
				}
				return
			}
		}
	}
	r.unsupported(m.decl.Pos(), e.ti.name, "", "Config does not return an ent.Config literal, so its table name cannot be read")
}

// annotations reads entsql.Annotation{Table: ...} from the Annotations
// method; any other entsql key is reported.
func (r *reader) annotations(e *entity) {
	for _, el := range r.sliceElems(e.ti, "Annotations") {
		cl, ok := el.expr.(*ast.CompositeLit)
		if !ok || !el.file.is(cl.Type, entsqlPath, "Annotation") {
			continue // an annotation of another package does not change the schema
		}
		for _, kv := range cl.Elts {
			kve, ok := kv.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key := exprName(kve.Key)
			if key == "Table" {
				if v, ok := strLit(kve.Value); ok {
					e.table = v
					continue
				}
				r.unsupported(kve.Pos(), e.ti.name, "", "the entsql Table is not a string literal")
				continue
			}
			r.unsupported(kve.Pos(), e.ti.name, "", "the entsql annotation key "+key+" is not imported; only Table is")
		}
	}
}

func (r *reader) readField(e *entity, el elem) {
	model := e.ti.name
	c, ok := unwind(el.file, el.expr)
	if !ok || c.pkg != fieldPath {
		r.unsupported(el.expr.Pos(), model, "", "this field is not a field.<Type>(...) builder chain, so it cannot be read without running it")
		return
	}
	var name string
	if len(c.args) > 0 {
		name, _ = strLit(c.args[0])
	}
	if name == "" {
		r.unsupported(c.pos, model, "", "the field name of "+c.ctor+" is not a string literal")
		return
	}
	pos := r.pos(c.pos)
	ft, known := fieldTypes[c.ctor]
	fd := &fieldDef{name: name, column: namer.ColumnName(name), pos: pos, ft: ft}
	bad := func(p token.Pos, msg string) {
		r.unsupported(p, model, pascal(name), msg)
		fd.ok = false
	}
	if !known {
		r.unsupported(c.pos, model, pascal(name), "field."+c.ctor+" has no column type the importer maps (JSON, UUID, Other and the slice types are not imported); convert the field by hand")
		e.fields = append(e.fields, fd)
		return
	}
	if name == "id" {
		r.unsupported(c.pos, model, pascal(name), "a field named id replaces Ent's key; custom keys are not imported")
		e.fields = append(e.fields, fd)
		return
	}
	fd.ok = true
	var defArg ast.Expr
	var defCall *call
	for i := range c.calls {
		m := c.calls[i]
		switch {
		case m.name == "Optional":
			fd.optional = true
		case m.name == "Nillable":
			fd.nillable = true
		case m.name == "Unique":
			fd.unique = true
		case m.name == "Immutable":
			r.rep.Add(importer.Change{Kind: importer.GoSideRule, Source: r.pos(m.pos), Model: model, Field: pascal(name),
				Message: "Immutable removes the update setter in Ent's builders; Momo does not stop updates"})
		case m.name == "StorageKey":
			if len(m.args) == 1 {
				if v, ok := strLit(m.args[0]); ok {
					fd.column = v
					continue
				}
			}
			bad(m.pos, "StorageKey is not a string literal")
		case m.name == "Default":
			if len(m.args) == 1 {
				defArg, defCall = m.args[0], &c.calls[i]
				continue
			}
			bad(m.pos, "Default needs one argument")
		case m.name == "DefaultFunc" || m.name == "UpdateDefault":
			r.rep.Add(importer.Change{Kind: importer.GoDefault, Source: r.pos(m.pos), Model: model, Field: pascal(name),
				Message: "Ent calls the function of " + m.name + " in Go; Momo does not, so set the value yourself or add a created_at or updated_at tag or a database default"})
		case m.name == "SchemaType":
			r.schemaType(fd, e, el.file, m)
		case m.name == "Values" || m.name == "NamedValues":
			if c.ctor != "Enum" {
				bad(m.pos, m.name+" is only valid on field.Enum")
			}
			r.rep.Add(importer.Change{Kind: importer.GoSideRule, Source: r.pos(m.pos), Model: model, Field: pascal(name),
				Message: "Ent checks an enum value in Go against its list and stores a varchar with no CHECK constraint; Momo imports the column as a varchar and does not check the values"})
		case m.name == "Annotations":
			r.fieldAnnotations(fd, e, el.file, m)
		case validators[m.name]:
			r.rep.Add(importer.Change{Kind: importer.GoSideRule, Source: r.pos(m.pos), Model: model, Field: pascal(name),
				Message: m.name + " is a validator Ent runs in Go before it writes; it is not part of the schema and Momo does not run it"})
		case metadata[m.name]:
		default:
			bad(m.pos, "the Ent builder method "+m.name+" is not imported (GoType, ValueScanner and the like change the Go type); convert the field by hand")
		}
	}
	if defArg != nil {
		r.fieldDefault(fd, e, el.file, c.ctor, defArg, defCall)
	}
	e.fields = append(e.fields, fd)
}

// schemaType reads the postgres entry of a SchemaType map literal.
func (r *reader) schemaType(fd *fieldDef, e *entity, f *fileInfo, m call) {
	model, field := e.ti.name, pascal(fd.name)
	if len(m.args) == 1 {
		if cl, ok := m.args[0].(*ast.CompositeLit); ok {
			for _, el := range cl.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				v, vok := strLit(kv.Value)
				key := ""
				switch k := kv.Key.(type) {
				case *ast.SelectorExpr:
					if f.pkg(k.X) == dialectPath {
						key = strings.ToLower(k.Sel.Name)
					}
				case *ast.BasicLit:
					key, _ = strLit(k)
				}
				switch {
				case key == "postgres" && vok:
					fd.ft.sqlType = strings.ToLower(v)
				case key == "":
					r.unsupported(kv.Pos(), model, field, "a SchemaType key that is not a dialect constant or a string literal cannot be read")
					fd.ok = false
				case key != "postgres":
					r.rep.Add(importer.Change{Kind: importer.Naming, Source: r.pos(kv.Pos()), Model: model, Field: field,
						Message: "SchemaType for " + key + " is ignored; the import reads the postgres entry"})
				default:
					r.unsupported(kv.Pos(), model, field, "the postgres SchemaType is not a string literal")
					fd.ok = false
				}
			}
			return
		}
	}
	r.unsupported(m.pos, model, field, "SchemaType is not a map literal")
	fd.ok = false
}

// fieldAnnotations reads entsql.Default and entsql.DefaultExpr; any other
// entsql annotation is reported.
func (r *reader) fieldAnnotations(fd *fieldDef, e *entity, fi *fileInfo, m call) {
	model, field := e.ti.name, pascal(fd.name)
	for _, a := range m.args {
		switch a := a.(type) {
		case *ast.CallExpr:
			sel, ok := a.Fun.(*ast.SelectorExpr)
			if !ok || fi.pkg(sel.X) != entsqlPath {
				continue
			}
			v, vok := "", false
			if len(a.Args) == 1 {
				v, vok = strLit(a.Args[0])
			}
			switch {
			case sel.Sel.Name == "Default" && vok:
				fd.def, fd.defKind = "'"+strings.ReplaceAll(v, "'", "''")+"'", "db"
			case sel.Sel.Name == "DefaultExpr" && vok:
				fd.def, fd.defKind = v, "db"
			default:
				r.unsupported(a.Pos(), model, field, "entsql."+sel.Sel.Name+" is not imported; only Default and DefaultExpr with a string literal are")
				fd.ok = false
			}
		case *ast.CompositeLit:
			if !fi.is(a.Type, entsqlPath, "Annotation") {
				continue
			}
			for _, el := range a.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				v, vok := strLit(kv.Value)
				switch key := exprName(kv.Key); {
				case key == "Default" && vok:
					fd.def, fd.defKind = "'"+strings.ReplaceAll(v, "'", "''")+"'", "db"
				case key == "DefaultExpr" && vok:
					fd.def, fd.defKind = v, "db"
				default:
					r.unsupported(kv.Pos(), model, field, "the entsql annotation key "+key+" is not imported; only Default and DefaultExpr with a string literal are")
					fd.ok = false
				}
			}
		}
	}
}

// fieldDefault reads Default(x). A literal becomes the database default Ent
// writes into the column; time.Now is a Go function.
func (r *reader) fieldDefault(fd *fieldDef, e *entity, fi *fileInfo, ctor string, arg ast.Expr, m *call) {
	model, field := e.ti.name, pascal(fd.name)
	if fi.is(arg, "time", "Now") {
		r.rep.Add(importer.Change{Kind: importer.GoDefault, Source: r.pos(m.pos), Model: model, Field: field,
			Message: "Ent sets the default time.Now in Go; Momo needs a created_at or updated_at tag or a database default"})
		return
	}
	lit, ok := defaultLiteral(ctor, arg)
	if !ok {
		r.unsupported(m.pos, model, field, "the default is not a literal of the field's type that can be read without running it; use a literal, time.Now, or DefaultFunc")
		fd.ok = false
		return
	}
	if fd.defKind == "" { // an entsql default annotation wins, as in Ent
		fd.def, fd.defKind = lit, "db"
	}
	if !fd.nillable {
		r.rep.Add(importer.Change{Kind: importer.ZeroValueHandling, Source: r.pos(m.pos), Model: model, Field: field,
			Message: "Ent applies the default " + lit + " when the setter was not called and writes it into the column default; Momo keeps the column default but writes the zero value of a non-pointer field, so use a pointer type to leave it unset"})
	}
}

// defaultLiteral renders a literal Default argument the way Ent writes it into
// the column default: numbers and booleans as they print, strings quoted.
func defaultLiteral(ctor string, e ast.Expr) (string, bool) {
	neg := ""
	if u, ok := e.(*ast.UnaryExpr); ok && (u.Op == token.SUB || u.Op == token.ADD) {
		if u.Op == token.SUB {
			neg = "-"
		}
		e = u.X
	}
	switch v := e.(type) {
	case *ast.Ident:
		if ctor == "Bool" && neg == "" && (v.Name == "true" || v.Name == "false") {
			return v.Name, true
		}
	case *ast.BasicLit:
		switch {
		case v.Kind == token.STRING && neg == "" && (ctor == "String" || ctor == "Text" || ctor == "Enum"):
			s, err := strconv.Unquote(v.Value)
			if err != nil {
				return "", false
			}
			return "'" + strings.ReplaceAll(s, "'", "''") + "'", true
		case (v.Kind == token.INT || v.Kind == token.FLOAT) && strings.HasPrefix(ctor, "Float"):
			bits := 64
			if ctor == "Float32" {
				bits = 32
			}
			f, err := strconv.ParseFloat(strings.ReplaceAll(neg+v.Value, "_", ""), bits)
			if err != nil {
				return "", false
			}
			return strconv.FormatFloat(f, 'g', -1, bits), true
		case v.Kind == token.INT && (strings.HasPrefix(ctor, "Int") || strings.HasPrefix(ctor, "Uint")):
			n, err := strconv.ParseInt(neg+v.Value, 0, 64)
			if err != nil {
				return "", false
			}
			return strconv.FormatInt(n, 10), true
		}
	}
	return "", false
}

var deleteActions = map[string]string{"Cascade": "CASCADE", "NoAction": "NO ACTION", "Restrict": "RESTRICT", "SetNull": "SET NULL", "SetDefault": "SET DEFAULT"}

func (r *reader) readEdge(e *entity, el elem) {
	model := e.ti.name
	c, ok := unwind(el.file, el.expr)
	if !ok || c.pkg != edgePath || (c.ctor != "To" && c.ctor != "From") {
		r.unsupported(el.expr.Pos(), model, "", "this edge is not an edge.To or edge.From builder chain, so it cannot be read without running it")
		return
	}
	ed := &edgeDef{owner: e, inverse: c.ctor == "From", pos: c.pos, file: el.file, ok: true}
	if len(c.args) == 2 {
		ed.name, _ = strLit(c.args[0])
		if sel, ok := c.args[1].(*ast.SelectorExpr); ok && sel.Sel.Name == "Type" {
			if id, ok := sel.X.(*ast.Ident); ok {
				ed.target = id.Name
			}
		}
	}
	if ed.name == "" || ed.target == "" {
		r.unsupported(c.pos, model, "", "the edge name must be a string literal and its type a Name.Type selector")
		return
	}
	bad := func(p token.Pos, msg string) {
		r.unsupported(p, model, pascal(ed.name), msg)
		ed.ok = false
	}
	for _, m := range c.calls {
		switch m.name {
		case "Unique":
			ed.unique = true
		case "Required":
			ed.required = true
		case "Ref":
			if len(m.args) == 1 {
				if v, ok := strLit(m.args[0]); ok {
					ed.ref = v
					continue
				}
			}
			bad(m.pos, "Ref is not a string literal")
		case "Field":
			if len(m.args) == 1 {
				if v, ok := strLit(m.args[0]); ok {
					ed.field = v
					continue
				}
			}
			bad(m.pos, "Field is not a string literal")
		case "StorageKey":
			r.edgeStorageKey(ed, m, bad)
		case "Annotations":
			r.edgeAnnotations(ed, m, bad)
		case "Immutable":
			r.rep.Add(importer.Change{Kind: importer.GoSideRule, Source: r.pos(m.pos), Model: model, Field: pascal(ed.name),
				Message: "Immutable removes the setter of this edge in Ent's builders; Momo does not stop updates"})
		case "Comment", "Deprecated":
		default:
			bad(m.pos, "the Ent edge method "+m.name+" is not imported (Through is for many-to-many edges, which are not imported)")
		}
	}
	e.edges = append(e.edges, ed)
}

// edgeStorageKey reads edge.Column and edge.Symbol.
func (r *reader) edgeStorageKey(ed *edgeDef, m call, bad func(token.Pos, string)) {
	for _, a := range m.args {
		ce, ok := a.(*ast.CallExpr)
		if !ok || len(ce.Args) != 1 {
			bad(a.Pos(), "StorageKey takes edge.Column(name) and edge.Symbol(name) with string literals")
			continue
		}
		v, vok := strLit(ce.Args[0])
		sel, sok := ce.Fun.(*ast.SelectorExpr)
		switch {
		case !vok || !sok || ed.file.pkg(sel.X) != edgePath:
			bad(a.Pos(), "StorageKey takes edge.Column(name) and edge.Symbol(name) with string literals")
		case sel.Sel.Name == "Column":
			ed.column = v
		case sel.Sel.Name == "Symbol":
			ed.symbol = v
		default:
			bad(a.Pos(), "edge."+sel.Sel.Name+" in StorageKey is for many-to-many edges, which are not imported")
		}
	}
}

// edgeAnnotations reads entsql.OnDelete.
func (r *reader) edgeAnnotations(ed *edgeDef, m call, bad func(token.Pos, string)) {
	action := func(e ast.Expr) (string, bool) {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok || ed.file.pkg(sel.X) != entsqlPath {
			return "", false
		}
		v, ok := deleteActions[sel.Sel.Name]
		return v, ok
	}
	for _, a := range m.args {
		switch a := a.(type) {
		case *ast.CallExpr:
			sel, ok := a.Fun.(*ast.SelectorExpr)
			if !ok || ed.file.pkg(sel.X) != entsqlPath {
				continue
			}
			if v, ok := action(firstArg(a)); ok && sel.Sel.Name == "OnDelete" {
				ed.onDelete = v
				continue
			}
			bad(a.Pos(), "entsql."+sel.Sel.Name+" on an edge is not imported; only OnDelete with entsql.Cascade, NoAction, Restrict, SetNull or SetDefault is")
		case *ast.CompositeLit:
			if !ed.file.is(a.Type, entsqlPath, "Annotation") {
				continue
			}
			for _, el := range a.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if v, ok := action(kv.Value); ok && exprName(kv.Key) == "OnDelete" {
					ed.onDelete = v
					continue
				}
				bad(kv.Pos(), "the entsql annotation key "+exprName(kv.Key)+" on an edge is not imported; only OnDelete is")
			}
		}
	}
}

func firstArg(c *ast.CallExpr) ast.Expr {
	if len(c.Args) == 1 {
		return c.Args[0]
	}
	return c.Fun
}

func (r *reader) readIndex(e *entity, el elem) {
	model := e.ti.name
	c, ok := unwind(el.file, el.expr)
	if !ok || c.pkg != indexPath || (c.ctor != "Fields" && c.ctor != "Edges") {
		r.unsupported(el.expr.Pos(), model, "", "this index is not an index.Fields or index.Edges builder chain, so it cannot be read without running it")
		return
	}
	ix := &indexDef{pos: c.pos}
	names := func(args []ast.Expr) []string {
		var out []string
		for _, a := range args {
			v, ok := strLit(a)
			if !ok {
				r.unsupported(a.Pos(), model, "", "an index field or edge name is not a string literal")
				return nil
			}
			out = append(out, v)
		}
		return out
	}
	if c.ctor == "Fields" {
		ix.fields = names(c.args)
	} else {
		ix.edges = names(c.args)
	}
	for _, m := range c.calls {
		switch m.name {
		case "Unique":
			ix.unique = true
		case "Fields":
			ix.fields = append(ix.fields, names(m.args)...)
		case "Edges":
			ix.edges = append(ix.edges, names(m.args)...)
		case "StorageKey":
			if len(m.args) == 1 {
				if v, ok := strLit(m.args[0]); ok {
					ix.storage = v
					continue
				}
			}
			r.unsupported(m.pos, model, "", "the index StorageKey is not a string literal")
		default:
			r.unsupported(m.pos, model, "", "the Ent index method "+m.name+" is not imported (annotations such as Where, OpClass and Include change the index in ways Momo's tags cannot carry)")
		}
	}
	e.indexes = append(e.indexes, ix)
}

// fk is one foreign key Ent creates.
type fk struct {
	holder, ref *entity
	column      string
	symbol      string
	notNull     bool
	unique      bool
	onDelete    string
	edge        *edgeDef // the edge on the holder that carries the key, if any
	assoc       *edgeDef
	fieldName   string // the Go name of the key column
	relName     string // the Go name of the relation field
	claimed     *fieldDef
}

// resolve turns the edges into foreign keys, the way Graph.resolve and
// Graph.Tables of entc/gen do: the key lives on the side with the unique
// edge, its column is <owner type>_<edge name>, and its constraint is named
// <table>_<referenced table>_<edge name>.
func (r *reader) resolve(ents []*entity, byName map[string]*entity) []*fk {
	var out []*fk
	// every edge.From must point back at an edge.To that points at it
	for _, e := range ents {
		for _, ed := range e.edges {
			if !ed.inverse {
				continue
			}
			t := byName[ed.target]
			if t == nil {
				r.unsupported(ed.pos, e.ti.name, pascal(ed.name), "the edge targets "+ed.target+", which is not an Ent schema of this package")
				ed.ok = false
				continue
			}
			a := t.edge(ed.ref)
			if ed.ref == "" || a == nil || a.inverse || a.target != e.ti.name {
				r.unsupported(ed.pos, e.ti.name, pascal(ed.name), "edge.From needs Ref naming the edge.To of "+ed.target+" that points back at "+e.ti.name)
				ed.ok = false
			}
		}
	}
	for _, e := range ents {
		for _, a := range e.edges {
			if a.inverse || !a.ok {
				continue
			}
			t := byName[a.target]
			if t == nil {
				r.unsupported(a.pos, e.ti.name, pascal(a.name), "the edge targets "+a.target+", which is not an Ent schema of this package")
				continue
			}
			var inv *edgeDef
			for _, i := range t.edges {
				if i.inverse && i.ref == a.name && i.target == e.ti.name {
					if inv != nil {
						r.unsupported(i.pos, t.ti.name, pascal(i.name), "two edge.From edges refer to "+e.ti.name+"."+a.name)
					}
					inv = i
				}
			}
			if f := r.relate(e, t, a, inv); f != nil {
				out = append(out, f)
			}
		}
	}
	return out
}

// relate builds the foreign key of an edge.To a and its optional back
// reference inv.
func (r *reader) relate(owner, target *entity, a, inv *edgeDef) *fk {
	self := owner == target
	f := &fk{assoc: a}
	switch {
	case inv != nil && !a.unique && !inv.unique, inv == nil && !a.unique && self:
		r.unsupported(a.pos, owner.ti.name, pascal(a.name), "a many-to-many edge needs a join table, which is not imported; declare the join table by hand")
		if inv != nil {
			inv.ok = false
		}
		return nil
	case inv != nil && inv.required && a.required:
		r.unsupported(a.pos, owner.ti.name, pascal(a.name), "Ent rejects edges that are required in both directions")
		return nil
	case inv != nil && inv.unique: // the key is on the type that declares the unique back reference
		f.holder, f.ref, f.edge = target, owner, inv
		f.notNull = !self && inv.required
		f.unique = a.unique
	case inv != nil: // a unique, inv not: the key is on the owner of a
		f.holder, f.ref, f.edge = owner, target, a
		f.notNull = !self && a.required
	case a.unique:
		f.holder, f.ref, f.edge = owner, target, a
		f.notNull = !self && a.required
		f.unique = self
	default: // a non-unique edge.To without a back reference: the key is on the target
		f.holder, f.ref = target, owner
	}
	if inv != nil && inv.onDelete != "" {
		r.unsupported(inv.pos, inv.owner.ti.name, pascal(inv.name), "Ent reads entsql.OnDelete from the edge.To side, so the action here would be ignored by Ent; move it to the edge.To "+owner.ti.name+"."+a.name)
	}
	// column and symbol
	f.column = namer.ColumnName(owner.ti.name) + "_" + namer.ColumnName(a.name)
	f.symbol = f.holder.table + "_" + f.ref.table + "_" + a.name
	customColumn := ""
	for _, ed := range []*edgeDef{a, inv} {
		if ed == nil {
			continue
		}
		if ed.column != "" {
			if customColumn != "" && customColumn != ed.column {
				r.unsupported(ed.pos, ed.owner.ti.name, pascal(ed.name), "StorageKey names a different column on each end of the edge")
			}
			customColumn, f.column = ed.column, ed.column
		}
		if ed.symbol != "" {
			f.symbol = ed.symbol
		}
	}
	for _, ed := range []*edgeDef{a, inv} {
		if ed != nil && ed.field != "" && ed != f.edge {
			r.unsupported(ed.pos, ed.owner.ti.name, pascal(ed.name), "Field names the key column, which this edge does not hold; set it on the unique edge that holds the key")
		}
	}
	if f.edge != nil && f.edge.field != "" {
		r.edgeField(f)
	}
	f.onDelete = a.onDelete
	switch {
	case f.onDelete == "NO ACTION":
		f.onDelete = ""
	case f.onDelete == "" && !f.notNull:
		f.onDelete = "SET NULL"
	}
	f.fieldName = pascal(f.column)
	if f.claimed != nil {
		f.fieldName = pascal(f.claimed.name)
	}
	f.relName = f.ref.ti.name
	if f.edge != nil {
		f.relName = pascal(f.edge.name)
	}
	return f
}

// edgeField ties the key to a field declared on the holder.
func (r *reader) edgeField(f *fk) {
	ed := f.edge
	fd := f.holder.field(ed.field)
	switch {
	case fd == nil:
		r.unsupported(ed.pos, f.holder.ti.name, pascal(ed.name), "Field names "+ed.field+", which is not a field of "+f.holder.ti.name)
	case fd.ft.goType != "int":
		r.unsupported(ed.pos, f.holder.ti.name, pascal(ed.name), "the edge field "+ed.field+" must be field.Int, the type of Ent's key")
	case fd.optional == ed.required:
		r.unsupported(ed.pos, f.holder.ti.name, pascal(ed.name), "Ent requires the edge field "+ed.field+" to be Optional exactly when the edge is not Required")
	case fd.unique && !f.unique:
		r.unsupported(ed.pos, f.holder.ti.name, pascal(ed.name), "Ent ignores Unique on the edge field "+ed.field+" unless the edge is one-to-one; remove it")
	default:
		fd.claimed = true
		f.claimed = fd
		if f.column == namer.ColumnName(f.assoc.owner.ti.name)+"_"+namer.ColumnName(f.assoc.name) {
			f.column = fd.column
		}
	}
}

// model builds the importer model of e.
func (r *reader) model(e *entity, fks []*fk) importer.Model {
	name := e.ti.name
	m := importer.Model{Name: name, Table: e.table, Source: r.pos(e.ti.spec.Pos())}
	m.Fields = append(m.Fields, importer.Field{Name: "ID", Column: "id", GoType: "int", SQLType: "bigint", PK: true, Identity: true, Source: m.Source})
	byColumn := map[string]int{}
	byColumn["id"] = 0
	taken := map[string]bool{"ID": true}
	add := func(f importer.Field) {
		if taken[f.Name] {
			r.unsupportedAt(f.Source, name, f.Name, "the Go field name "+f.Name+" is used twice in "+name+"; rename one field or edge")
			return
		}
		taken[f.Name] = true
		byColumn[f.Column] = len(m.Fields)
		m.Fields = append(m.Fields, f)
	}
	// the foreign keys this model holds, by claimed field
	holds := map[*fieldDef]*fk{}
	for _, f := range fks {
		if f.holder == e && f.claimed != nil {
			holds[f.claimed] = f
		}
	}
	for _, fd := range e.fields {
		if !fd.ok {
			continue
		}
		if f, ok := holds[fd]; ok {
			add(r.keyField(e, f, fd.pos))
			continue
		}
		if fd.claimed {
			continue
		}
		add(r.column(e, fd))
	}
	for _, f := range fks {
		if f.holder != e {
			continue
		}
		pos := r.pos(f.assoc.pos)
		if f.edge != nil {
			pos = r.pos(f.edge.pos)
		}
		if f.claimed == nil {
			add(r.keyField(e, f, pos))
		}
		fkd := importer.ForeignKey{
			Name: f.symbol, Field: f.relName, RefModel: f.ref.ti.name, FKField: f.fieldName,
			Columns: []string{f.column}, RefTable: f.ref.table, RefColumns: []string{"id"}, OnDelete: f.onDelete,
		}
		if taken[f.relName] {
			r.unsupportedAt(pos, name, f.relName, "the relation field "+f.relName+" for the foreign key "+f.symbol+" collides with another field of "+name)
			continue
		}
		taken[f.relName] = true
		m.ForeignKeys = append(m.ForeignKeys, fkd)
		r.rep.Add(importer.Change{Kind: importer.Naming, Source: pos, Model: name, Field: f.relName,
			Message: "foreign key " + f.symbol + " is named " + e.table + "_" + f.column + "_fkey by Momo"})
		if f.edge == nil {
			r.rep.Add(importer.Change{Kind: importer.Naming, Source: pos, Model: name, Field: f.relName,
				Message: "Ent keeps the key " + f.column + " of " + f.assoc.owner.ti.name + "." + f.assoc.name + " on " + e.table + " without an edge there; the import adds a belongs-to field " + f.relName + " to " + name + " to keep the constraint"})
		}
	}
	for _, ed := range e.edges {
		if !ed.ok {
			continue
		}
		r.rep.Add(importer.Change{Kind: importer.ImplicitSave, Source: r.pos(ed.pos), Model: name, Field: pascal(ed.name),
			Message: "Ent writes the edge " + ed.name + " only when its setter is called (Set" + pascal(ed.name) + " or Add...); a Momo model holds the key field and the relation field is not saved with " + name + ", so set the key yourself"})
	}
	r.indexes(e, &m, byColumn)
	return m
}

// column turns an Ent field into a model field.
func (r *reader) column(e *entity, fd *fieldDef) importer.Field {
	goType := fd.ft.goType
	if fd.nillable {
		goType = "*" + goType
	}
	f := importer.Field{
		Name: pascal(fd.name), Column: fd.column, GoType: goType, SQLType: fd.ft.sqlType,
		Nullable: fd.optional, Default: fd.def, DefaultKind: fd.defKind, Source: fd.pos,
	}
	if fd.nillable && !fd.optional {
		r.unsupportedAt(fd.pos, e.ti.name, f.Name, "Nillable without Optional: Momo reads nullability from the Go type, so the pointer would make the column nullable; add Optional or drop Nillable")
	}
	if fd.unique {
		f.Unique, f.UniqueIndex = true, true
		f.UniqueName = e.table + "_" + fd.column + "_key"
	}
	return f
}

// keyField is the model field that holds a foreign key.
func (r *reader) keyField(e *entity, f *fk, pos string) importer.Field {
	kf := importer.Field{Name: f.fieldName, Column: f.column, GoType: "int", SQLType: "bigint", Nullable: !f.notNull, Source: pos}
	if !f.notNull {
		kf.GoType = "*int"
	}
	if f.unique {
		kf.Unique, kf.UniqueIndex = true, true
		kf.UniqueName = e.table + "_" + f.column + "_key"
	}
	if f.claimed != nil && f.claimed.defKind == "db" {
		kf.Default, kf.DefaultKind = f.claimed.def, "db"
	}
	return kf
}

// indexes applies the Ent indexes to the fields of m. A Momo tag names one
// index per field, so composite indexes and a second index on one field are
// reported and dropped.
func (r *reader) indexes(e *entity, m *importer.Model, byColumn map[string]int) {
	model := e.ti.name
	for _, ix := range e.indexes {
		var cols []string
		for _, n := range ix.fields {
			switch fd := e.field(n); {
			case n == "id":
				cols = append(cols, "id")
			case fd == nil:
				r.unsupported(ix.pos, model, "", "the index names the field "+n+", which is not a field of "+model)
			default:
				cols = append(cols, fd.column)
			}
		}
		for _, n := range ix.edges {
			col := ""
			for _, f := range r.keysOf(e, n) {
				col = f
			}
			if col == "" {
				r.unsupported(ix.pos, model, "", "the index names the edge "+n+", which is not a unique edge that holds a foreign key of "+model)
				continue
			}
			cols = append(cols, col)
		}
		if len(cols) == 0 {
			continue
		}
		name := ix.storage
		if name == "" {
			name = strings.ToLower(model) + "_" + strings.Join(cols, "_")
		}
		pos := r.pos(ix.pos)
		if len(cols) > 1 {
			r.rep.Add(importer.Change{Kind: importer.DroppedIndex, Source: pos, Model: model, Message: "index " + name + " spans " + strings.Join(cols, ", ") + "; composite indexes are not imported, declare it in the companion file"})
			continue
		}
		i, ok := byColumn[cols[0]]
		if !ok {
			r.unsupported(ix.pos, model, "", "the index column "+cols[0]+" has no field in the imported model")
			continue
		}
		f := &m.Fields[i]
		if f.Index || f.Unique || f.UniqueIndex {
			r.rep.Add(importer.Change{Kind: importer.DroppedIndex, Source: pos, Model: model, Field: f.Name, Message: "the column " + cols[0] + " already carries an index in the model; Momo's tags name one index per field, so index " + name + " is dropped"})
			continue
		}
		if ix.unique {
			f.Unique, f.UniqueIndex, f.UniqueName = true, true, name
		} else {
			f.Index, f.IndexName = true, name
		}
	}
}

// keysOf returns the key column of the unique edge called name of e, if it
// holds a foreign key.
func (r *reader) keysOf(e *entity, name string) []string {
	var cols []string
	for _, ed := range e.edges {
		if ed.name != name || !ed.unique {
			continue
		}
		for _, f := range r.fks {
			if f.holder == e && f.edge == ed {
				cols = append(cols, f.column)
			}
		}
	}
	return cols
}
