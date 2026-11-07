// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"go/format"
	"go/types"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/gen/naming"
)

const momoImportPath = "github.com/azrtydxb/momo"

// descEmitter builds the source of one model's descriptor subpackage.
type descEmitter struct {
	m     *Model
	diags []Diagnostic
	// imports maps an import alias to its path; only used packages appear.
	imports map[string]string
	namer   naming.Namer
}

func (e *descEmitter) diag(msg string) {
	e.diags = append(e.diags, Diagnostic{Pos: e.m.Pos, Msg: msg})
}

// use records that alias is needed for path and reports conflicts.
func (e *descEmitter) use(alias, path string) string {
	if prev, ok := e.imports[alias]; ok && prev != path {
		e.diags = append(e.diags, Diagnostic{Pos: e.m.Pos, Msg: fmt.Sprintf(
			"import name %q is used by both %s and %s", alias, prev, path)})
		return alias
	}
	e.imports[alias] = path
	return alias
}

func (e *descEmitter) qualifier(p *types.Package) string {
	return e.use(p.Name(), p.Path())
}

func (e *descEmitter) typeString(t types.Type) string {
	return types.TypeString(t, e.qualifier)
}

// modelType returns the qualified Go type name of a model.
func (e *descEmitter) modelType(m *Model) string {
	return e.use(m.PkgName, e.modelImportPath(m)) + "." + m.GoName
}

// modelImportPath builds the import path of m's package through ImportPath,
// so it is slash-separated on every OS. A model without module information
// (built by hand) falls back to its recorded package path.
func (e *descEmitter) modelImportPath(m *Model) string {
	if m.ModulePath == "" {
		return m.PkgPath
	}
	p, err := ImportPath(m.ModulePath, m.ModuleDir, m.PkgDir)
	if err != nil {
		e.diags = append(e.diags, Diagnostic{Pos: m.Pos, Msg: err.Error()})
		return m.PkgPath
	}
	return p
}

func (e *descEmitter) tableRef(m *Model) string {
	s := "momo.TableRef{Name: " + strconv.Quote(m.Table)
	if m.Schema != "" {
		s += ", Schema: " + strconv.Quote(m.Schema)
	}
	return s + "}"
}

// baseTableRef is tableRef for the Table variable: it also names the
// soft-delete column, so a projection read FROM the table is filtered.
func (e *descEmitter) baseTableRef(m *Model) string {
	s := strings.TrimSuffix(e.tableRef(m), "}")
	for _, f := range m.Fields {
		if f.Tags.SoftDelete == "" {
			continue
		}
		s += ", SoftDelete: " + strconv.Quote(f.Column)
		switch f.Tags.SoftDelete {
		case "unix":
			s += ", SoftDeleteKind: momo.SoftDeleteUnix"
		case "bool":
			s += ", SoftDeleteKind: momo.SoftDeleteBool"
		}
		break
	}
	return s + "}"
}

// fieldVar walks the index path of f through the model struct and returns the
// Go selector and the field's variable.
func fieldVar(m *Model, f *Field) (string, *types.Var) {
	if m.named == nil {
		return f.GoName, nil
	}
	st, _ := m.named.Underlying().(*types.Struct)
	var parts []string
	var v *types.Var
	for i, idx := range f.Path {
		if st == nil || idx >= st.NumFields() {
			return f.GoName, nil
		}
		v = st.Field(idx)
		parts = append(parts, v.Name())
		if i < len(f.Path)-1 {
			t := types.Unalias(v.Type())
			if p, ok := t.(*types.Pointer); ok {
				t = types.Unalias(p.Elem())
			}
			st, _ = t.Underlying().(*types.Struct)
		}
	}
	return strings.Join(parts, "."), v
}

func derefType(t types.Type) (types.Type, bool) {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		return p.Elem(), true
	}
	return t, false
}

// vectorTypeNames are the momo/vector types a momo.VectorColumn can hold.
var vectorTypeNames = map[string]bool{"Vector": true, "HalfVector": true, "SparseVector": true, "Bits": true}

