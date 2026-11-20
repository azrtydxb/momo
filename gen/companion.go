// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/azrtydxb/momo/schema"
)

const schemaPkgPath = "github.com/azrtydxb/momo/schema"

// CompanionResult is the statically read MomoConfig of a model. Maps are keyed
// by column name; they are only read by key.
type CompanionResult struct {
	Indexes []schema.Index
	Uniques []schema.Unique
	// ForeignKeys are the foreign keys declared with Config.ForeignKey.
	ForeignKeys []schema.ForeignKey
	// Excludes are the exclusion constraints declared with Config.Exclude.
	Excludes []schema.Exclude
	Checks   []schema.Check
	// CheckIns are the value-list checks declared with Config.CheckIn.
	CheckIns []CheckInDecl
	Enums    []schema.Enum
	// Sequences are the standalone sequences declared with Config.Sequence.
	Sequences []schema.Sequence
	// Domains and Composites are the PostgreSQL domains and composite types
	// declared with Config.Domain and Config.Composite.
	Domains    []schema.Domain
	Composites []schema.Composite
	Defaults   map[string]string
	// Generated maps a column to the expression set by a Generated call.
	Generated map[string]string
	// GeneratedVirtual marks the columns declared with a GeneratedVirtual call;
	// their expression is in Generated.
	GeneratedVirtual map[string]bool
	Comments         map[string]string
	Collates         map[string]string
	// IDGens maps a column to the id generator named by an IDGen call.
	IDGens map[string]string
	// FKNames maps a foreign key column to the constraint name set by FKName.
	FKNames map[string]string
	// PrimaryKey is the column order set by PrimaryKey, empty when not called.
	PrimaryKey []string
	// PrimaryKeyDeferrable and PrimaryKeyInitiallyDeferred are set by the
	// PrimaryKeyDeferrable and PrimaryKeyInitiallyDeferred calls; the second
	// implies the first.
	PrimaryKeyDeferrable, PrimaryKeyInitiallyDeferred bool
	// History is set by the History call: the model keeps an audit history
	// table, filled by a trigger.
	History bool

	Views       []schema.View
	Functions   []schema.Function
	Triggers    []schema.Trigger
	Policies    []schema.Policy
	RLS         *schema.RLS
	Partition   *schema.Partition
	PartitionOf *schema.PartitionOf
}

// CheckInDecl is one value-list check read from a Config.CheckIn call: the
// column, the values in source order and the constraint name, which is
// <table>_<column>_check unless the declaration names it.
type CheckInDecl struct {
	Name   string
	Column string
	Values []string
}

// structuredTypes maps the structured companion methods to their schema type.
var structuredTypes = map[string]reflect.Type{
	"View":        reflect.TypeOf(schema.View{}),
	"Function":    reflect.TypeOf(schema.Function{}),
	"Trigger":     reflect.TypeOf(schema.Trigger{}),
	"Policy":      reflect.TypeOf(schema.Policy{}),
	"RLS":         reflect.TypeOf(schema.RLS{}),
	"Partition":   reflect.TypeOf(schema.Partition{}),
	"PartitionOf": reflect.TypeOf(schema.PartitionOf{}),
}

// readCompanion reads the MomoConfig method of m without executing it. A
// model without the method has an empty result. Anything the reader does not
// accept is a positioned diagnostic; it never evaluates code and never guesses.
func readCompanion(pkg *packages.Package, m *Model) (*CompanionResult, []Diagnostic) {
	r := &companionReader{
		pkg: pkg,
		m:   m,
		res: &CompanionResult{
			Defaults:         map[string]string{},
			Generated:        map[string]string{},
			GeneratedVirtual: map[string]bool{},
			Comments:         map[string]string{},
			Collates:         map[string]string{},
			IDGens:           map[string]string{},
			FKNames:          map[string]string{},
		},
		byPath:  map[string]*Field{},
		indexAt: map[string]string{},
		checkAt: map[string]string{},
		enumAt:  map[string]string{},
		fkAt:    map[string]string{},
		seqAt:   map[string]string{},
		typeAt:  map[string]string{},
	}
	for _, f := range m.Fields {
		r.byPath[pathKey(f.Path)] = f
	}
	r.seedTagNames()
	fn := r.findMethod()
	if fn == nil || fn.Body == nil {
		return r.res, nil
	}
	if !r.bindParams(fn) {
		return r.res, r.diags
	}
	for _, s := range fn.Body.List {
		r.stmt(s)
	}
	return r.res, r.diags
}

type companionReader struct {
	pkg    *packages.Package
	m      *Model
	res    *CompanionResult
	diags  []Diagnostic
	recv   types.Object // nil when the receiver is unnamed
	param  types.Object // nil when the parameter is unnamed
	byPath map[string]*Field
	// optSeen holds the options already applied to the index being read.
	optSeen map[string]bool
	// indexAt, checkAt and enumAt record where each name comes from: a tag
	// description for tag-produced names, a file:line for companion names.
	indexAt, checkAt, enumAt, fkAt, seqAt map[string]string
	// typeAt records the domains and composite types, which share one name
	// space in a schema.
	typeAt map[string]string
}

func pathKey(p []int) string {
	var b strings.Builder
	for _, i := range p {
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('.')
	}
	return b.String()
}

func (r *companionReader) diag(pos token.Pos, msg string) {
	r.diags = append(r.diags, Diagnostic{Pos: r.pkg.Fset.Position(pos), Msg: msg})
}

// unsupported reports the generic diagnostic for node n.
func (r *companionReader) unsupported(n ast.Node) {
	r.diag(n.Pos(), "unsupported companion expression: "+r.source(n))
}

