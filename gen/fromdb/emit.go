// SPDX-License-Identifier: Apache-2.0

// Package fromdb turns an inspected database schema into Go model source and
// the schema the matching init snapshot is written from. It is the library
// behind `momo generate --from-db`; it never connects to a database.
//
// The models it writes use the tag grammar and the MomoConfig companion that
// `momo generate` reads, so the schema they build is the inspected schema.
// What the models cannot declare is left out of both the models and the
// returned snapshot schema and reported as a finding, so the two never
// disagree silently.
package fromdb

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/gen/typemap"
	"github.com/azrtydxb/momo/internal/dbdefault"
	"github.com/azrtydxb/momo/schema"
)

const (
	importMomo   = "github.com/azrtydxb/momo"
	importSchema = "github.com/azrtydxb/momo/schema"
)

// Severity ranks a finding.
type Severity string

// The severities. An error stops the generation: nothing is written.
const (
	SevError Severity = "error"
	SevWarn  Severity = "warn"
	SevNote  Severity = "note"
)

// Finding is one thing the reader of the report should know.
type Finding struct {
	Severity Severity
	Object   string // table, table.column or object name, empty for none
	Msg      string
}

// String renders the finding as one report line.
func (f Finding) String() string {
	if f.Object == "" {
		return string(f.Severity) + ": " + f.Msg
	}
	return string(f.Severity) + ": " + f.Object + ": " + f.Msg
}

// File is one generated model file.
type File struct {
	Name    string // file name, without a directory
	Table   string // the table it models
	Content []byte // header and source
}

// Result is the output of Generate.
type Result struct {
	Files    []File
	Findings []Finding
	// Snapshot is the schema the init snapshot is written from: the inspected
	// schema without what the models cannot declare.
	Snapshot *schema.Schema
}

// Failed reports whether a finding is an error.
func (r *Result) Failed() bool {
	for _, f := range r.Findings {
		if f.Severity == SevError {
			return true
		}
	}
	return false
}

// Options configure Generate.
type Options struct {
	// Package is the Go package name of the models.
	Package string
	// Dialect is the dialect of the inspected schema: PostgreSQL (the default)
	// or SQLite, the dialects with an inspector.
	Dialect dialect.Name
	// Namer derives the table and column names the generator will derive from
	// the Go names; tags are written only where it would derive another name.
	// Default: naming.Momo.
	Namer  naming.Namer
	Config Config
}

// Generate writes the model sources for the inspected schema s. It returns an
// error only for bad options; a database it cannot model is reported through
// Result.Findings, and Result.Failed says whether the files may be written.
func Generate(s *schema.Schema, o Options) (*Result, error) {
	if s == nil {
		return nil, errors.New("fromdb: no schema")
	}
	if o.Dialect == "" {
		o.Dialect = dialect.Postgres
	}
	if o.Dialect != dialect.Postgres && o.Dialect != dialect.SQLite {
		return nil, fmt.Errorf("fromdb: dialect %s is not supported yet: only PostgreSQL and SQLite have an inspector", o.Dialect)
	}
	if !token.IsIdentifier(o.Package) {
		return nil, fmt.Errorf("fromdb: %q is not a Go package name", o.Package)
	}
	if o.Namer == nil {
		o.Namer = naming.Momo{}
	}
	if err := o.Config.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("fromdb: copy schema: %w", err)
	}
	clean := &schema.Schema{}
	if err := json.Unmarshal(raw, clean); err != nil {
		return nil, fmt.Errorf("fromdb: copy schema: %w", err)
	}
	g := &generator{
		o: o, clean: clean, byKey: map[string]*model{}, used: map[string]bool{"Model": true},
		types: map[string]TypeRule{},
	}
	for k, r := range o.Config.Types {
		g.types[schema.NormalizeType(o.Dialect, k)] = r
	}
	if err := g.run(); err != nil {
		return nil, err
	}
	return &Result{Files: g.files, Findings: g.finds, Snapshot: clean}, nil
}

// model is the Go model of one table while it is built.
type model struct {
	t       *schema.Table
	name    string
	fields  []*field
	byCol   map[string]*field
	rels    []*relation
	used    map[string]bool
	stmts   []string
	imports map[string]bool
}

// field is one persisted column.
type field struct {
	name, goType, col string
	opts              []string
	tagged            [2]bool // has an index tag, has a unique tag
	tagSig            string  // sort, nulls and method written for the index tags
}

// relation is a relation field: a belongs_to read from a foreign key, or the
// has_one or has_many inverse of one.
type relation struct {
	name, target string
	opts         []string
	// kind is belongs_to, has_one or has_many.
	kind string
	// child, parent and fkField describe the foreign key behind the relation:
	// the referencing model, the referenced model and the Go field that holds it.
	child, parent *model
	fkField       string
	// derived is the introspect.relations key of an inverse: the parent model
	// and the name derived before any rule or collision suffix.
	derived string
}

type generator struct {
	o      Options
	clean  *schema.Schema
	finds  []Finding
	models []*model
	byKey  map[string]*model
	used   map[string]bool
	types  map[string]TypeRule
	files  []File
}

func (g *generator) add(sev Severity, obj, format string, args ...any) {
	g.finds = append(g.finds, Finding{Severity: sev, Object: obj, Msg: fmt.Sprintf(format, args...)})
}

func tableKey(schemaName, table string) string { return schemaName + "\x00" + table }