// isVectorType reports whether t, after one pointer, is one of the four types
// of the momo vector package. A look-alike declared elsewhere is not.
func isVectorType(t types.Type) bool {
	t, _ = derefType(t)
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return false
	}
	return n.Obj().Pkg().Path() == momoImportPath+"/vector" && vectorTypeNames[n.Obj().Name()]
}

var vectorTypeDim = regexp.MustCompile(`\((\d+)\)`)

// vectorDim is the declared dimension of a vector field: the dim tag, else
// the length in an explicit type tag such as vector(3), else 0 (unchecked).
func vectorDim(f *Field) int {
	if f.Tags.Dim > 0 {
		return f.Tags.Dim
	}
	if m := vectorTypeDim.FindStringSubmatch(f.Tags.Type); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

var flagOrder = []struct {
	name string
	on   func(f *Field) bool
}{
	{"FlagPK", func(f *Field) bool { return f.Tags.PK }},
	{"FlagUnique", func(f *Field) bool { return f.Tags.Unique }},
	{"FlagNullable", func(f *Field) bool { return f.Nullable || f.Tags.Nullable }},
	{"FlagIdentity", func(f *Field) bool { return f.Tags.Identity }},
	{"FlagReadOnly", func(f *Field) bool { return f.Access == ReadOnly }},
	{"FlagVersion", func(f *Field) bool { return f.Tags.Version }},
	{"FlagSoftDelete", func(f *Field) bool { return f.Tags.SoftDelete != "" }},
	{"FlagHasDefault", func(f *Field) bool { return f.Tags.Default != "" }},
	{"FlagCreateOnly", func(f *Field) bool { return f.Access == CreateOnly }},
	{"FlagImmutable", func(f *Field) bool { return f.Access == Immutable }},
	{"FlagSensitive", func(f *Field) bool { return f.Tags.Sensitive }},
	{"FlagCreatedAt", func(f *Field) bool { return f.Tags.CreatedAt }},
	{"FlagUpdatedAt", func(f *Field) bool { return f.Tags.UpdatedAt }},
}

func flagExpr(f *Field) string {
	var parts []string
	for _, fl := range flagOrder {
		if fl.on(f) {
			parts = append(parts, "momo."+fl.name)
		}
	}
	if len(parts) == 0 {
		return "0"
	}
	return strings.Join(parts, "|")
}

// EmitDescriptors returns the source of the per-model descriptor subpackage:
// typed columns, the alias-aware As constructor and one descriptor per
// relation. The result is gofmt-formatted and starts with Header().
func EmitDescriptors(m *Model, g *Graph) ([]byte, []Diagnostic) {
	e := &descEmitter{m: m, imports: map[string]string{"momo": momoImportPath}, namer: graphNamer(g)}

	// Register every import alias the output will use before any identifier
	// is chosen, so a field named like an alias is renamed instead of
	// redeclaring the import.
	for _, f := range m.Fields {
		if _, v := fieldVar(m, f); v != nil {
			t, _ := derefType(v.Type())
			e.typeString(t)
		}
	}
	for _, r := range m.Relations {
		e.emitRelation(&strings.Builder{}, m, r, "_")
	}
	e.diags = nil

	aliases := map[string]bool{}
	for alias := range e.imports {
		aliases[alias] = true
	}
	ids := fieldIdents(m, aliases)
	used := DescriptorReserved()
	for alias := range aliases {
		used[alias] = true
	}
	for _, id := range ids {
		used[id] = true
	}
	type col struct {
		name, typ, ctor, colName, flags string
		dim                             int  // vector columns only
		vector                          bool // emitted as a momo vector column
	}
	cols := make([]col, 0, len(m.Fields))
	for i, f := range m.Fields {
		name := ids[i]
		_, v := fieldVar(m, f)
		var elem string
		if v != nil {
			t, _ := derefType(v.Type())
			elem = e.typeString(t)
		} else {
			elem = strings.TrimPrefix(f.GoType, "*")
		}
		typeName, ctor := DescriptorType(f.Access, f.Nullable || f.Tags.Nullable)
		c := col{
			name:    name,
			typ:     "momo." + typeName + "[" + elem + "]",
			ctor:    "momo." + ctor + "[" + elem + "]",
			colName: f.Column,
			flags:   flagExpr(f),
		}
		if v != nil && isVectorType(v.Type()) {
			tn, cn := VectorDescriptorType(f.Access, f.Nullable || f.Tags.Nullable)
			c.typ = "momo." + tn + "[" + elem + "]"
			c.ctor = "momo." + cn + "[" + elem + "]"
			c.dim = vectorDim(f)
			c.vector = true
		}
		cols = append(cols, c)
	}

	var body strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&body, format, a...) }

	w("// Table is the base table reference.\nvar Table = %s\n\n", e.baseTableRef(m))
	w("// Columns is one alias's set of column descriptors.\ntype Columns struct {\n")
	for _, c := range cols {
		w("\t%s %s\n", c.name, c.typ)
	}
	w("}\n\n")
	w("// As returns the column descriptors qualified by alias (the table name when alias is empty).\n")
	w("func As(alias string) Columns {\n\tq := alias\n\tif q == \"\" {\n\t\tq = %s\n\t}\n\treturn Columns{\n", strconv.Quote(m.Table))
	for _, c := range cols {
		if c.vector {
			w("\t\t%s: %s(q, %s, %d, %s),\n", c.name, c.ctor, strconv.Quote(c.colName), c.dim, c.flags)
			continue
		}
		w("\t\t%s: %s(q, %s, %s),\n", c.name, c.ctor, strconv.Quote(c.colName), c.flags)
	}
	w("\t}\n}\n\n")
	w("var base = As(\"\")\n\n")
	if len(cols) > 0 {
		w("var (\n")
		for _, c := range cols {
			w("\t%s = base.%s\n", c.name, c.name)
		}
		w(")\n")
	}
	lookup, lds := emitLookup(m, ids)
	e.diags = append(e.diags, lds...)
	e.use("fmt", "fmt")
	body.Write(lookup)
	for _, r := range m.Relations {
		name := Identifier(r.GoName, used)
		used[name] = true
		e.emitRelation(&body, m, r, name)
	}

	var out strings.Builder
	out.WriteString(Header() + "\n\n")
	fmt.Fprintf(&out, "// Package %s holds typed column descriptors and relation descriptors for the %s model.\n", m.SubPkg, m.GoName)
	fmt.Fprintf(&out, "package %s\n\n", m.SubPkg)
	out.WriteString(e.importBlock())
	out.WriteString(body.String())

	if len(e.diags) > 0 {
		return nil, e.diags
	}
	src, err := format.Source([]byte(out.String()))
	if err != nil {
		return nil, []Diagnostic{{Pos: m.Pos, Msg: "generator bug: formatting descriptors for " + m.GoName + ": " + err.Error()}}
	}
	return src, nil
}