// source renders n as one line of source text.
func (r *companionReader) source(n ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, r.pkg.Fset, n); err != nil {
		return "<unprintable>"
	}
	first, _, multi := strings.Cut(buf.String(), "\n")
	first = strings.TrimSpace(first)
	if multi {
		first += " ..."
	}
	return first
}

// seedTagNames records the index and check names already produced from tags,
// so an explicit companion name that repeats one is reported.
func (r *companionReader) seedTagNames() {
	for _, ix := range r.m.Indexes {
		r.indexAt[ix.Name] = "a mixin index tag"
	}
	for _, f := range r.m.Fields {
		if f.Tags.Check != "" {
			r.checkAt[r.m.Table+"_"+f.Column+"_check"] = "the check tag on field " + f.GoName
		}
		if len(f.Path) != 1 {
			continue // flattened fields are already in m.Indexes
		}
		if f.Tags.Index {
			name := f.Tags.IndexName
			if name == "" {
				name = r.m.Table + "_" + f.Column + "_idx"
			}
			r.indexAt[name] = "the index tag on field " + f.GoName
		}
		if f.Tags.Unique {
			name := f.Tags.UniqueName
			if name == "" {
				name = r.m.Table + "_" + f.Column + "_key"
			}
			r.indexAt[name] = "the unique tag on field " + f.GoName
		}
	}
}

// findMethod returns the MomoConfig method declaration of the model.
func (r *companionReader) findMethod() *ast.FuncDecl {
	for _, file := range r.pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "MomoConfig" {
				continue
			}
			obj, _ := r.pkg.TypesInfo.Defs[fn.Name].(*types.Func)
			if obj == nil {
				continue
			}
			sig, _ := obj.Type().(*types.Signature)
			if sig == nil || sig.Recv() == nil {
				continue
			}
			t := types.Unalias(sig.Recv().Type())
			if p, ok := t.(*types.Pointer); ok {
				t = types.Unalias(p.Elem())
			}
			n, ok := t.(*types.Named)
			if ok && n.Obj().Name() == r.m.GoName && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == r.m.PkgPath {
				return fn
			}
		}
	}
	return nil
}

// bindParams checks the signature and resolves the receiver and parameter
// objects. It reports false when the signature is not func(*momo.Config).
func (r *companionReader) bindParams(fn *ast.FuncDecl) bool {
	obj, _ := r.pkg.TypesInfo.Defs[fn.Name].(*types.Func)
	sig := obj.Type().(*types.Signature)
	if sig.Params().Len() != 1 || sig.Variadic() || sig.Results().Len() != 0 || !isMomoNamedPtr(sig.Params().At(0).Type(), "Config") {
		r.diag(fn.Name.Pos(), "MomoConfig must take exactly one *momo.Config parameter and return nothing")
		return false
	}
	if names := fn.Recv.List[0].Names; len(names) == 1 && names[0].Name != "_" {
		r.recv = r.pkg.TypesInfo.Defs[names[0]]
	}
	if names := fn.Type.Params.List[0].Names; len(names) == 1 && names[0].Name != "_" {
		r.param = r.pkg.TypesInfo.Defs[names[0]]
	}
	return true
}

func isMomoNamedPtr(t types.Type, name string) bool {
	p, ok := types.Unalias(t).(*types.Pointer)
	if !ok {
		return false
	}
	n, ok := types.Unalias(p.Elem()).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == momoPkgPath && n.Obj().Name() == name
}

// call is one link of a method chain.
type call struct {
	name string
	expr *ast.CallExpr
	sel  *ast.SelectorExpr
}

// stmt reads one statement of the method body. The generator must never
// panic on user input, so an unexpected panic becomes a positioned diagnostic.
func (r *companionReader) stmt(s ast.Stmt) {
	defer func() {
		if rec := recover(); rec != nil {
			r.diag(s.Pos(), fmt.Sprintf("internal error reading companion expression %s: %v", r.source(s), rec))
		}
	}()
	r.readStmt(s)
}

func (r *companionReader) readStmt(s ast.Stmt) {
	es, ok := s.(*ast.ExprStmt)
	if !ok {
		r.unsupported(s)
		return
	}
	calls, bad := r.chain(es.X)
	if bad != nil {
		r.unsupported(bad)
		return
	}
	// The receiver of the first call is the Config parameter; every later
	// call is on the IndexOpts returned by the one before it.
	for i, c := range calls {
		want := "IndexOpts"
		switch calls[0].name {
		case "ForeignKey":
			want = "ForeignKeyOpts"
		case "Exclude":
			want = "ExcludeOpts"
		case "FullText":
			want = "FullTextOpts"
		case "Sequence":
			want = "SequenceOpts"
		case "Domain":
			want = "DomainOpts"
		case "Composite":
			want = "CompositeOpts"
		case "CheckIn":
			want = "CheckInOpts"
		}
		if i == 0 {
			want = "Config"
		}
		if r.recvName(c.sel) != want {
			r.unsupported(es.X)
			return
		}
	}
	first := calls[0]
	switch first.name {
	case "Index", "Unique", "IndexExpr":
		r.index(calls)
		return
	case "FullText":
		r.fullText(calls)
		return
	case "CheckIn":
		r.checkIn(calls)
		return
	case "ForeignKey":
		r.foreignKey(calls)
		return
	case "Exclude":
		r.exclude(calls)
		return
	case "Sequence":
		r.sequence(calls)
		return
	case "Domain":
		r.domain(calls)
		return
	case "Composite":
		r.composite(calls)
		return
	}
	if len(calls) > 1 {
		r.unsupported(es.X) // only index and foreign key methods return options
		return
	}
	r.config(first)
}