func (g *generator) run() error {
	g.cleanGlobals()
	g.collectTables()
	for _, m := range g.models {
		g.columns(m)
	}
	for _, m := range g.models {
		g.constraints(m)
	}
	if g.o.Config.Inverse {
		g.inverses()
	}
	g.globals()
	names := map[string]bool{}
	for _, m := range g.models {
		f, err := g.render(m, names)
		if err != nil {
			return err
		}
		g.files = append(g.files, f)
	}
	sort.Slice(g.files, func(i, j int) bool { return g.files[i].Name < g.files[j].Name })
	if len(g.models) == 0 {
		g.add(SevNote, "", "the database has no tables to model")
	}
	return nil
}

// cleanGlobals drops what no model can declare: PostgreSQL schemas other than
// the current one and extensions (pgvector excepted, because the generator
// derives it from the vector columns).
func (g *generator) cleanGlobals() {
	if len(g.clean.Schemas) > 0 {
		g.add(SevWarn, "", "the database uses schemas other than the current one: the models cannot declare schema namespaces, so they are left out of the snapshot")
		g.clean.Schemas = nil
	}
	var keep []schema.Extension
	var drop []string
	vec := false
	for _, t := range g.clean.Tables {
		for _, c := range t.Columns {
			base, _, _ := splitType(schema.NormalizeType(g.o.Dialect, c.Type))
			if _, ok := postgresVectors[base]; ok && base != "bit" && g.o.Dialect == dialect.Postgres {
				vec = true
			}
		}
	}
	for _, e := range g.clean.Extensions {
		if e.Name == "vector" && vec {
			e.Version = ""
			keep = append(keep, e)
			continue
		}
		drop = append(drop, e.Name)
	}
	g.clean.Extensions = keep
	if len(drop) > 0 {
		g.add(SevWarn, "", "extensions %s are in the database but cannot be declared by models: add them to migrate.exclude so the baseline ignores them", strings.Join(drop, ", "))
	}
}

func (g *generator) collectTables() {
	var kept []schema.Table
	for _, t := range g.clean.Tables {
		if g.o.Config.outsideOnly(t.Schema, t.Name) {
			// momo generate --from-db never reports this: inspection already leaves
			// such tables out, so only a direct fromdb.Generate caller sees it.
			g.add(SevNote, t.Name, "not matched by introspect.only: no model and not in the snapshot")
			continue
		}
		if g.o.Config.skipped(t.Schema, t.Name) {
			g.add(SevNote, t.Name, "skipped by introspect.skip: no model and not in the snapshot; add it to migrate.exclude so the baseline ignores the live table")
			continue
		}
		kept = append(kept, t)
	}
	g.clean.Tables = kept
	g.dropUnmodeledObjects()
	for i := range g.clean.Tables {
		t := &g.clean.Tables[i]
		m := &model{
			t: t, byCol: map[string]*field{}, imports: map[string]bool{importMomo: true},
			used: map[string]bool{"Model": true, "MomoConfig": true},
		}
		m.name = g.uniqueStruct(g.structName(t.Name))
		g.models = append(g.models, m)
		g.byKey[tableKey(t.Schema, t.Name)] = m
	}
}

// dropUnmodeledObjects removes the triggers and policies whose table has no
// model, and notes how many it left out. Inspection leaves a skipped or
// only-excluded table out of the schema but keeps its triggers and policies,
// so the table list alone does not find them; without this they would land on
// the first model.
func (g *generator) dropUnmodeledObjects() {
	have := map[string]bool{}
	for _, t := range g.clean.Tables {
		have[tableKey(t.Schema, t.Name)] = true
	}
	var trg []schema.Trigger
	for _, x := range g.clean.Triggers {
		if have[tableKey(x.Schema, x.Table)] {
			trg = append(trg, x)
		}
	}
	var pol []schema.Policy
	for _, x := range g.clean.Policies {
		if have[tableKey(x.Schema, x.Table)] {
			pol = append(pol, x)
		}
	}
	if nt, np := len(g.clean.Triggers)-len(trg), len(g.clean.Policies)-len(pol); nt+np > 0 {
		g.add(SevNote, "", "%d triggers and %d policies of tables with no model left out", nt, np)
	}
	g.clean.Triggers = trg
	g.clean.Policies = pol
}

func (g *generator) structName(table string) string {
	if r, ok := g.o.Config.Tables[table]; ok && r.Name != "" {
		return r.Name
	}
	words := splitWords(table)
	if len(words) == 0 {
		return "Table"
	}
	head := pascal(words[:len(words)-1], "")
	last := strings.ToLower(words[len(words)-1])
	var first string
	for i, c := range singulars(last) {
		name := head + titleWord(c)
		name = exported(name)
		if i == 0 {
			first = name
		}
		if g.o.Namer.TableName(name) == table {
			return name
		}
	}
	return first
}

func (g *generator) uniqueStruct(name string) string {
	out := name
	for i := 2; g.used[out]; i++ {
		out = name + strconv.Itoa(i)
	}
	g.used[out] = true
	return out
}