func (e *descEmitter) importBlock() string {
	var std, other []string
	for alias, path := range e.imports {
		line := "\t" + strconv.Quote(path) + "\n"
		// A first path element without a dot is a standard library package,
		// the same rule the go command applies. A module path without a dot
		// would be misfiled, which only changes the grouping, not the build.
		first, _, _ := strings.Cut(path, "/")
		if !strings.Contains(first, ".") {
			if alias != path {
				line = "\t" + alias + " " + strconv.Quote(path) + "\n"
			}
			std = append(std, line)
			continue
		}
		if path != momoImportPath {
			line = "\t" + alias + " " + strconv.Quote(path) + "\n"
		}
		other = append(other, line)
	}
	sort.Strings(std)
	sort.Strings(other)
	var b strings.Builder
	b.WriteString("import (\n")
	b.WriteString(strings.Join(std, ""))
	if len(std) > 0 && len(other) > 0 {
		b.WriteString("\n")
	}
	b.WriteString(strings.Join(other, ""))
	b.WriteString(")\n\n")
	return b.String()
}

// keyPart is one component of a relation key: how to read a field as a base
// Go value and how to store a base value back.
type keyPart struct {
	sel   string // selector below the receiver, e.g. "Addr.ID"
	ptr   bool
	base  string // the base type asserted from the key slice
	named string // the field's own element type when it differs from base
}