// chain unwraps c.A(...).B(...) into its calls in source order. The root must
// be the Config parameter. bad is the node to report when it is not a chain.
func (r *companionReader) chain(e ast.Expr) (calls []call, bad ast.Node) {
	for {
		ce, ok := e.(*ast.CallExpr)
		if !ok {
			return nil, e
		}
		sel, ok := ce.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil, e
		}
		calls = append([]call{{name: sel.Sel.Name, expr: ce, sel: sel}}, calls...)
		if id, ok := sel.X.(*ast.Ident); ok {
			if r.param == nil || r.pkg.TypesInfo.Uses[id] != r.param {
				return nil, e
			}
			return calls, nil
		}
		e = sel.X
	}
}

// recvName returns the momo type name that declares the selected method, or
// "" when the selection does not resolve to a momo method.
func (r *companionReader) recvName(sel *ast.SelectorExpr) string {
	s := r.pkg.TypesInfo.Selections[sel]
	if s == nil || s.Kind() != types.MethodVal {
		return ""
	}
	fn, ok := s.Obj().(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != momoPkgPath {
		return ""
	}
	sig, _ := fn.Type().(*types.Signature)
	if sig == nil || sig.Recv() == nil {
		return ""
	}
	t := types.Unalias(sig.Recv().Type())
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	if n, ok := t.(*types.Named); ok {
		return n.Obj().Name()
	}
	return ""
}

// str accepts a plain string literal.
func (r *companionReader) str(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if ok && lit.Kind == token.STRING {
		if s, err := strconv.Unquote(lit.Value); err == nil {
			return s, true
		}
	}
	r.unsupported(e)
	return "", false
}

// field accepts &<receiver>.<Field> naming a persisted field of the model.
func (r *companionReader) field(e ast.Expr) (*Field, bool) {
	un, ok := e.(*ast.UnaryExpr)
	if !ok || un.Op != token.AND {
		r.unsupported(e)
		return nil, false
	}
	sel, ok := un.X.(*ast.SelectorExpr)
	if !ok {
		r.unsupported(e)
		return nil, false
	}
	path, ok := r.selectorPath(sel)
	if !ok {
		r.unsupported(e)
		return nil, false
	}
	if path != nil {
		if f := r.byPath[pathKey(path)]; f != nil {
			return f, true
		}
	}
	r.diag(e.Pos(), fmt.Sprintf("field reference %s is not a persisted field of %s", r.source(e), r.m.GoName))
	return nil, false
}

// selectorPath walks a selector chain from the receiver and returns the
// concatenated field index path, so &r.Geo.Lon and the promoted &r.Lon name
// the same field. ok is false when the chain is not rooted at the receiver;
// a nil path with ok true means a root is the receiver but a link is not a
// plain field selection.
func (r *companionReader) selectorPath(sel *ast.SelectorExpr) (path []int, ok bool) {
	var inner []int
	switch x := sel.X.(type) {
	case *ast.Ident:
		if r.recv == nil || r.pkg.TypesInfo.Uses[x] != r.recv {
			return nil, false
		}
	case *ast.SelectorExpr:
		p, ok := r.selectorPath(x)
		if !ok {
			return nil, false
		}
		if p == nil {
			return nil, true
		}
		inner = p
	default:
		return nil, false
	}
	s := r.pkg.TypesInfo.Selections[sel]
	if s == nil || s.Kind() != types.FieldVal {
		return nil, true
	}
	return append(append([]int(nil), inner...), s.Index()...), true
}

// declare records a name in a namespace and reports a clash.
func (r *companionReader) declare(at map[string]string, kind string, name string, pos token.Pos) bool {
	if prev, dup := at[name]; dup {
		if strings.HasPrefix(prev, "the ") || strings.HasPrefix(prev, "a ") {
			r.diag(pos, fmt.Sprintf("%s name %q is already produced by %s", kind, name, prev))
		} else {
			r.diag(pos, fmt.Sprintf("%s name %q is already declared at %s", kind, name, prev))
		}
		return false
	}
	p := r.pkg.Fset.Position(pos)
	at[name] = filepath.Base(p.Filename) + ":" + strconv.Itoa(p.Line)
	return true
}

func (r *companionReader) index(calls []call) {
	first := calls[0]
	args := first.expr.Args
	if len(args) < 2 {
		r.unsupported(first.expr)
		return
	}
	startDiags := len(r.diags)
	r.optSeen = map[string]bool{}
	name, _ := r.str(args[0])
	ix := schema.Index{Name: name, Unique: first.name == "Unique"}
	for _, a := range args[1:] {
		if first.name == "IndexExpr" {
			if s, ok := r.str(a); ok {
				ix.Parts = append(ix.Parts, schema.IndexPart{Expr: s})
			}
		} else if f, ok := r.field(a); ok {
			ix.Parts = append(ix.Parts, schema.IndexPart{Column: f.Column})
		}
	}
	var deferrable, initially bool
	options := 0
	for _, c := range calls[1:] {
		if c.name != "Deferrable" && c.name != "InitiallyDeferred" {
			options++
			r.indexOption(&ix, c)
			continue
		}
		if len(c.expr.Args) != 0 {
			r.unsupported(c.expr)
			continue
		}
		if r.optSeen[c.name] {
			r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s on index %q", c.name, name))
			continue
		}
		r.optSeen[c.name] = true
		deferrable = true
		initially = initially || c.name == "InitiallyDeferred"
	}
	if deferrable && len(r.diags) == startDiags {
		switch {
		case first.name != "Unique":
			r.diag(first.expr.Pos(), fmt.Sprintf("index %q: only a unique constraint, declared with Config.Unique, can be deferrable; PostgreSQL cannot defer an index", name))
		case options > 0:
			r.diag(first.expr.Pos(), fmt.Sprintf("unique %q: a deferrable unique constraint cannot take other options (Where, Method, With, Opclass, Desc, Nulls, Concurrently, Unique): those make it an index, and PostgreSQL cannot defer an index", name))
		}
	}
	if len(r.diags) != startDiags {
		return
	}
	if !r.declare(r.indexAt, "index", name, args[0].Pos()) {
		return
	}
	if first.name == "Unique" && options == 0 {
		cols := make([]string, len(ix.Parts))
		for i, p := range ix.Parts {
			cols[i] = p.Column
		}
		r.res.Uniques = append(r.res.Uniques, schema.Unique{Name: name, Columns: cols, Deferrable: deferrable, InitiallyDeferred: initially})
		return
	}
	// A unique with options is a unique index: constraints cannot be partial.
	r.res.Indexes = append(r.res.Indexes, ix)
}

// fullText reads c.FullText(name, &m.A, &m.B) with its optional Config,
// Weights, Tokenizer, Parser, Expression and Concurrently.
func (r *companionReader) fullText(calls []call) {
	first := calls[0]
	args := first.expr.Args
	if len(args) < 2 {
		r.unsupported(first.expr)
		return
	}
	startDiags := len(r.diags)
	r.optSeen = map[string]bool{}
	name, _ := r.str(args[0])
	ft := schema.FullText{}
	ix := schema.Index{Name: name, Method: "fulltext"}
	for _, a := range args[1:] {
		if f, ok := r.field(a); ok {
			ix.Parts = append(ix.Parts, schema.IndexPart{Column: f.Column})
			ft.Columns = append(ft.Columns, f.Column)
		}
	}
	once := func(c call, key string, dst *string, v string) {
		if r.optSeen[key] {
			r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s on full-text index %q", c.name, name))
			return
		}
		r.optSeen[key] = true
		*dst = v
	}
	for _, c := range calls[1:] {
		cargs := c.expr.Args
		switch c.name {
		case "Config", "Tokenizer", "Parser":
			if len(cargs) != 1 {
				r.unsupported(c.expr)
				continue
			}
			s, ok := r.str(cargs[0])
			if !ok {
				continue
			}
			switch c.name {
			case "Config":
				once(c, "Config", &ft.Config, s)
			case "Tokenizer":
				once(c, "Tokenizer", &ft.Tokenizer, s)
			default:
				once(c, "Parser", &ft.Parser, s)
			}
		case "Weights":
			if r.optSeen["Weights"] {
				r.diag(c.expr.Pos(), fmt.Sprintf("duplicate Weights on full-text index %q", name))
				continue
			}
			r.optSeen["Weights"] = true
			for _, a := range cargs {
				if s, ok := r.str(a); ok {
					ft.Weights = append(ft.Weights, s)
				}
			}
		case "Expression":
			if len(cargs) != 0 {
				r.unsupported(c.expr)
				continue
			}
			if ft.Expression {
				r.diag(c.expr.Pos(), fmt.Sprintf("duplicate Expression on full-text index %q", name))
				continue
			}
			ft.Expression = true
		case "Concurrently":
			if len(cargs) != 0 {
				r.unsupported(c.expr)
				continue
			}
			ix.Concurrently = true
		default:
			r.unsupported(c.expr)
		}
	}
	if len(r.diags) != startDiags {
		return
	}
	if !r.declare(r.indexAt, "index", name, args[0].Pos()) {
		return
	}
	ix.FullText = &ft
	r.res.Indexes = append(r.res.Indexes, ix)
}

// sequence reads c.Sequence(name) with its optional Schema, As, Start,
// Increment, MinValue, MaxValue, Cache and Cycle.
func (r *companionReader) sequence(calls []call) {
	first := calls[0]
	if len(first.expr.Args) != 1 {
		r.unsupported(first.expr)
		return
	}
	startDiags := len(r.diags)
	name, _ := r.str(first.expr.Args[0])
	q := schema.Sequence{Name: name}
	seen := map[string]bool{}
	for _, c := range calls[1:] {
		if seen[c.name] {
			r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s on sequence %q", c.name, name))
			continue
		}
		seen[c.name] = true
		args := c.expr.Args
		switch c.name {
		case "Cycle":
			if len(args) != 0 {
				r.unsupported(c.expr)
				continue
			}
			q.Cycle = true
		case "Schema", "As":
			if len(args) != 1 {
				r.unsupported(c.expr)
				continue
			}
			s, ok := r.str(args[0])
			if !ok {
				continue
			}
			if c.name == "Schema" {
				q.Schema = s
			} else {
				q.AsType = s
			}
		case "Start", "Increment", "MinValue", "MaxValue", "Cache":
			if len(args) != 1 {
				r.unsupported(c.expr)
				continue
			}
			n, ok := r.int64Lit(args[0])
			if !ok {
				continue
			}
			switch c.name {
			case "Start":
				q.Start = &n
			case "Increment":
				q.Increment = n
			case "MinValue":
				q.MinValue = &n
			case "MaxValue":
				q.MaxValue = &n
			default:
				q.Cache = n
			}
		default:
			r.unsupported(c.expr)
		}
	}
	if len(r.diags) != startDiags {
		return
	}
	if q.Name == "" {
		r.diag(first.expr.Pos(), "a sequence needs a name")
		return
	}
	if seen["Increment"] && q.Increment == 0 {
		r.diag(first.expr.Pos(), fmt.Sprintf("sequence %q: Increment must not be 0", name))
		return
	}
	if err := schema.ValidateSequence(q); err != nil {
		r.diag(first.expr.Pos(), err.Error())
		return
	}
	if !r.declare(r.seqAt, "sequence", q.Schema+"."+name, first.expr.Args[0].Pos()) {
		return
	}
	r.res.Sequences = append(r.res.Sequences, q)
}

// domain reads c.Domain(name).As(base) with its optional Schema, NotNull,
// Default and one Check(name, expr) for each named CHECK constraint.
func (r *companionReader) domain(calls []call) {
	first := calls[0]
	if len(first.expr.Args) != 1 {
		r.unsupported(first.expr)
		return
	}
	startDiags := len(r.diags)
	name, _ := r.str(first.expr.Args[0])
	d := schema.Domain{Name: name}
	seen := map[string]bool{}
	for _, c := range calls[1:] {
		args := c.expr.Args
		if c.name != "Check" {
			if seen[c.name] {
				r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s on domain %q", c.name, name))
				continue
			}
			seen[c.name] = true
		}
		switch c.name {
		case "NotNull":
			if len(args) != 0 {
				r.unsupported(c.expr)
				continue
			}
			d.NotNull = true
		case "Schema", "As", "Default":
			if len(args) != 1 {
				r.unsupported(c.expr)
				continue
			}
			s, ok := r.str(args[0])
			if !ok {
				continue
			}
			switch c.name {
			case "Schema":
				d.Schema = s
			case "As":
				d.Type = s
			default:
				d.Default = s
			}
		case "Check":
			if len(args) != 2 {
				r.unsupported(c.expr)
				continue
			}
			n, ok1 := r.str(args[0])
			e, ok2 := r.str(args[1])
			if ok1 && ok2 {
				d.Checks = append(d.Checks, schema.DomainCheck{Name: n, Expr: e})
			}
		default:
			r.unsupported(c.expr)
		}
	}
	if len(r.diags) != startDiags {
		return
	}
	if err := schema.ValidateDomain(d); err != nil {
		r.diag(first.expr.Pos(), err.Error())
		return
	}
	if !r.declare(r.typeAt, "type", d.Ref(), first.expr.Args[0].Pos()) {
		return
	}
	r.res.Domains = append(r.res.Domains, d)
}

// composite reads c.Composite(name) with its optional Schema and one
// Attr(name, type) for each attribute, in order.
func (r *companionReader) composite(calls []call) {
	first := calls[0]
	if len(first.expr.Args) != 1 {
		r.unsupported(first.expr)
		return
	}
	startDiags := len(r.diags)
	name, _ := r.str(first.expr.Args[0])
	cp := schema.Composite{Name: name}
	schemaSeen := false
	for _, c := range calls[1:] {
		args := c.expr.Args
		switch c.name {
		case "Schema":
			if len(args) != 1 {
				r.unsupported(c.expr)
				continue
			}
			if schemaSeen {
				r.diag(c.expr.Pos(), fmt.Sprintf("duplicate Schema on composite type %q", name))
				continue
			}
			schemaSeen = true
			if s, ok := r.str(args[0]); ok {
				cp.Schema = s
			}
		case "Attr":
			if len(args) != 2 {
				r.unsupported(c.expr)
				continue
			}
			n, ok1 := r.str(args[0])
			t, ok2 := r.str(args[1])
			if ok1 && ok2 {
				cp.Attributes = append(cp.Attributes, schema.CompositeAttr{Name: n, Type: t})
			}
		default:
			r.unsupported(c.expr)
		}
	}
	if len(r.diags) != startDiags {
		return
	}
	if err := schema.ValidateComposite(cp); err != nil {
		r.diag(first.expr.Pos(), err.Error())
		return
	}
	if !r.declare(r.typeAt, "type", cp.Ref(), first.expr.Args[0].Pos()) {
		return
	}
	r.res.Composites = append(r.res.Composites, cp)
}

// int64Lit accepts an integer literal, optionally signed.
func (r *companionReader) int64Lit(e ast.Expr) (int64, bool) {
	text := ""
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind == token.INT {
			text = x.Value
		}
	case *ast.UnaryExpr:
		if lit, ok := x.X.(*ast.BasicLit); ok && lit.Kind == token.INT {
			switch x.Op {
			case token.SUB:
				text = "-" + lit.Value
			case token.ADD:
				text = lit.Value
			}
		}
	}
	if text != "" {
		if n, err := strconv.ParseInt(text, 0, 64); err == nil {
			return n, true
		}
	}
	r.unsupported(e)
	return 0, false
}