// tagSafe reports whether v can be written as a momo tag value: the grammar
// has no escape for a comma, and the tag sits in a double-quoted raw string.
func tagSafe(v string) bool {
	if v == "" || !utf8.ValidString(v) || v != strings.TrimSpace(v) {
		return false
	}
	for _, r := range v {
		if r == ',' || r == '"' || r == '`' || r == '\\' || r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// columnRule merges the matching introspect.columns rules in order.
func (g *generator) columnRule(table, col string) ColumnRule {
	var out ColumnRule
	for _, r := range g.o.Config.Columns {
		if !matchGlob(r.Match, table+"."+col) {
			continue
		}
		if r.Go != "" {
			out.Go, out.Import = r.Go, r.Import
		}
		if r.Name != "" {
			out.Name = r.Name
		}
		out.Tags = append(out.Tags, r.Tags...)
	}
	return out
}

// columns builds the fields of m.
func (g *generator) columns(m *model) {
	t := m.t
	pk := map[string]bool{}
	for _, c := range t.PrimaryKey {
		pk[c] = true
		// A primary key column is NOT NULL in a model. SQLite lets a key column
		// of a rowid table hold NULL unless it says NOT NULL, so the database can
		// differ: the model and the snapshot take the NOT NULL, and the baseline
		// reports the column.
		if col := colByName(t, c); col != nil && col.Nullable {
			col.Nullable = false
			g.add(SevWarn, t.Name+"."+c, "the primary key column allows NULL in the database, which a model cannot declare: it is written NOT NULL, and the baseline will report the column")
		}
	}
	for i := range t.Columns {
		// field may adjust the column (an enum column's type), and the snapshot
		// keeps that, so it gets the column in place.
		col := &t.Columns[i]
		obj := t.Name + "." + col.Name
		f, err := g.field(m, col, pk[col.Name])
		if err != nil {
			g.add(SevError, obj, "%v", err)
		} else {
			m.fields = append(m.fields, f)
			m.byCol[col.Name] = f
		}
	}
	if len(t.PrimaryKey) == 0 {
		g.add(SevNote, t.Name, "the table has no primary key")
	} else if !pkInColumnOrder(t) {
		refs, ok := m.refs(t.PrimaryKey)
		if ok {
			m.stmts = append(m.stmts, "c.PrimaryKey("+strings.Join(refs, ", ")+")")
		} else {
			g.add(SevWarn, t.Name, "the primary key uses a column the models do not have: the baseline will report the table")
		}
	}
}

func pkInColumnOrder(t *schema.Table) bool {
	var order []string
	pk := map[string]bool{}
	for _, c := range t.PrimaryKey {
		pk[c] = true
	}
	for _, c := range t.Columns {
		if pk[c.Name] {
			order = append(order, c.Name)
		}
	}
	return strings.Join(order, "\x00") == strings.Join(t.PrimaryKey, "\x00")
}

func custom(goType, imp string) GoType {
	g := GoType{Expr: goType, Import: imp}
	g.Nilable = strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[")
	return g
}

// goType resolves the Go type of a column: the enum, a column rule, a type
// rule, then the built-in mapping.
func (g *generator) goType(col *schema.Column, rule ColumnRule) (GoType, error) {
	switch {
	case col.Enum != "":
		return GoType{Expr: "string"}, nil
	case rule.Go != "":
		return custom(rule.Go, rule.Import), nil
	}
	if tr, ok := g.types[schema.NormalizeType(g.o.Dialect, col.Type)]; ok {
		return custom(tr.Go, tr.Import), nil
	}
	gt, err := MapType(g.o.Dialect, col.Type)
	if err != nil {
		return GoType{}, fmt.Errorf("%w: add an introspect.types rule for %q or an introspect.columns rule for the column", err, col.Type)
	}
	if g.o.Config.UUID == UUIDGoogle {
		gt = googleUUID(gt)
	}
	return gt, nil
}

// googleUUID swaps types.UUID and []types.UUID for github.com/google/uuid's
// UUID, which maps to uuid and uuid[] without a type tag.
func googleUUID(gt GoType) GoType {
	switch gt.Expr {
	case "types.UUID":
		return GoType{Expr: "uuid.UUID", Import: importGoogleUUID}
	case "[]types.UUID":
		return GoType{Expr: "[]uuid.UUID", Import: importGoogleUUID, Nilable: true}
	}
	return gt
}

// knownImports are the packages a Go type expression can name without a rule.
var knownImports = map[string]string{
	"types.": importTypes, "vector.": importVector, "time.": "time",
	"json.": "encoding/json", "sql.": "database/sql",
}

func exprImports(expr string) []string {
	var out []string
	for prefix, path := range knownImports {
		if strings.Contains(expr, prefix) {
			out = append(out, path)
		}
	}
	return out
}

func (g *generator) field(m *model, col *schema.Column, inPK bool) (*field, error) {
	rule := g.columnRule(m.t.Name, col.Name)
	name := rule.Name
	if name == "" {
		name = pascal(splitWords(col.Name), "Column")
	}
	name = uniqueName(m.used, name)
	if !tagSafe(col.Name) {
		return nil, fmt.Errorf("the column name %q cannot be written in a tag", col.Name)
	}
	gt, err := g.goType(col, rule)
	if err != nil {
		return nil, err
	}
	var opts []string
	if inPK {
		opts = append(opts, "pk")
	}
	switch col.Identity {
	case "always":
		opts = append(opts, "identity:always")
	case "by_default":
		opts = append(opts, "identity")
	case "serial":
		opts = append(opts, "serial")
	}
	if g.o.Namer.ColumnName(name) != col.Name {
		opts = append(opts, "column:"+col.Name)
	}
	tt, err := g.typeTags(col, gt)
	if err != nil {
		return nil, err
	}
	opts = append(opts, tt...)
	if col.Nullable && gt.Nilable {
		opts = append(opts, "nullable")
	}
	if col.Generated != "" {
		// A generated column is written by the database only.
		opts = append(opts, "read_only")
	}
	for _, e := range []struct{ key, val string }{{"collate", col.Collate}, {"default", col.Default}, {"generated", col.Generated}, {"comment", col.Comment}} {
		if e.val == "" {
			continue
		}
		virtual := e.key == "generated" && col.GeneratedVirtual
		if tagSafe(e.val) {
			opts = append(opts, e.key+":"+e.val)
			if virtual {
				opts = append(opts, "virtual")
			}
			continue
		}
		hook := strings.ToUpper(e.key[:1]) + e.key[1:]
		if virtual {
			hook = "GeneratedVirtual" // the hook marks the column VIRTUAL as well
		}
		m.stmts = append(m.stmts, "c."+hook+"(&m."+name+", "+strconv.Quote(e.val)+")")
	}
	opts = mergeTags(opts, rule.Tags)
	if dbDefaultFits(g.o.Dialect, col, opts) || (g.o.Config.DBDefault == "all" && dbDefaultAny(g.o.Dialect, col, opts)) {
		opts = append(opts, "dbdefault")
	}
	goType := gt.Expr
	if col.Nullable && !gt.Nilable {
		goType = "*" + goType
	}
	if !col.Nullable && slices.Contains(opts, "dbdefault") && dbdefault.NonZeroConst(gt.Expr, col.Default) {
		// A bool or number whose constant default is true or non-zero: a plain
		// field would replace an explicit false or 0 with the default, so the
		// field is a pointer (nil takes the default, any value is written).
		goType = "*" + goType
	}
	if gt.Note != "" {
		g.add(SevNote, m.t.Name+"."+col.Name, "%s", gt.Note)
	}
	if gt.Import != "" {
		m.imports[gt.Import] = true
	}
	for _, p := range exprImports(gt.Expr) {
		m.imports[p] = true
	}
	return &field{name: name, goType: goType, col: col.Name, opts: opts}, nil
}

// typeTags decides how the column type is spelled: nothing when the Go type
// maps to it, size, dim, precision and scale where those give it, the enum
// tag for an enum, and the type tag otherwise.
func (g *generator) typeTags(col *schema.Column, gt GoType) ([]string, error) {
	if col.Enum != "" {
		if !tagSafe(col.Enum) {
			return nil, fmt.Errorf("the enum name %q cannot be written in a tag", col.Enum)
		}
		if g.o.Dialect != dialect.SQLite {
			// A PostgreSQL enum column has the enum as its type. A SQLite one is a
			// TEXT column with a CHECK, and the enum tag leaves it TEXT.
			col.Type = col.Enum
		}
		return []string{"enum:" + col.Enum}, nil
	}
	want := schema.NormalizeType(g.o.Dialect, col.Type)
	base := gt.Expr
	if got, err := typemap.Map(base, g.o.Dialect, tags.Tags{Size: gt.Size, Dim: gt.Dim}); err == nil && schema.NormalizeType(g.o.Dialect, got) == want {
		var out []string
		if gt.Size > 0 {
			out = append(out, "size:"+strconv.Itoa(gt.Size))
		}
		if gt.Dim > 0 {
			out = append(out, "dim:"+strconv.Itoa(gt.Dim))
		}
		return out, nil
	}
	if p, s, scaled, ok := numericArgs(want); ok {
		if decimalCarrier(base) && p > 0 && !(scaled && s == 0) {
			out := []string{"precision:" + strconv.Itoa(p)}
			if s > 0 {
				out = append(out, "scale:"+strconv.Itoa(s))
			}
			return out, nil
		}
	}
	if !tagSafe(col.Type) {
		return nil, fmt.Errorf("the column type %q cannot be written in a tag; add an introspect.types rule with a Go type that has precision and scale", col.Type)
	}
	return []string{"type:" + col.Type}, nil
}

// decimalCarrier reports whether the generator accepts precision and scale on
// the Go type.
func decimalCarrier(goType string) bool {
	return goType == "string" || goType == "types.Numeric" || goType == "float32" || goType == "float64" || strings.HasSuffix(goType, "decimal.Decimal")
}

// numericArgs parses numeric(p) and numeric(p,s).
func numericArgs(normalized string) (p, s int, scaled, ok bool) {
	base, args, rest := splitType(normalized)
	if base != "numeric" || args == "" || rest != "" {
		return 0, 0, false, false
	}
	ps, ss, scaled := strings.Cut(args, ",")
	p, err := strconv.Atoi(ps)
	if err != nil {
		return 0, 0, false, false
	}
	if scaled {
		if s, err = strconv.Atoi(ss); err != nil {
			return 0, 0, false, false
		}
	}
	return p, s, scaled, true
}

// mergeTags adds the user's options, replacing a generated option of the same
// key.
func mergeTags(generated, user []string) []string {
	key := func(o string) string { k, _, _ := strings.Cut(o, ":"); return k }
	out := generated
	for _, u := range user {
		out = removeKey(out, key(u))
		out = append(out, u)
	}
	return out
}

// dbDefaultFits reports whether the field of col gets the dbdefault tag: a
// NOT NULL column whose database default is computed (a function call such as
// gen_random_uuid() or now(), or CURRENT_TIMESTAMP), which a Go zero value
// would otherwise overwrite. A constant default is left alone, as is a column
// whose other tags already decide how it is written. A sequence default
// (nextval(...)) on a column that is not identity or serial counts as
// computed, so a zero id is no longer written over it. Only PostgreSQL and
// SQLite qualify: the tag needs INSERT ... RETURNING to read the value back,
// which plain MySQL lacks.
func dbDefaultFits(d dialect.Name, col *schema.Column, opts []string) bool {
	return computedDefault(col.Default) && dbDefaultAny(d, col, opts)
}

// dbDefaultAny reports whether the field of col could carry dbdefault whatever
// its default is: a NOT NULL PostgreSQL or SQLite column with a default that no other
// tag already owns. introspect.dbdefault: all tags every such column, constant
// defaults (DEFAULT 1) included. A nullable column never qualifies, because a
// nil pointer would stop meaning NULL.
func dbDefaultAny(d dialect.Name, col *schema.Column, opts []string) bool {
	if (d != dialect.Postgres && d != dialect.SQLite) || col.Nullable || col.Identity != "" || strings.TrimSpace(col.Default) == "" {
		return false
	}
	for _, o := range opts {
		switch k, _, _ := strings.Cut(o, ":"); k {
		case "identity", "serial", "read_only", "version", "soft_delete", "created_at", "updated_at", "idgen", "dbdefault":
			return false
		}
	}
	return true
}

// computedDefault reports whether a default expression is evaluated by the
// database per row rather than a literal.
func computedDefault(d string) bool {
	d = strings.ToLower(strings.TrimSpace(d))
	if d == "" || strings.HasPrefix(d, "'") || strings.HasPrefix(d, "-") || (d[0] >= '0' && d[0] <= '9') {
		return false
	}
	switch d {
	case "true", "false", "null":
		return false
	}
	return strings.Contains(d, "(") || strings.HasPrefix(d, "current_") || strings.HasPrefix(d, "localtime")
}

func removeKey(opts []string, k string) []string {
	var out []string
	for _, o := range opts {
		if kk, _, _ := strings.Cut(o, ":"); kk != k {
			out = append(out, o)
		}
	}
	return out
}

func uniqueName(used map[string]bool, name string) string {
	out := name
	for i := 2; used[out]; i++ {
		out = name + strconv.Itoa(i)
	}
	used[out] = true
	return out
}

// ref is the companion reference to the field of a column.
func (m *model) ref(col string) (string, bool) {
	f, ok := m.byCol[col]
	if !ok {
		return "", false
	}
	return "&m." + f.name, true
}

func (m *model) refs(cols []string) ([]string, bool) {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		r, ok := m.ref(c)
		if !ok {
			return nil, false
		}
		out = append(out, r)
	}
	return out, true
}

func (g *generator) constraints(m *model) {
	t := m.t
	var uniques []schema.Unique
	for _, u := range t.Uniques {
		refs, ok := m.refs(u.Columns)
		if !ok {
			g.add(SevWarn, t.Name, "unique constraint %q uses a column the models do not have: left out of the models and the snapshot", u.Name)
			continue
		}
		m.stmts = append(m.stmts, "c.Unique("+strconv.Quote(u.Name)+", "+strings.Join(refs, ", ")+")")
		uniques = append(uniques, u)
	}
	t.Uniques = uniques
	for _, ck := range t.Checks {
		m.stmts = append(m.stmts, "c.Check("+strconv.Quote(ck.Name)+", "+strconv.Quote(ck.Expr)+")")
	}
	var fks []schema.ForeignKey
	for _, fk := range t.ForeignKeys {
		if g.foreignKey(m, fk) {
			fks = append(fks, fk)
		}
	}
	t.ForeignKeys = fks
	var ixs []schema.Index
	for _, ix := range t.Indexes {
		if g.index(m, ix) {
			ixs = append(ixs, ix)
		}
	}
	t.Indexes = ixs
	g.tableObjects(m)
}

// fkNameOK is what a constraint name may hold to be written as an fk_name tag
// value: no comma, colon, quote or space.
var fkNameOK = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]{0,62}$`)

var fkActions = map[string]bool{"": true, "CASCADE": true, "RESTRICT": true, "SET NULL": true, "SET DEFAULT": true}

// foreignKey declares fk as a belongs_to relation when the generator can
// derive exactly this constraint from it. It reports whether it did.
func (g *generator) foreignKey(m *model, fk schema.ForeignKey) bool {
	t := m.t
	obj := t.Name + " foreign key " + fk.Name
	drop := func(why string) bool {
		g.add(SevWarn, obj, "%s: the models cannot declare this constraint, so it is left out of the models and the snapshot and the baseline will report it", why)
		return false
	}
	ns, name := t.Schema, fk.RefTable
	if i := strings.IndexByte(name, '.'); i >= 0 {
		ns, name = name[:i], name[i+1:]
	}
	target := g.byKey[tableKey(ns, name)]
	if target == nil {
		return drop("it references " + fk.RefTable + ", which has no model")
	}
	if len(fk.Columns) != 1 || len(fk.RefColumns) != 1 {
		return g.compositeForeignKey(m, target, fk, drop)
	}
	if len(target.t.PrimaryKey) != 1 || target.t.PrimaryKey[0] != fk.RefColumns[0] {
		return drop("it does not reference the single-column primary key of " + fk.RefTable)
	}
	f := m.byCol[fk.Columns[0]]
	if f == nil {
		return drop("its column is not modeled")
	}
	if !fkActions[fk.OnDelete] || !fkActions[fk.OnUpdate] {
		return drop("its referential action is not supported")
	}
	lc, tc := colByName(t, fk.Columns[0]), colByName(target.t, fk.RefColumns[0])
	if lc == nil || tc == nil || schema.NormalizeType(g.o.Dialect, lc.Type) != schema.NormalizeType(g.o.Dialect, tc.Type) {
		return drop("the column types differ")
	}
	opts := []string{"belongs_to", "fk:" + f.name}
	if want := t.Name + "_" + fk.Columns[0] + "_fkey"; fk.Name != want {
		if !fkNameOK.MatchString(fk.Name) {
			return drop("its name " + fk.Name + " cannot be written in a struct tag")
		}
		opts = append(opts, "fk_name:"+fk.Name)
	}
	for _, a := range []struct{ key, val string }{{"on_delete", fk.OnDelete}, {"on_update", fk.OnUpdate}} {
		if a.val != "" {
			opts = append(opts, a.key+":"+strings.ToLower(strings.ReplaceAll(a.val, " ", "_")))
		}
	}
	base := strings.TrimSuffix(strings.TrimSuffix(f.name, "ID"), "Id")
	rel := ""
	for _, c := range []string{base, target.name, target.name + "Ref"} {
		if c != "" && !m.used[c] {
			rel = c
			break
		}
	}
	if rel == "" {
		rel = target.name
	}
	rel = uniqueName(m.used, rel)
	m.rels = append(m.rels, &relation{name: rel, target: target.name, opts: opts, kind: "belongs_to", child: m, parent: target, fkField: f.name})
	return true
}

// compositeForeignKey declares a multi-column foreign key with a
// c.ForeignKey statement. It reports whether it did; drop explains why not.
func (g *generator) compositeForeignKey(m *model, target *model, fk schema.ForeignKey, drop func(string) bool) bool {
	if len(fk.Columns) != len(fk.RefColumns) || len(fk.Columns) == 0 {
		return drop("its column lists differ in length")
	}
	if !fkActions[fk.OnDelete] || !fkActions[fk.OnUpdate] {
		return drop("its referential action is not supported")
	}
	refs, ok := m.refs(fk.Columns)
	if !ok {
		return drop("a column is not modeled")
	}
	for _, c := range fk.RefColumns {
		if colByName(target.t, c) == nil {
			return drop("the referenced column " + c + " is not modeled")
		}
	}
	s := "c.ForeignKey(" + strconv.Quote(fk.Name) + ", []any{" + strings.Join(refs, ", ") + "}, " +
		strconv.Quote(fk.RefTable) + ", []string{" + quoteAll(fk.RefColumns) + "})"
	if fk.OnDelete != "" {
		s += ".OnDelete(" + strconv.Quote(fk.OnDelete) + ")"
	}
	if fk.OnUpdate != "" {
		s += ".OnUpdate(" + strconv.Quote(fk.OnUpdate) + ")"
	}
	m.stmts = append(m.stmts, s)
	return true
}

func quoteAll(in []string) string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strconv.Quote(s)
	}
	return strings.Join(out, ", ")
}

func colByName(t *schema.Table, name string) *schema.Column {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
}

// index declares ix through a tag or a companion statement. It reports
// whether the models can declare it.
func (g *generator) index(m *model, ix schema.Index) bool {
	t := m.t
	obj := t.Name + " index " + ix.Name
	drop := func(why string) bool {
		g.add(SevWarn, obj, "%s: the models cannot declare this index, so it is left out of the models and the snapshot and the baseline will report it", why)
		return false
	}
	if len(ix.Parts) == 0 {
		return drop("it has no key")
	}
	cols, exprs := 0, 0
	for _, p := range ix.Parts {
		switch {
		case p.Column != "" && p.Expr == "":
			cols++
		case p.Column == "" && p.Expr != "":
			exprs++
		}
	}
	if cols+exprs != len(ix.Parts) {
		return drop("a key part is neither a column nor an expression")
	}
	// An index with any expression is written with IndexExpr, which takes the
	// expressions and the plain column names as strings.
	allExpr := exprs > 0
	if allExpr {
		for _, p := range ix.Parts {
			if p.Desc || p.Nulls != "" || p.OpClass != "" {
				return drop("a key with an order or operator class has no declaration next to an expression")
			}
			if p.Column != "" && (!plainIdent(p.Column) || m.byCol[p.Column] == nil) {
				return drop("it uses a column the models do not have, or one whose name is not a plain identifier")
			}
		}
	} else if _, ok := m.refs(partColumns(ix.Parts)); !ok {
		return drop("it uses a column the models do not have")
	}
	method := ix.Method
	if method == "btree" {
		method = ""
	}
	if !allExpr && g.tagIndex(m, ix, method) {
		return true
	}
	m.stmts = append(m.stmts, indexStmt(m, ix, method, allExpr, g.o.Dialect))
	return true
}

// plainIdent reports whether s is a lower-case identifier that needs no
// quoting, so an IndexExpr string naming it is read back as the column.
func plainIdent(s string) bool {
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

func partColumns(parts []schema.IndexPart) []string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = p.Column
	}
	return out
}

// tagIndex writes a plain single-column index as the index or unique tag of
// its field. It reports whether it could.
func (g *generator) tagIndex(m *model, ix schema.Index, method string) bool {
	if len(ix.Parts) != 1 || ix.Where != "" || len(ix.With) > 0 {
		return false
	}
	p := ix.Parts[0]
	if p.OpClass != "" || !tagSafe(ix.Name) || (method != "" && !tagSafe(method)) {
		return false
	}
	f := m.byCol[p.Column]
	if f == nil {
		return false
	}
	slot, key := 0, "index"
	if ix.Unique {
		slot, key = 1, "unique"
	}
	var extra []string
	if p.Desc {
		extra = append(extra, "sort:desc")
	}
	if p.Nulls != "" {
		extra = append(extra, "nulls:"+p.Nulls)
	}
	if method != "" {
		extra = append(extra, "method:"+method)
	}
	sig := strings.Join(extra, ",")
	if f.tagged[slot] || (f.tagged[1-slot] && f.tagSig != sig) {
		return false
	}
	if ix.Name != defaultIndexName(m.t.Name, p.Column, ix.Unique) {
		key += ":" + ix.Name
	}
	f.opts = append(f.opts, key)
	if !f.tagged[1-slot] {
		f.opts = append(f.opts, extra...)
		f.tagSig = sig
	}
	f.tagged[slot] = true
	return true
}

// defaultIndexName mirrors the generator: <table>_<column>_idx, and
// <table>_<column>_key for a unique index.
func defaultIndexName(table, column string, unique bool) string {
	if unique {
		return table + "_" + column + "_key"
	}
	return table + "_" + column + "_idx"
}

func indexStmt(m *model, ix schema.Index, method string, allExpr bool, d dialect.Name) string {
	var args []string
	call := "c.Index"
	// PostgreSQL normalizes the btree method away, so a unique index with no
	// option is written c.Unique(...).Method("btree") to tell it from a
	// constraint. Other dialects keep the method as written and inspect none,
	// so there the unique index is c.Index(...).Unique().
	uniqueOpt := ix.Unique && !allExpr && d != dialect.Postgres
	if allExpr {
		call = "c.IndexExpr"
		for _, p := range ix.Parts {
			if p.Column != "" {
				args = append(args, strconv.Quote(p.Column))
			} else {
				args = append(args, strconv.Quote(p.Expr))
			}
		}
	} else {
		if ix.Unique && !uniqueOpt {
			call = "c.Unique"
		}
		for _, p := range ix.Parts {
			r, _ := m.ref(p.Column)
			args = append(args, r)
		}
	}
	s := call + "(" + strconv.Quote(ix.Name)
	if len(args) > 0 {
		s += ", " + strings.Join(args, ", ")
	}
	s += ")"
	opts := 0
	if (allExpr || uniqueOpt) && ix.Unique {
		s += ".Unique()"
		opts++
	}
	if ix.Where != "" {
		s += ".Where(" + strconv.Quote(ix.Where) + ")"
		opts++
	}
	if method != "" {
		s += ".Method(" + strconv.Quote(method) + ")"
		opts++
	}
	for _, kv := range ix.With {
		s += ".With(" + strconv.Quote(kv.Key) + ", " + strconv.Quote(kv.Value) + ")"
		opts++
	}
	for _, p := range ix.Parts {
		if p.Column == "" {
			continue
		}
		r, _ := m.ref(p.Column)
		if p.OpClass != "" {
			s += ".Opclass(" + r + ", " + strconv.Quote(p.OpClass) + ")"
			opts++
		}
		if p.Desc {
			s += ".Desc(" + r + ")"
			opts++
		}
		if p.Nulls != "" {
			s += ".Nulls(" + r + ", " + strconv.Quote(p.Nulls) + ")"
			opts++
		}
	}
	if ix.Unique && !allExpr && !uniqueOpt && opts == 0 {
		// A unique with no option reads as a constraint; the btree method makes
		// it the unique index it is.
		s += `.Method("btree")`
	}
	return s
}

// tableObjects declares the row-level security and partition settings of m.
func (g *generator) tableObjects(m *model) {
	t := m.t
	if t.RLS != nil {
		m.addLiteral("RLS", *t.RLS)
	}
	if t.Partition != nil {
		m.addLiteral("Partition", *t.Partition)
	}
	if t.PartitionOf != nil {
		m.addLiteral("PartitionOf", *t.PartitionOf)
	}
}

// addLiteral appends c.<method>(schema.<Type>{...}) to the companion.
func (m *model) addLiteral(method string, v any) {
	lit, err := structLiteral(v)
	if err != nil {
		return
	}
	m.imports[importSchema] = true
	m.stmts = append(m.stmts, "c."+method+"("+lit+")")
}

// structLiteral renders a schema struct as the keyed literal of strings,
// bools and string slices that the companion reader accepts.
func structLiteral(v any) (string, error) {
	rv := reflect.ValueOf(v)
	rt := rv.Type()
	var parts []string
	for i := 0; i < rt.NumField(); i++ {
		f := rv.Field(i)
		if f.IsZero() {
			continue
		}
		var lit string
		switch f.Kind() {
		case reflect.String:
			lit = strconv.Quote(f.String())
		case reflect.Bool:
			lit = strconv.FormatBool(f.Bool())
		case reflect.Slice:
			if f.Type().Elem().Kind() != reflect.String {
				return "", fmt.Errorf("field %s has no literal form", rt.Field(i).Name)
			}
			els := make([]string, f.Len())
			for j := range els {
				els[j] = strconv.Quote(f.Index(j).String())
			}
			lit = "[]string{" + strings.Join(els, ", ") + "}"
		default:
			return "", fmt.Errorf("field %s has no literal form", rt.Field(i).Name)
		}
		parts = append(parts, rt.Field(i).Name+": "+lit)
	}
	return "schema." + rt.Name() + "{" + strings.Join(parts, ", ") + "}", nil
}

// globals declares the objects that belong to no table on the first model
// (enums and views) and the ones that belong to a table on that table's model
// (triggers, policies, and the functions a table uses).
func (g *generator) globals() {
	if len(g.models) == 0 {
		return
	}
	host := g.models[0]
	for _, e := range g.clean.Enums {
		args := []string{strconv.Quote(e.Name)}
		for _, v := range e.Values {
			args = append(args, strconv.Quote(v))
		}
		host.stmts = append(host.stmts, "c.Enum("+strings.Join(args, ", ")+")")
	}
	for i := range g.clean.Views {
		v := &g.clean.Views[i]
		if len(v.Indexes) > 0 {
			g.add(SevWarn, v.Name, "the indexes of a materialized view have no declaration: left out of the models and the snapshot")
			v.Indexes = nil
		}
		host.global("View", *v, g, v.Name)
	}
	g.functions(host)
	for _, x := range g.clean.Triggers {
		g.owner(x.Schema, x.Table).global("Trigger", x, g, x.Name)
	}
	for _, x := range g.clean.Policies {
		g.owner(x.Schema, x.Table).global("Policy", x, g, x.Name)
	}
}

// owner returns the model of a table. Every trigger and policy has one:
// dropUnmodeledObjects removed the others.
func (g *generator) owner(schemaName, table string) *model {
	return g.byKey[tableKey(schemaName, table)]
}

func (m *model) global(method string, v any, g *generator, name string) {
	lit, err := structLiteral(v)
	if err != nil {
		g.add(SevError, name, "%v", err)
		return
	}
	m.imports[importSchema] = true
	m.stmts = append(m.stmts, "c."+method+"("+lit+")")
}

// render writes the source file of m.
func (g *generator) render(m *model, names map[string]bool) (File, error) {
	var b strings.Builder
	b.WriteString("package " + g.o.Package + "\n\n")
	b.WriteString(importBlock(m.imports))
	marker := m.markerTags(g)
	fmt.Fprintf(&b, "// %s models the table %s.\n", m.name, strconv.Quote(m.t.Name))
	fmt.Fprintf(&b, "type %s struct {\n", m.name)
	b.WriteString("\tmomo.Model")
	if len(marker) > 0 {
		b.WriteString(" `momo:\"" + strings.Join(marker, ",") + "\"`")
	}
	b.WriteString("\n")
	for _, f := range m.fields {
		b.WriteString("\t" + f.name + " " + f.goType)
		if len(f.opts) > 0 {
			b.WriteString(" `momo:\"" + strings.Join(f.opts, ",") + "\"`")
		}
		b.WriteString("\n")
	}
	if len(m.rels) > 0 {
		b.WriteString("\n")
	}
	for _, r := range m.rels {
		typ := "*" + r.target
		if r.kind == "has_many" {
			typ = "[]" + r.target
		}
		b.WriteString("\t" + r.name + " " + typ + " `momo:\"" + strings.Join(r.opts, ",") + "\"`\n")
	}
	b.WriteString("}\n")
	if len(m.stmts) > 0 {
		fmt.Fprintf(&b, "\n// MomoConfig declares what a tag cannot: the other indexes, constraints, enums and defaults.\nfunc (m *%s) MomoConfig(c *momo.Config) {\n", m.name)
		for _, s := range m.stmts {
			b.WriteString("\t" + s + "\n")
		}
		b.WriteString("}\n")
	}
	body, err := format.Source([]byte(b.String()))
	if err != nil {
		return File{}, fmt.Errorf("fromdb: format the model of %s: %w\n%s", m.t.Name, err, b.String())
	}
	return File{Name: fileName(m.t, names), Table: m.t.Name, Content: WithHeader(body)}, nil
}