func (e *descEmitter) keyPart(m *Model, f *Field) keyPart {
	sel, v := fieldVar(m, f)
	kp := keyPart{sel: sel}
	if v == nil {
		kp.base = strings.TrimPrefix(f.GoType, "*")
		kp.ptr = f.Nullable
		return kp
	}
	t, ptr := derefType(v.Type())
	kp.ptr = ptr
	kp.base = e.typeString(t)
	if _, isNamed := types.Unalias(t).(*types.Named); isNamed {
		if b, ok := t.Underlying().(*types.Basic); ok {
			kp.named = kp.base
			kp.base = b.Name()
		}
	}
	return kp
}

// read returns the expression for the key value of recv.
func (kp keyPart) read(recv string) string {
	x := recv + "." + kp.sel
	if kp.ptr {
		x = "*" + x
	}
	if kp.named != "" {
		return kp.base + "(" + x + ")"
	}
	return x
}

func getter(recv, typ string, kps []keyPart) string {
	head := "func(" + recv + " *" + typ + ") ([]any, bool) {"
	var vals []string
	var nilChecks []string
	for _, kp := range kps {
		vals = append(vals, kp.read(recv))
		if kp.ptr {
			nilChecks = append(nilChecks, recv+"."+kp.sel+" == nil")
		}
	}
	ret := "return []any{" + strings.Join(vals, ", ") + "}, true"
	if len(nilChecks) == 0 {
		return head + " " + ret + " }"
	}
	return head + "\nif " + strings.Join(nilChecks, " || ") + " {\nreturn nil, false\n}\n" + ret + "\n}"
}

func setter(recv, typ string, kps []keyPart, extra ...string) string {
	head := "func(" + recv + " *" + typ + ", k []any) {"
	var stmts []string
	simple := true
	for i, kp := range kps {
		val := "k[" + strconv.Itoa(i) + "].(" + kp.base + ")"
		if kp.named != "" {
			val = kp.named + "(" + val + ")"
		}
		if kp.ptr {
			simple = false
			v := "v" + strconv.Itoa(i)
			stmts = append(stmts, v+" := "+val, recv+"."+kp.sel+" = &"+v)
			continue
		}
		stmts = append(stmts, recv+"."+kp.sel+" = "+val)
	}
	stmts = append(stmts, extra...)
	if simple && len(stmts) == 1 {
		return head + " " + stmts[0] + " }"
	}
	return head + "\n" + strings.Join(stmts, "\n") + "\n}"
}

func pkFields(m *Model) []*Field {
	var out []*Field
	for _, f := range m.Fields {
		if f.Tags.PK {
			out = append(out, f)
		}
	}
	return out
}

func columnsOf(fs []*Field) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Column
	}
	return out
}

func stringSlice(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = strconv.Quote(s)
	}
	return "[]string{" + strings.Join(q, ", ") + "}"
}