// checkIn reads c.CheckIn(&m.Kind, "a", "b") with its optional Name. The field
// must be a string field, the values distinct string literals, and the
// constraint name unused by a tag or another declaration.
func (r *companionReader) checkIn(calls []call) {
	first := calls[0]
	args := first.expr.Args
	if len(args) == 0 || first.expr.Ellipsis.IsValid() {
		r.unsupported(first.expr)
		return
	}
	startDiags := len(r.diags)
	f, fieldOK := r.field(args[0])
	if fieldOK && f.BaseType != "string" {
		r.diag(args[0].Pos(), fmt.Sprintf("CheckIn field %s must be a string field, it is %s", r.source(args[0]), f.GoType))
	}
	if len(args) == 1 {
		r.diag(first.expr.Pos(), "CheckIn lists no values; it needs at least one")
	}
	var values []string
	seen := map[string]bool{}
	for _, a := range args[1:] {
		v, ok := r.str(a)
		if !ok {
			continue
		}
		if seen[v] {
			r.diag(a.Pos(), fmt.Sprintf("CheckIn repeats the value %q", v))
			continue
		}
		seen[v] = true
		values = append(values, v)
	}
	name, named := "", false
	for _, c := range calls[1:] {
		if c.name != "Name" || len(c.expr.Args) != 1 {
			r.unsupported(c.expr)
			continue
		}
		if named {
			r.diag(c.expr.Pos(), "duplicate Name on CheckIn")
			continue
		}
		named = true
		if s, ok := r.str(c.expr.Args[0]); ok {
			if s == "" {
				r.diag(c.expr.Args[0].Pos(), "CheckIn Name is empty")
				continue
			}
			name = s
		}
	}
	if len(r.diags) != startDiags || f == nil {
		return
	}
	if name == "" {
		name = r.m.Table + "_" + f.Column + "_check"
	}
	if !r.declare(r.checkAt, "check", name, args[0].Pos()) {
		return
	}
	r.res.CheckIns = append(r.res.CheckIns, CheckInDecl{Name: name, Column: f.Column, Values: values})
}