// markerTags are the options of the momo.Model marker: the table and schema
// the namer would not derive, and the options of an introspect.tables rule.
func (m *model) markerTags(g *generator) []string {
	var out []string
	if g.o.Namer.TableName(m.name) != m.t.Name {
		out = append(out, "table:"+m.t.Name)
	}
	if m.t.Schema != "" {
		out = append(out, "schema:"+m.t.Schema)
	}
	if m.t.Autoincrement {
		out = append(out, "autoincrement")
	}
	if m.t.WithoutRowID {
		out = append(out, "without_rowid")
	}
	if m.t.Strict {
		out = append(out, "strict")
	}
	if r, ok := g.o.Config.Tables[m.t.Name]; ok {
		out = mergeTags(out, r.Tags)
	}
	for _, o := range out {
		_, v, _ := strings.Cut(o, ":")
		if strings.HasPrefix(o, "table:") || strings.HasPrefix(o, "schema:") {
			if !tagSafe(v) {
				g.add(SevError, m.t.Name, "the table or schema name %q cannot be written in a tag", v)
			}
		}
	}
	return out
}

// importBlock renders the import declaration: standard library first.
func importBlock(imports map[string]bool) string {
	var std, other []string
	for p := range imports {
		first, _, _ := strings.Cut(p, "/")
		if strings.Contains(first, ".") {
			other = append(other, p)
		} else {
			std = append(std, p)
		}
	}
	sort.Strings(std)
	sort.Strings(other)
	var b strings.Builder
	b.WriteString("import (\n")
	for _, p := range std {
		b.WriteString("\t" + strconv.Quote(p) + "\n")
	}
	if len(std) > 0 && len(other) > 0 {
		b.WriteString("\n")
	}
	for _, p := range other {
		b.WriteString("\t" + strconv.Quote(p) + "\n")
	}
	b.WriteString(")\n\n")
	return b.String()
}