// emitRelation writes one relation descriptor, or diagnostics when the keys
// cannot be derived.
func (e *descEmitter) emitRelation(b *strings.Builder, m *Model, r *Relation, name string) {
	t := r.Target
	if t == nil {
		e.diag("relation " + r.GoName + " has no resolved target")
		return
	}
	parentPK, childPK := pkFields(m), pkFields(t)
	if len(parentPK) != 1 || len(childPK) != 1 {
		e.diag(fmt.Sprintf("relation %s: tag relations need a single-column primary key on %s and %s", r.GoName, m.GoName, t.GoName))
		return
	}
	var parentKey, childKey []*Field
	var parentCols, childCols []string
	switch r.Kind {
	case BelongsTo:
		fk := fieldByGoName(m, r.FK)
		parentKey, childKey = []*Field{fk}, childPK
		parentCols, childCols = []string{fk.Column}, columnsOf(childPK)
	case HasOne, HasMany:
		fk := fieldByGoName(t, r.FK)
		parentKey, childKey = parentPK, []*Field{fk}
		parentCols, childCols = columnsOf(parentPK), []string{fk.Column}
	default:
		parentKey, childKey = parentPK, childPK
		parentCols, childCols = columnsOf(parentPK), columnsOf(childPK)
	}
	pt, ct := e.modelType(m), e.modelType(t)
	partsOf := func(owner *Model, fs []*Field) []keyPart {
		out := make([]keyPart, len(fs))
		for i, f := range fs {
			out[i] = e.keyPart(owner, f)
		}
		return out
	}
	pk, ck := partsOf(m, parentKey), partsOf(t, childKey)

	kind := map[RelKind]string{BelongsTo: "BelongsTo", HasOne: "HasOne", HasMany: "HasMany", ManyToMany: "ManyToMany"}[r.Kind]
	fmt.Fprintf(b, "\n// %s is the %s relation of %s.\n", name, r.Kind.tagName(), m.GoName)
	fmt.Fprintf(b, "var %s = momo.Relation[%s, %s]{\n", name, pt, ct)
	fmt.Fprintf(b, "Name: %s,\nKind: momo.%s,\n", strconv.Quote(r.GoName), kind)
	fmt.Fprintf(b, "Parent: %s,\nChild: %s,\n", e.tableRef(m), e.tableRef(t))
	fmt.Fprintf(b, "ParentMeta: (*%s)(nil).MomoMeta(),\nChildMeta: (*%s)(nil).MomoMeta(),\n", pt, ct)
	fmt.Fprintf(b, "ParentCols: %s,\nChildCols: %s,\nChildPK: %s,\n", stringSlice(parentCols), stringSlice(childCols), stringSlice(columnsOf(childPK)))
	if r.Kind == ManyToMany {
		jp := joinColumnName(e.namer, m, parentPK[0].Column)
		jc := joinColumnName(e.namer, t, childPK[0].Column)
		fmt.Fprintf(b, "Through: &momo.Through{Table: momo.TableRef{Name: %s}, ParentCols: %s, ChildCols: %s},\n",
			strconv.Quote(r.JoinTable), stringSlice([]string{jp}), stringSlice([]string{jc}))
	}
	if r.PolyType != "" {
		fmt.Fprintf(b, "Discriminator: &momo.Discriminator{Column: %s, Value: %s},\n", strconv.Quote(r.PolyType), strconv.Quote(r.PolyValue))
	}
	fmt.Fprintf(b, "ParentKey: %s,\nChildKey: %s,\n", getter("p", pt, pk), getter("c", ct, ck))
	switch r.Kind {
	case BelongsTo:
		fmt.Fprintf(b, "SetParentKey: %s,\n", setter("p", pt, pk))
	case HasOne, HasMany:
		fmt.Fprintf(b, "SetChildKey: %s,\n", setter("c", ct, ck, e.discriminatorStmts(t, r)...))
	}
	if r.Kind == BelongsTo || r.Kind == HasOne {
		fmt.Fprintf(b, "GetOne: func(p *%s) *%s { return p.%s },\n", pt, ct, r.GoName)
		fmt.Fprintf(b, "SetOne: func(p *%s, c *%s) { p.%s = c },\n", pt, ct, r.GoName)
	} else {
		fmt.Fprintf(b, "GetMany: func(p *%s) []%s { return p.%s },\n", pt, ct, r.GoName)
		fmt.Fprintf(b, "SetMany: func(p *%s, cs []%s) { p.%s = cs },\n", pt, ct, r.GoName)
	}
	b.WriteString("}\n")
}

// discriminatorStmts returns the statements that set the discriminator field
// of a polymorphic child to the relation's value, converting to the field's
// named type when it has one. It returns nil for a plain relation.
func (e *descEmitter) discriminatorStmts(t *Model, r *Relation) []string {
	if r.PolyType == "" {
		return nil
	}
	var f *Field
	for _, c := range t.Fields {
		if c.Column == r.PolyType {
			f = c
		}
	}
	if f == nil {
		e.diag("polymorphic column " + r.PolyType + " is not a field of " + t.GoName)
		return nil
	}
	kp := e.keyPart(t, f)
	if kp.base != "string" {
		e.diag(fmt.Sprintf("polymorphic column %s of %s must be a string field", r.PolyType, t.GoName))
		return nil
	}
	val := strconv.Quote(r.PolyValue)
	if kp.named != "" {
		val = kp.named + "(" + val + ")"
	}
	if kp.ptr {
		return []string{"d := " + val, "c." + kp.sel + " = &d"}
	}
	return []string{"c." + kp.sel + " = " + val}
}