// foreignKey reads c.ForeignKey(name, []any{&m.A, &m.B}, "table", []string{"x", "y"})
// with its optional OnDelete and OnUpdate.
func (r *companionReader) foreignKey(calls []call) {
	first := calls[0]
	args := first.expr.Args
	if len(args) != 4 {
		r.unsupported(first.expr)
		return
	}
	startDiags := len(r.diags)
	name, _ := r.str(args[0])
	fk := schema.ForeignKey{Name: name}
	fieldsLit, ok := args[1].(*ast.CompositeLit)
	if !ok || !r.isAnySlice(fieldsLit) {
		r.unsupported(args[1])
	} else {
		for _, el := range fieldsLit.Elts {
			if f, ok := r.field(el); ok {
				fk.Columns = append(fk.Columns, f.Column)
			}
		}
	}
	fk.RefTable, _ = r.str(args[2])
	refLit, ok := args[3].(*ast.CompositeLit)
	if !ok || !r.isStringSlice(refLit) {
		r.unsupported(args[3])
	} else {
		for _, el := range refLit.Elts {
			if s, ok := r.str(el); ok {
				fk.RefColumns = append(fk.RefColumns, s)
			}
		}
	}
	seen := map[string]bool{}
	for _, c := range calls[1:] {
		if c.name == "Deferrable" || c.name == "InitiallyDeferred" {
			if len(c.expr.Args) != 0 {
				r.unsupported(c.expr)
				continue
			}
			if seen[c.name] {
				r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s on foreign key %q", c.name, name))
				continue
			}
			seen[c.name] = true
			fk.Deferrable = true
			fk.InitiallyDeferred = fk.InitiallyDeferred || c.name == "InitiallyDeferred"
			continue
		}
		if len(c.expr.Args) != 1 || (c.name != "OnDelete" && c.name != "OnUpdate") {
			r.unsupported(c.expr)
			continue
		}
		if seen[c.name] {
			r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s on foreign key %q", c.name, name))
			continue
		}
		seen[c.name] = true
		v, ok := r.str(c.expr.Args[0])
		if !ok {
			continue
		}
		a, valid := action(v)
		if !valid || a == "" {
			r.diag(c.expr.Args[0].Pos(), fmt.Sprintf("%s %q is not a referential action", c.name, v))
			continue
		}
		if c.name == "OnDelete" {
			fk.OnDelete = a
		} else {
			fk.OnUpdate = a
		}
	}
	if len(r.diags) != startDiags {
		return
	}
	if len(fk.Columns) == 0 || len(fk.Columns) != len(fk.RefColumns) {
		r.diag(first.expr.Pos(), fmt.Sprintf("foreign key %q: %d columns reference %d columns; the lists must be non-empty and of equal length", name, len(fk.Columns), len(fk.RefColumns)))
		return
	}
	if !r.declare(r.fkAt, "foreign key", name, args[0].Pos()) {
		return
	}
	r.res.ForeignKeys = append(r.res.ForeignKeys, fk)
}