// fileName is <schema_>table_model.go, lower case, with every character
// outside [a-z0-9_] replaced; the _model suffix keeps it clear of the _test
// and GOOS/GOARCH file name rules.
func fileName(t *schema.Table, taken map[string]bool) string {
	raw := strings.ToLower(t.Name)
	if t.Schema != "" {
		raw = strings.ToLower(t.Schema) + "_" + raw
	}
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	base := b.String() + "_model"
	name := base + ".go"
	for i := 2; taken[name]; i++ {
		name = base + strconv.Itoa(i) + ".go"
	}
	taken[name] = true
	return name
}

// headerPrefix starts the first line of every generated model file.
const headerPrefix = "// momo:fromdb sha256="

// WithHeader prepends the header line, which carries the hash of body, so a
// later run can tell whether the file was edited by hand.
func WithHeader(body []byte) []byte {
	sum := sha256.Sum256(body)
	var b bytes.Buffer
	b.WriteString(headerPrefix + hex.EncodeToString(sum[:]) + "\n")
	b.WriteString("// Written by momo generate --from-db. A re-run keeps this file if you edit it, and reports a conflict when the database changed the model.\n\n")
	b.Write(body)
	return b.Bytes()
}

// SplitHeader returns the body of a file written by WithHeader and the hash
// the header records. ok is false for any other file.
func SplitHeader(content []byte) (body []byte, hash string, ok bool) {
	line, rest, found := bytes.Cut(content, []byte("\n"))
	if !found || !bytes.HasPrefix(line, []byte(headerPrefix)) {
		return nil, "", false
	}
	_, body, found = bytes.Cut(rest, []byte("\n\n"))
	if !found {
		return nil, "", false
	}
	return body, string(bytes.TrimPrefix(line, []byte(headerPrefix))), true
}

// Edited reports whether the body of a file written by WithHeader no longer
// matches the hash in its header.
func Edited(content []byte) (edited, generated bool) {
	body, hash, ok := SplitHeader(content)
	if !ok {
		return false, false
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]) != hash, true
}