// excludeOperator matches the spelling of an operator name; it reaches the DDL
// unquoted, so nothing else is accepted.
var excludeOperator = regexp.MustCompile("^[-+*/<>=~!@#%^&|`?]+$")

// excludeMethods are the access methods an exclusion constraint can use.
var excludeMethods = map[string]bool{"gist": true, "spgist": true, "btree": true, "hash": true}

// exclude reads c.Exclude(name, &m.A, "=", &m.B, "&&") with its optional Using,
// Deferrable and InitiallyDeferred.
func (r *companionReader) exclude(calls []call) {
	first := calls[0]
	args := first.expr.Args
	if len(args) < 3 || len(args)%2 == 0 || first.expr.Ellipsis.IsValid() {
		r.diag(first.expr.Pos(), "Exclude takes a name followed by pairs of a field reference and an operator, such as c.Exclude(\"name\", &m.RoomID, \"=\", &m.During, \"&&\")")
		return
	}
	startDiags := len(r.diags)
	name, _ := r.str(args[0])
	x := schema.Exclude{Name: name}
	for i := 1; i < len(args); i += 2 {
		f, okField := r.field(args[i])
		op, okOp := r.str(args[i+1])
		if okOp && !excludeOperator.MatchString(op) {
			r.diag(args[i+1].Pos(), fmt.Sprintf("exclusion constraint %q: %q is not an operator", name, op))
			okOp = false
		}
		if okField && okOp {
			x.Elements = append(x.Elements, schema.ExcludeElement{Column: f.Column, Operator: op})
		}
	}
	seen := map[string]bool{}
	for _, c := range calls[1:] {
		if seen[c.name] {
			r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s on exclusion constraint %q", c.name, name))
			continue
		}
		seen[c.name] = true
		switch c.name {
		case "Using":
			if len(c.expr.Args) != 1 {
				r.unsupported(c.expr)
				continue
			}
			m, ok := r.str(c.expr.Args[0])
			if !ok {
				continue
			}
			if !excludeMethods[m] {
				r.diag(c.expr.Args[0].Pos(), fmt.Sprintf("exclusion constraint %q: access method %q is not one of gist, spgist, btree, hash", name, m))
				continue
			}
			x.Method = m
		case "Deferrable", "InitiallyDeferred":
			if len(c.expr.Args) != 0 {
				r.unsupported(c.expr)
				continue
			}
			x.Deferrable = true
			x.InitiallyDeferred = x.InitiallyDeferred || c.name == "InitiallyDeferred"
		default:
			r.unsupported(c.expr)
		}
	}
	if len(r.diags) != startDiags {
		return
	}
	// The constraint is backed by an index of its own name, so it shares the
	// index namespace.
	if !r.declare(r.indexAt, "index", name, args[0].Pos()) {
		return
	}
	r.res.Excludes = append(r.res.Excludes, x)
}

// isAnySlice reports whether the composite literal is a []any.
func (r *companionReader) isAnySlice(lit *ast.CompositeLit) bool {
	t := r.pkg.TypesInfo.TypeOf(lit)
	if t == nil {
		return false
	}
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	i, ok := s.Elem().Underlying().(*types.Interface)
	return ok && i.Empty()
}

func (r *companionReader) part(ix *schema.Index, e ast.Expr) *schema.IndexPart {
	f, ok := r.field(e)
	if !ok {
		return nil
	}
	for i := range ix.Parts {
		if ix.Parts[i].Column == f.Column {
			return &ix.Parts[i]
		}
	}
	r.diag(e.Pos(), fmt.Sprintf("field %s is not a part of index %q", r.source(e), ix.Name))
	return nil
}

func (r *companionReader) indexOption(ix *schema.Index, c call) {
	args := c.expr.Args
	need := func(n int) bool {
		if len(args) != n {
			r.unsupported(c.expr)
			return false
		}
		return true
	}
	// once tracks which options were set, so an empty value still counts.
	once := func(key string, dst *string, v string) {
		if r.optSeen[key] {
			r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s on index %q", c.name, ix.Name))
			return
		}
		r.optSeen[key] = true
		*dst = v
	}
	switch c.name {
	case "Where":
		if need(1) {
			if s, ok := r.str(args[0]); ok {
				once("Where", &ix.Where, s)
			}
		}
	case "Method":
		if need(1) {
			if s, ok := r.str(args[0]); ok {
				once("Method", &ix.Method, s)
			}
		}
	case "With":
		if need(2) {
			k, ok1 := r.str(args[0])
			v, ok2 := r.str(args[1])
			if ok1 && ok2 {
				ix.With = append(ix.With, schema.KV{Key: k, Value: v})
			}
		}
	case "Opclass":
		if need(2) {
			p := r.part(ix, args[0])
			s, ok := r.str(args[1])
			if p != nil && ok {
				once("Opclass:"+p.Column, &p.OpClass, s)
			}
		}
	case "Desc":
		if need(1) {
			if p := r.part(ix, args[0]); p != nil {
				p.Desc = true
			}
		}
	case "Nulls":
		if need(2) {
			p := r.part(ix, args[0])
			s, ok := r.str(args[1])
			if ok && s != "first" && s != "last" {
				r.diag(args[1].Pos(), `Nulls order must be "first" or "last"`)
				ok = false
			}
			if p != nil && ok {
				once("Nulls:"+p.Column, &p.Nulls, s)
			}
		}
	case "Concurrently":
		if need(0) {
			ix.Concurrently = true
		}
	case "Unique":
		if need(0) {
			if ix.Unique {
				r.diag(c.expr.Pos(), fmt.Sprintf("index %q is already unique", ix.Name))
				return
			}
			ix.Unique = true
		}
	default:
		r.unsupported(c.expr)
	}
}

// config handles the single-call Config methods other than the index ones.
func (r *companionReader) config(c call) {
	args := c.expr.Args
	if rt, ok := structuredTypes[c.name]; ok {
		r.structured(c, rt)
		return
	}
	startDiags := len(r.diags)
	switch c.name {
	case "Check":
		if len(args) != 2 {
			r.unsupported(c.expr)
			return
		}
		name, _ := r.str(args[0])
		expr, _ := r.str(args[1])
		if len(r.diags) == startDiags && r.declare(r.checkAt, "check", name, args[0].Pos()) {
			r.res.Checks = append(r.res.Checks, schema.Check{Name: name, Expr: expr})
		}
	case "Enum":
		if len(args) < 2 {
			r.unsupported(c.expr)
			return
		}
		name, _ := r.str(args[0])
		var vals []string
		for _, a := range args[1:] {
			if v, ok := r.str(a); ok {
				vals = append(vals, v)
			}
		}
		if len(r.diags) == startDiags && r.declare(r.enumAt, "enum", name, args[0].Pos()) {
			r.res.Enums = append(r.res.Enums, schema.Enum{Name: name, Values: vals})
		}
	case "Default":
		r.columnText(c, r.res.Defaults, "default")
	case "Generated":
		r.columnText(c, r.res.Generated, "generated expression")
	case "GeneratedVirtual":
		r.columnText(c, r.res.Generated, "generated expression")
		if len(c.expr.Args) == 2 {
			if f, ok := r.field(c.expr.Args[0]); ok {
				r.res.GeneratedVirtual[f.Column] = true
			}
		}
	case "Comment":
		r.columnText(c, r.res.Comments, "comment")
	case "Collate":
		r.columnText(c, r.res.Collates, "collate")
	case "IDGen":
		r.columnText(c, r.res.IDGens, "idgen")
	case "PrimaryKey":
		if len(r.res.PrimaryKey) > 0 {
			r.diag(c.expr.Pos(), "duplicate PrimaryKey call")
			return
		}
		if len(c.expr.Args) == 0 || c.expr.Ellipsis.IsValid() {
			r.unsupported(c.expr)
			return
		}
		var cols []string
		for _, a := range c.expr.Args {
			if f, ok := r.field(a); ok {
				cols = append(cols, f.Column)
			}
		}
		if len(cols) == len(c.expr.Args) {
			r.res.PrimaryKey = cols
		}
	case "PrimaryKeyDeferrable", "PrimaryKeyInitiallyDeferred":
		if len(c.expr.Args) != 0 {
			r.unsupported(c.expr)
			return
		}
		// Both calls set PrimaryKeyDeferrable, so any second call among the
		// two, in either order, is a repeat.
		if r.res.PrimaryKeyDeferrable {
			r.diag(c.expr.Pos(), "duplicate "+c.name+" call")
			return
		}
		r.res.PrimaryKeyDeferrable = true
		r.res.PrimaryKeyInitiallyDeferred = r.res.PrimaryKeyInitiallyDeferred || c.name == "PrimaryKeyInitiallyDeferred"
	case "History":
		if len(c.expr.Args) != 0 {
			r.unsupported(c.expr)
			return
		}
		if r.res.History {
			r.diag(c.expr.Pos(), "duplicate History call")
			return
		}
		r.res.History = true
	case "FKName":
		r.columnText(c, r.res.FKNames, "foreign key name")
	default:
		r.unsupported(c.expr)
	}
}

// columnText handles Default, Comment and Collate: a field and a string.
func (r *companionReader) columnText(c call, dst map[string]string, what string) {
	args := c.expr.Args
	if len(args) != 2 {
		r.unsupported(c.expr)
		return
	}
	f, ok1 := r.field(args[0])
	s, ok2 := r.str(args[1])
	if !ok1 || !ok2 {
		return
	}
	if _, dup := dst[f.Column]; dup {
		r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s for column %q", what, f.Column))
		return
	}
	dst[f.Column] = s
}

// structured reads a composite literal of a schema type with keyed literal
// fields only.
func (r *companionReader) structured(c call, rt reflect.Type) {
	if len(c.expr.Args) != 1 {
		r.unsupported(c.expr)
		return
	}
	arg := c.expr.Args[0]
	lit, ok := arg.(*ast.CompositeLit)
	if !ok || !r.isSchemaType(lit, rt.Name()) {
		r.unsupported(arg)
		return
	}
	rv := reflect.New(rt).Elem()
	startDiags := len(r.diags)
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			r.unsupported(lit)
			return
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			r.unsupported(kv.Key)
			continue
		}
		fv := rv.FieldByName(key.Name)
		if !fv.IsValid() {
			r.diag(key.Pos(), fmt.Sprintf("schema.%s has no field %s", rt.Name(), key.Name))
			continue
		}
		r.literal(fv, kv.Value)
	}
	if len(r.diags) != startDiags {
		return
	}
	set := func(dup bool) bool {
		if dup {
			r.diag(c.expr.Pos(), fmt.Sprintf("duplicate %s declaration", c.name))
		}
		return !dup
	}
	switch v := rv.Interface().(type) {
	case schema.View:
		r.res.Views = append(r.res.Views, v)
	case schema.Function:
		r.res.Functions = append(r.res.Functions, v)
	case schema.Trigger:
		r.res.Triggers = append(r.res.Triggers, v)
	case schema.Policy:
		r.res.Policies = append(r.res.Policies, v)
	case schema.RLS:
		if set(r.res.RLS != nil) {
			r.res.RLS = &v
		}
	case schema.Partition:
		if set(r.res.Partition != nil) {
			r.res.Partition = &v
		}
	case schema.PartitionOf:
		if set(r.res.PartitionOf != nil) {
			r.res.PartitionOf = &v
		}
	}
}

// isSchemaType reports whether the composite literal has type schema.<name>.
func (r *companionReader) isSchemaType(lit *ast.CompositeLit, name string) bool {
	t := r.pkg.TypesInfo.TypeOf(lit)
	if t == nil {
		return false
	}
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == schemaPkgPath && n.Obj().Name() == name
}

// literal stores a string literal, a bool literal or a []string literal of
// string literals into fv.
func (r *companionReader) literal(fv reflect.Value, e ast.Expr) {
	if !fv.CanSet() {
		r.unsupported(e)
		return
	}
	switch fv.Kind() {
	case reflect.String:
		if s, ok := r.str(e); ok {
			fv.Set(reflect.ValueOf(s).Convert(fv.Type()))
		}
	case reflect.Bool:
		if id, ok := e.(*ast.Ident); ok && (id.Name == "true" || id.Name == "false") {
			if c, ok := r.pkg.TypesInfo.ObjectOf(id).(*types.Const); ok && c.Parent() == types.Universe {
				fv.SetBool(id.Name == "true")
				return
			}
		}
		r.unsupported(e)
	case reflect.Slice:
		lit, ok := e.(*ast.CompositeLit)
		if !ok || fv.Type().Elem().Kind() != reflect.String || !r.isStringSlice(lit) {
			r.unsupported(e)
			return
		}
		elem := fv.Type().Elem()
		if !reflect.TypeOf("").ConvertibleTo(elem) {
			r.unsupported(e)
			return
		}
		out := reflect.MakeSlice(fv.Type(), 0, len(lit.Elts))
		for _, el := range lit.Elts {
			s, ok := r.str(el)
			if !ok {
				return
			}
			out = reflect.Append(out, reflect.ValueOf(s).Convert(elem))
		}
		fv.Set(out)
	default:
		r.unsupported(e)
	}
}

func (r *companionReader) isStringSlice(lit *ast.CompositeLit) bool {
	t := r.pkg.TypesInfo.TypeOf(lit)
	if t == nil {
		return false
	}
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	b, ok := types.Unalias(s.Elem()).(*types.Basic)
	return ok && b.Kind() == types.String
}
