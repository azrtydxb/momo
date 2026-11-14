// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"go/token"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/gen/typemap"
	"github.com/azrtydxb/momo/schema"
)

// BuildSchema builds the normalized schema of the discovered models: tag
// effects, the merged MomoConfig companions, foreign keys and many-to-many
// join tables. Problems are positioned diagnostics (a field's position for a
// tag, the model's position for a MomoConfig entry); the schema is still
// returned so one run reports every problem.
//
// The schema's dialect is the first entry of cfg.Dialects (PostgreSQL when
// empty). Dialect diagnostics ("<tag> is not supported on <dialect>") are
// produced for each explicitly listed dialect only; an empty list means the
// caller has not chosen dialects yet, so nothing is dialect-checked.
//
// The output is canonical: models are processed in sorted order, every named
// collection is sorted by schema.Sort, and column order is declaration order.
func BuildSchema(models []*Model, cfg Config) (*schema.Schema, []Diagnostic) {
	s, _, diags := buildSchema(models, cfg)
	return s, diags
}

// buildSchema is BuildSchema that also returns the table built for each view
// model. The tables are not part of the schema (a view is not created as a
// table) but the metadata emitter needs their columns.
func buildSchema(models []*Model, cfg Config) (*schema.Schema, map[*Model]*schema.Table, []Diagnostic) {
	b := &schemaBuilder{
		viewTab: map[*Model]*schema.Table{},
		namer:   cfg.Naming,
		typeDef: cfg.TypeDefaults,
		primary: dialect.Postgres,
		index:   map[*Model]int{},
		enums:   map[string]enumDecl{},
		jointab: map[string]bool{},
		tableBy: map[string]*Model{},
		seen:    map[string]any{},
	}
	if b.namer == nil {
		b.namer = naming.Momo{}
	}
	if len(cfg.Dialects) > 0 {
		b.primary = cfg.Dialects[0]
	}
	for _, k := range slices.Sorted(maps.Keys(cfg.TypeDefaults)) {
		if k != "time.Time" {
			b.diag(token.Position{}, fmt.Sprintf("TypeDefaults: %q cannot be overridden; only time.Time is supported", k))
		}
	}
	for _, d := range cfg.Dialects {
		if !slices.Contains(b.dialects, d) {
			b.dialects = append(b.dialects, d)
		}
	}
	s := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: b.primary, Tables: []schema.Table{}}
	b.s = s

	sorted := slices.Clone(models)
	sort.SliceStable(sorted, func(i, j int) bool {
		x, y := sorted[i], sorted[j]
		if x.PkgPath != y.PkgPath {
			return x.PkgPath < y.PkgPath
		}
		return x.GoName < y.GoName
	})
	for _, m := range sorted {
		if m.IsView {
			continue
		}
		if prev, dup := b.tableBy[tableKey(m)]; dup {
			b.diag(m.Pos, fmt.Sprintf("table %q is produced by models %s and %s", m.Table, prev.GoName, m.GoName))
			continue
		}
		b.tableBy[tableKey(m)] = m
		b.addModel(m)
	}
	// A view model reads a view its companion declares. It is not a table: it
	// gets no CREATE TABLE and no table key, so no foreign key refers to it
	// and its relations add no constraint. It still may not share its name
	// with a table model or with another view model.
	viewBy := map[string]*Model{}
	for _, m := range sorted {
		if !m.IsView {
			continue
		}
		prev, dup := b.tableBy[tableKey(m)]
		if !dup {
			prev, dup = viewBy[tableKey(m)]
		}
		if dup {
			b.diag(m.Pos, fmt.Sprintf("table %q is produced by models %s and %s", m.Table, prev.GoName, m.GoName))
			continue
		}
		viewBy[tableKey(m)] = m
		b.addModel(m)
	}
	for _, m := range sorted {
		if b.tableBy[tableKey(m)] == m {
			b.addRelations(m)
		}
	}
	b.finish()
	s.Sort()
	sort.SliceStable(b.diags, func(i, j int) bool {
		x, y := b.diags[i].Pos, b.diags[j].Pos
		if x.Filename != y.Filename {
			return x.Filename < y.Filename
		}
		if x.Line != y.Line {
			return x.Line < y.Line
		}
		return x.Column < y.Column
	})
	return s, b.viewTab, dedupeDiagnostics(b.diags)
}

func tableKey(m *Model) string { return m.Schema + "\x00" + m.Table }

type enumDecl struct {
	enum schema.Enum
	pos  token.Position
}

// enumUse is a column that names an enum, checked once every model has
// contributed its declarations.
type enumUse struct {
	name string
	pos  token.Position
}

// feature is a dialect-restricted construct found while building a table.
type feature struct {
	name string // the diagnostic name: vector, halfvec, hnsw, ...
	pos  token.Position
}

type schemaBuilder struct {
	s        *schema.Schema
	namer    naming.Namer
	typeDef  map[string]string
	primary  dialect.Name
	dialects []dialect.Name
	diags    []Diagnostic
	index    map[*Model]int
	enums    map[string]enumDecl
	enumUses []enumUse
	jointab  map[string]bool
	tableBy  map[string]*Model
	vector   bool
	feats    []feature
	// names is the index and unique constraint namespace of the table being
	// built, mapping a name to a description of its declarer.
	names map[string]string
	// seen holds managed objects and join tables already merged, so an
	// identical repeat is kept once.
	seen map[string]any
	// viewTab holds the table built for each view model, kept out of the
	// schema.
	viewTab map[*Model]*schema.Table
}

func (b *schemaBuilder) diag(pos token.Position, msg string) {
	b.diags = append(b.diags, Diagnostic{Pos: pos, Msg: msg})
}

// addModel builds the table of one model.
func (b *schemaBuilder) addModel(m *Model) {
	t := schema.Table{Schema: m.Schema, Name: m.Table, Columns: []schema.Column{}}
	for _, f := range m.Fields {
		col := b.applyColumnTags(m, f)
		if f.Tags.PK {
			t.PrimaryKey = append(t.PrimaryKey, f.Column)
			if col.Nullable {
				b.diag(f.Pos, fmt.Sprintf("primary key column %q must not be nullable", f.Column))
			}
		}
		if f.Tags.Check != "" {
			t.Checks = append(t.Checks, schema.Check{Name: m.Table + "_" + f.Column + "_check", Expr: f.Tags.Check})
		}
		t.Columns = append(t.Columns, col)
	}
	b.names = map[string]string{}
	b.applyIndexTags(m, &t)
	b.applyCompanion(m, &t)
	if m.IsView {
		// The companion's views, functions and policies are merged above; the
		// table is not part of the schema, only the emitter reads it.
		b.viewTab[m] = &t
		b.dialectCheck()
		return
	}
	b.index[m] = len(b.s.Tables)
	b.s.Tables = append(b.s.Tables, t)
	b.dialectCheck()
}

// applyColumnTags maps the column-level tags of f to a schema column.
func (b *schemaBuilder) applyColumnTags(m *Model, f *Field) schema.Column {
	tg := f.Tags
	core, ptr := strings.CutPrefix(f.GoType, "*")
	nullType := false
	if strings.HasPrefix(core, "sql.Null") {
		nullType = true
	}
	nullable := ptr || f.Nullable || nullType || tg.Nullable
	if dbDefaultPointer(f.GoType, tg) {
		nullable = false // nil means "use the database default": the column is NOT NULL
	}
	if tg.NotNull {
		if nullable {
			b.diag(f.Pos, fmt.Sprintf("field %s: notnull conflicts with a nullable Go type or the nullable tag", f.GoName))
		}
		nullable = false
	}
	col := schema.Column{Name: f.Column, Nullable: nullable, Default: tg.Default, Generated: tg.Generated, GeneratedVirtual: tg.GeneratedVirtual, Collate: tg.Collate, Comment: tg.Comment}
	if tg.Identity {
		col.Identity = "by_default"
		if tg.IdentityAlways {
			col.Identity = "always"
			b.applyIdentityAlways(f)
		}
	}
	col.Type = b.columnType(f, core)
	if tg.Serial {
		b.applySerial(f, &col)
	}
	if tg.DBDefault {
		b.checkDBDefault(m, f)
	}
	if tg.Enum != "" {
		col.Enum = tg.Enum
		b.enumUses = append(b.enumUses, enumUse{name: tg.Enum, pos: f.Pos})
	}
	return col
}

// checkDBDefault reports a dbdefault field whose other tags already decide how
// the column is written: the database generates identity and serial values,
// read-only fields are never written, and the version, soft-delete, timestamp
// and idgen columns are filled by Momo.
func (b *schemaBuilder) checkDBDefault(m *Model, f *Field) {
	tg := f.Tags
	conflict := ""
	switch {
	case tg.Identity:
		conflict = "identity"
	case tg.Serial:
		conflict = "serial"
	case f.Access == ReadOnly:
		conflict = "read_only"
	case tg.Version:
		conflict = "version"
	case tg.SoftDelete != "":
		conflict = "soft_delete"
	case tg.CreatedAt:
		conflict = "created_at"
	case tg.UpdatedAt:
		conflict = "updated_at"
	case tg.IDGen != "" || (m.Config != nil && m.Config.IDGens[f.Column] != ""):
		conflict = "idgen"
	}
	if conflict != "" {
		b.diag(f.Pos, fmt.Sprintf("field %s: dbdefault conflicts with %s", f.GoName, conflict))
	}
	if core, _ := strings.CutPrefix(f.GoType, "*"); strings.HasPrefix(core, "sql.Null") || tg.Nullable {
		b.diag(f.Pos, fmt.Sprintf("field %s: dbdefault on a nullable field: nil means use the database default, so Model can never write NULL (use Values with SetNull); use a plain type, or a pointer to one on a NOT NULL column", f.GoName))
	}
}

// dbDefaultPointer reports whether a dbdefault field is a pointer to a plain
// type: its column is NOT NULL and the pointer says whether the caller set the
// value (nil takes the database default; any other value, false and 0
// included, is written).
func dbDefaultPointer(goType string, tg tags.Tags) bool {
	core, ptr := strings.CutPrefix(goType, "*")
	return tg.DBDefault && ptr && !tg.Nullable && !strings.HasPrefix(core, "sql.Null")
}

// applyIdentityAlways refuses identity:always for a MySQL dialect, which has
// only AUTO_INCREMENT (the by-default kind). SQLite keeps it: its rowid alias
// has one kind, and the schema normalizer maps always to by default there.
func (b *schemaBuilder) applyIdentityAlways(f *Field) {
	for _, d := range append(slices.Clone(b.dialects), b.primary) {
		if d == dialect.MySQL {
			b.diag(f.Pos, fmt.Sprintf("field %s: identity:always is not supported on %s; use identity", f.GoName, d))
			return
		}
	}
}

// serialTypes are the integer column types a serial column can have.
var serialTypes = map[string]bool{"smallint": true, "integer": true, "bigint": true}

// applySerial marks col as a serial column, a sequence-backed integer whose
// DEFAULT is nextval of a sequence owned by the column. It is PostgreSQL
// only; it excludes identity, an explicit default, and non-integer types.
func (b *schemaBuilder) applySerial(f *Field, col *schema.Column) {
	switch {
	case f.Tags.Identity:
		b.diag(f.Pos, fmt.Sprintf("field %s: serial conflicts with identity", f.GoName))
		return
	case f.Tags.Default != "":
		b.diag(f.Pos, fmt.Sprintf("field %s: serial conflicts with default", f.GoName))
		return
	case col.Nullable:
		b.diag(f.Pos, fmt.Sprintf("field %s: serial columns are always NOT NULL; use a non-nullable Go type without the nullable tag", f.GoName))
		return
	}
	for _, d := range b.dialects {
		if d != dialect.Postgres {
			b.diag(f.Pos, fmt.Sprintf("field %s: serial is not supported on %s; use identity", f.GoName, d))
			return
		}
	}
	if b.primary != dialect.Postgres {
		b.diag(f.Pos, fmt.Sprintf("field %s: serial is not supported on %s; use identity", f.GoName, b.primary))
		return
	}
	if !serialTypes[schema.NormalizeType(b.primary, col.Type)] {
		b.diag(f.Pos, fmt.Sprintf("field %s: serial needs an integer column, not %s", f.GoName, col.Type))
		return
	}
	col.Identity = "serial"
	col.Type = schema.NormalizeType(b.primary, col.Type)
}

// vectorKinds maps the pgvector Go type names to their SQL type.
var vectorKinds = map[string]string{
	"Vector": "vector", "HalfVector": "halfvec", "SparseVector": "sparsevec", "Bits": "bit",
}

// vectorKind returns the SQL kind of a pgvector Go type. A type from the
// vector package always counts; any other type with a pgvector name counts
// only when the field also carries dim, so a stand-in type declared next to
// the models behaves like the real one.
func vectorKind(core string, dim int) string {
	pkg, name, qualified := strings.Cut(core, ".")
	if !qualified {
		name = pkg
	}
	k := vectorKinds[name]
	if k == "" || (pkg != "vector" && dim == 0) {
		return ""
	}
	return k
}

// columnType resolves the SQL type of f: the type tag, then size, precision
// and dim shapes, then an enum, then the Go type's base mapping.
func (b *schemaBuilder) columnType(f *Field, core string) string {
	tg := f.Tags
	inner := unwrapNull(core)
	kind := vectorKind(inner, tg.Dim)
	isString := inner == "string"
	// A string that carries a decimal (numeric) takes precision and scale like
	// a float does.
	isFloat := inner == "float32" || inner == "float64" || inner == "types.Numeric" || strings.HasSuffix(inner, "decimal.Decimal") ||
		(isString && tg.Size == 0 && tg.Precision > 0)

	conflict := func(tag string) {
		b.diag(f.Pos, fmt.Sprintf("field %s: %s conflicts with type", f.GoName, tag))
	}
	if tg.Size > 0 && !isString && tg.Type == "" {
		b.diag(f.Pos, fmt.Sprintf("field %s: size is only valid on string fields", f.GoName))
	}
	if tg.Dim > 0 && kind == "" && tg.Type == "" {
		b.diag(f.Pos, fmt.Sprintf("field %s: dim is only valid on a vector field", f.GoName))
	}
	if (tg.Precision > 0 || tg.Scale > 0) && !isFloat && tg.Type == "" {
		b.diag(f.Pos, fmt.Sprintf("field %s: precision is only valid on float or decimal fields (a string without size that carries a numeric also takes it)", f.GoName))
	}
	if tg.Scale > 0 && tg.Precision == 0 && isFloat {
		b.diag(f.Pos, fmt.Sprintf("field %s: scale requires precision", f.GoName))
	}
	if kind != "" && (kind == "sparsevec" || kind == "bit") && tg.Dim == 0 && tg.Type == "" {
		b.diag(f.Pos, fmt.Sprintf("field %s: %s requires dim", f.GoName, kind))
	}

	switch {
	case tg.Type != "":
		for _, c := range []struct {
			on  bool
			tag string
		}{{tg.Size > 0, "size"}, {tg.Precision > 0, "precision"}, {tg.Dim > 0, "dim"}} {
			if c.on {
				conflict(c.tag)
			}
		}
		b.noteType(f, tg.Type)
		return tg.Type
	case tg.Enum != "":
		if b.primary == dialect.SQLite {
			return "TEXT"
		}
		if b.primary == dialect.MySQL {
			return "text"
		}
		return tg.Enum
	case kind != "" && b.primary != dialect.MySQL:
		typ := kind
		if tg.Dim > 0 {
			typ = fmt.Sprintf("%s(%d)", kind, tg.Dim)
		}
		b.noteType(f, typ)
		return typ
	case isString && tg.Size > 0:
		return fmt.Sprintf("varchar(%d)", tg.Size)
	case inner == "types.Numeric" && tg.Precision > 0 && b.primary == dialect.SQLite:
		// numeric(p,s) gives a SQLite column NUMERIC affinity, which would
		// round the decimal text; keep it TEXT. MySQL has an exact DECIMAL.
		return "TEXT"
	case isFloat && tg.Precision > 0:
		name := "numeric"
		if b.primary == dialect.MySQL {
			name = "decimal"
			b.checkDecimal(f, tg)
		}
		if tg.Scale > 0 {
			return fmt.Sprintf("%s(%d,%d)", name, tg.Precision, tg.Scale)
		}
		return fmt.Sprintf("%s(%d)", name, tg.Precision)
	}
	// BaseType is the Go type with named types resolved; hand-built fields
	// without it map by their spelled type.
	goType := f.BaseType
	if goType == "" {
		goType = inner
	}
	// The type default applies to the primary dialect (the first of
	// Config.Dialects) only, and only when it is PostgreSQL.
	// Only the time.Time entry is a column type default; BuildSchema reports
	// every other key.
	def := ""
	if goType == "time.Time" {
		def = b.typeDef["time.Time"]
	}
	if def != "" && b.primary == dialect.Postgres {
		if ValidTimeType(def) {
			return strings.TrimSpace(def)
		}
		b.diag(f.Pos, fmt.Sprintf("field %s: type default %q for %s is not a timestamp type; use timestamp, timestamptz, time or timetz with an optional precision 0-6", f.GoName, def, goType))
	}
	typ, err := typemap.Map(goType, b.primary, tg)
	if err != nil {
		b.diag(f.Pos, fmt.Sprintf("field %s: %v; add a type tag", f.GoName, err))
	}
	return typ
}

// checkDecimal reports a precision or scale that MySQL and MariaDB cannot
// store: DECIMAL(p,s) takes p up to 65, s up to 30 and s not above p. A
// precision without a scale is a scale of 0.
func (b *schemaBuilder) checkDecimal(f *Field, tg tags.Tags) {
	switch {
	case tg.Precision > 65:
		b.diag(f.Pos, fmt.Sprintf("field %s: precision %d is above 65, the largest DECIMAL on MySQL and MariaDB", f.GoName, tg.Precision))
	case tg.Scale > 30:
		b.diag(f.Pos, fmt.Sprintf("field %s: scale %d is above 30, the largest DECIMAL scale on MySQL and MariaDB", f.GoName, tg.Scale))
	case tg.Scale > tg.Precision:
		b.diag(f.Pos, fmt.Sprintf("field %s: scale %d is above precision %d", f.GoName, tg.Scale, tg.Precision))
	}
}

// mysqlUnkeyable are the column types MySQL and MariaDB cannot put in a key
// or an index without a prefix length.
var mysqlUnkeyable = map[string]bool{
	"text": true, "tinytext": true, "mediumtext": true, "longtext": true,
	"blob": true, "tinyblob": true, "mediumblob": true, "longblob": true,
	"json": true,
}

// checkMySQLKeys refuses a primary key, unique key, index or foreign key over
// a text, blob or json column. MySQL rejects it with error 1170 in the middle
// of the migration, and DDL there does not roll back, which would leave a dirty
// migration; so the schema is refused before anything is written. An index
// part with a prefix length (name(20)) is allowed.
func (b *schemaBuilder) checkMySQLKeys() {
	for i := range b.s.Tables {
		t := &b.s.Tables[i]
		m := b.tableBy[t.Schema+"\x00"+t.Name]
		if m == nil {
			continue
		}
		bad := map[string]string{}
		for _, c := range t.Columns {
			base, _, _ := strings.Cut(schema.NormalizeType(dialect.MySQL, c.Type), "(")
			if mysqlUnkeyable[strings.TrimSpace(base)] {
				bad[c.Name] = base
			}
		}
		if len(bad) == 0 {
			continue
		}
		report := func(what string, cols []string) {
			for _, col := range cols {
				if typ, ok := bad[col]; ok {
					b.diag(m.Pos, fmt.Sprintf("table %s: column %s is %s and is part of %s, which MySQL and MariaDB cannot key without a length; give a string field size:N (a varchar), or give another column type with a type tag", t.Name, col, typ, what))
				}
			}
		}
		report("the primary key", t.PrimaryKey)
		for _, u := range t.Uniques {
			report("unique key "+u.Name, u.Columns)
		}
		for _, ix := range t.Indexes {
			cols := make([]string, 0, len(ix.Parts))
			for _, p := range ix.Parts {
				cols = append(cols, p.Column)
			}
			report("index "+ix.Name, cols)
		}
		for _, fk := range t.ForeignKeys {
			report("foreign key "+fk.Name, fk.Columns)
		}
	}
}

// noteType records the dialect-restricted types of a column.
func (b *schemaBuilder) noteType(f *Field, typ string) {
	base, _, _ := strings.Cut(strings.ToLower(typ), "(")
	switch base = strings.TrimSpace(base); base {
	case "vector", "halfvec", "sparsevec":
		b.vector = true
		b.feats = append(b.feats, feature{base, f.Pos})
	case "bit":
		b.feats = append(b.feats, feature{base, f.Pos})
	}
}

// unwrapNull maps the sql.Null family to the Go type it wraps.
func unwrapNull(core string) string {
	if inner, ok := strings.CutPrefix(core, "sql.Null["); ok {
		return strings.TrimSuffix(inner, "]")
	}
	switch core {
	case "sql.NullString":
		return "string"
	case "sql.NullInt64":
		return "int64"
	case "sql.NullInt32":
		return "int32"
	case "sql.NullInt16":
		return "int16"
	case "sql.NullByte":
		return "uint8"
	case "sql.NullBool":
		return "bool"
	case "sql.NullFloat64":
		return "float64"
	case "sql.NullTime":
		return "time.Time"
	}
	return core
}

// addIndex appends ix to t unless its name is already taken in the table's
// index and unique namespace.
func (b *schemaBuilder) addIndex(t *schema.Table, ix schema.Index, pos token.Position, who string) {
	if !b.claimName(ix.Name, pos, who) {
		return
	}
	t.Indexes = append(t.Indexes, ix)
	switch ix.Method {
	case "hnsw", "ivfflat":
		b.vector = true
		b.feats = append(b.feats, feature{ix.Method, pos})
		if b.primary == dialect.Postgres {
			if err := schema.ValidateVectorIndex(b.primary, t, ix); err != nil {
				b.diag(pos, err.Error())
			}
		}
	}
}

// defaultIndexName is the name of an index or unique index that a tag does
// not name: <table>_<column>_idx, and <table>_<column>_key for a unique one.
// Direct fields (schema.go) and mixin fields (embed.go) both use it.
func defaultIndexName(table, column string, unique bool) string {
	if unique {
		return table + "_" + column + "_key"
	}
	return table + "_" + column + "_idx"
}

func (b *schemaBuilder) claimName(name string, pos token.Position, who string) bool {
	if prev, dup := b.names[name]; dup {
		b.diag(pos, fmt.Sprintf("index name %q is declared by both %s and %s", name, prev, who))
		return false
	}
	b.names[name] = who
	return true
}

// applyIndexTags adds the indexes of the index, unique, sort, nulls and
// method tags. A field below the model's first level (an embedded struct or
// mixin) already contributed an index through Model.Indexes; that entry is
// kept and receives the field's sort, nulls and method.
func (b *schemaBuilder) applyIndexTags(m *Model, t *schema.Table) {
	byColumn := map[string]*Field{}
	for _, f := range m.Fields {
		byColumn[f.Column] = f
		tg := f.Tags
		if !tg.Index && !tg.Unique {
			if tg.Sort != "" {
				b.diag(f.Pos, fmt.Sprintf("field %s: sort requires index or unique", f.GoName))
			}
			if tg.Nulls != "" {
				b.diag(f.Pos, fmt.Sprintf("field %s: nulls requires index or unique", f.GoName))
			}
			if tg.Method != "" {
				b.diag(f.Pos, fmt.Sprintf("field %s: method requires index or unique", f.GoName))
			}
			continue
		}
		if len(f.Path) > 1 {
			continue
		}
		who := "the tag on field " + f.GoName
		part := schema.IndexPart{Column: f.Column, Desc: tg.Sort == "desc", Nulls: tg.Nulls}
		if tg.Index {
			name := tg.IndexName
			if name == "" {
				name = defaultIndexName(m.Table, f.Column, false)
			}
			b.addIndex(t, schema.Index{Name: name, Parts: []schema.IndexPart{part}, Method: tg.Method}, f.Pos, who)
		}
		if tg.Unique {
			name := tg.UniqueName
			if name == "" {
				name = defaultIndexName(m.Table, f.Column, true)
			}
			b.addIndex(t, schema.Index{Name: name, Unique: true, Parts: []schema.IndexPart{part}, Method: tg.Method}, f.Pos, who)
		}
	}
	embedded := map[string]bool{}
	for _, ix := range m.Indexes {
		if embedded[ix.Name] {
			continue // the repeat was already reported while flattening
		}
		embedded[ix.Name] = true
		ix.Parts = slices.Clone(ix.Parts)
		pos, who := m.Pos, "an embedded struct"
		if len(ix.Parts) == 1 {
			if f := byColumn[ix.Parts[0].Column]; f != nil && len(f.Path) > 1 {
				ix.Parts[0].Desc = f.Tags.Sort == "desc"
				ix.Parts[0].Nulls = f.Tags.Nulls
				ix.Method = f.Tags.Method
				pos, who = f.Pos, "the tag on field "+f.GoName
			}
		}
		b.addIndex(t, ix, pos, who)
	}
}

// applyCompanion merges the model's MomoConfig into the table. A value that
// the tag and the companion both set and that differs is a conflict.
func (b *schemaBuilder) applyCompanion(m *Model, t *schema.Table) {
	c := m.Config
	if c == nil {
		return
	}
	missing := func(kind, name string, cols []string) bool {
		bad := false
		for _, col := range cols {
			if _, ok := t.Column(col); col != "" && !ok {
				b.diag(m.Pos, fmt.Sprintf("MomoConfig: %s %q: column %q is not a column of table %s", kind, name, col, t.Name))
				bad = true
			}
		}
		return bad
	}
	for _, ix := range c.Indexes {
		ix.Parts = slices.Clone(ix.Parts)
		cols := make([]string, len(ix.Parts))
		for i, p := range ix.Parts {
			// An IndexExpr argument that is exactly a column name is a
			// plain column key, which is how the database reports it.
			if name := strings.TrimSpace(p.Expr); p.Column == "" && name != "" {
				if _, ok := t.Column(name); ok {
					ix.Parts[i] = schema.IndexPart{Column: name}
					p = ix.Parts[i]
				}
			}
			cols[i] = p.Column
		}
		if missing("index", ix.Name, cols) {
			continue
		}
		b.addIndex(t, ix, m.Pos, "MomoConfig")
	}
	for _, u := range c.Uniques {
		if missing("unique", u.Name, u.Columns) {
			continue
		}
		if b.claimName(u.Name, m.Pos, "MomoConfig") {
			t.Uniques = append(t.Uniques, schema.Unique{Name: u.Name, Columns: slices.Clone(u.Columns)})
		}
	}
	if len(c.PrimaryKey) > 0 {
		have := slices.Clone(t.PrimaryKey)
		want := slices.Clone(c.PrimaryKey)
		slices.Sort(have)
		slices.Sort(want)
		if !slices.Equal(have, want) {
			b.diag(m.Pos, fmt.Sprintf("MomoConfig: PrimaryKey lists (%s) but the fields tagged pk are (%s); it must name exactly those, once each", strings.Join(c.PrimaryKey, ", "), strings.Join(t.PrimaryKey, ", ")))
		} else {
			t.PrimaryKey = slices.Clone(c.PrimaryKey)
		}
	}
	checks := map[string]string{}
	for _, f := range m.Fields {
		if f.Tags.Check != "" {
			checks[m.Table+"_"+f.Column+"_check"] = "the tag on field " + f.GoName
		}
	}
	for _, ck := range c.Checks {
		if prev, dup := checks[ck.Name]; dup {
			b.diag(m.Pos, fmt.Sprintf("check name %q is declared by both %s and MomoConfig", ck.Name, prev))
			continue
		}
		checks[ck.Name] = "MomoConfig"
		t.Checks = append(t.Checks, ck)
	}
	b.columnText(m, t, "default", c.Defaults, func(c *schema.Column) *string { return &c.Default })
	b.columnText(m, t, "generated", c.Generated, func(c *schema.Column) *string { return &c.Generated })
	b.checkVirtual(m, t, c.GeneratedVirtual)
	b.checkGenerated(m, t)
	b.columnText(m, t, "comment", c.Comments, func(c *schema.Column) *string { return &c.Comment })
	b.columnText(m, t, "collate", c.Collates, func(c *schema.Column) *string { return &c.Collate })
	for _, e := range c.Enums {
		if prev, ok := b.enums[e.Name]; ok {
			if !slices.Equal(prev.enum.Values, e.Values) {
				b.diag(m.Pos, fmt.Sprintf("enum %q is declared with different values (also at %s:%d)",
					e.Name, prev.pos.Filename, prev.pos.Line))
			}
			continue
		}
		b.enums[e.Name] = enumDecl{enum: schema.Enum{Name: e.Name, Values: slices.Clone(e.Values)}, pos: m.Pos}
	}
	if c.RLS != nil {
		rls := *c.RLS
		t.RLS = &rls
	}
	if c.Partition != nil {
		p := *c.Partition
		t.Partition = &p
	}
	if c.PartitionOf != nil {
		p := *c.PartitionOf
		t.PartitionOf = &p
	}
	mergeObjects(b, &b.s.Views, "view", c.Views, func(v schema.View) string { return v.Schema + "." + v.Name }, m.Pos)
	mergeObjects(b, &b.s.Functions, "function", c.Functions, func(f schema.Function) string { return f.Schema + "." + f.Name + "(" + f.Args + ")" }, m.Pos)
	mergeObjects(b, &b.s.Triggers, "trigger", c.Triggers, func(g schema.Trigger) string { return g.Schema + "." + g.Table + "." + g.Name }, m.Pos)
	mergeObjects(b, &b.s.Policies, "policy", c.Policies, func(p schema.Policy) string { return p.Schema + "." + p.Table + "." + p.Name }, m.Pos)
}

// checkVirtual finishes the virtual generated columns of t: the ones the
// MomoConfig hook marks join the ones the tag marked, a virtual column with no
// generated expression is an error, and so is one for a PostgreSQL target,
// because PostgreSQL has VIRTUAL columns only from version 18 and momo
// supports 16 and 17.
func (b *schemaBuilder) checkVirtual(m *Model, t *schema.Table, hook map[string]bool) {
	for i := range t.Columns {
		col := &t.Columns[i]
		if hook[col.Name] {
			col.GeneratedVirtual = true
		}
		if !col.GeneratedVirtual {
			continue
		}
		pos := m.Pos
		for _, f := range m.Fields {
			if f.Column == col.Name {
				pos = f.Pos
			}
		}
		if col.Generated == "" {
			b.diag(pos, fmt.Sprintf("column %q: virtual is valid only together with generated", col.Name))
			col.GeneratedVirtual = false
			continue
		}
		if slices.Contains(b.dialects, dialect.Postgres) {
			b.diag(pos, fmt.Sprintf("column %q: virtual is not supported on %s (VIRTUAL generated columns need PostgreSQL 18; momo supports 16 and 17); drop virtual to make it STORED", col.Name, dialect.Postgres))
		}
	}
}

// checkGenerated reports a stored generated column that is not read_only, so a
// write would fail in the database, and one that also has a default, identity
// or serial, which the database refuses.
func (b *schemaBuilder) checkGenerated(m *Model, t *schema.Table) {
	for _, col := range t.Columns {
		if col.Generated == "" {
			continue
		}
		for _, f := range m.Fields {
			if f.Column != col.Name {
				continue
			}
			conflict := ""
			switch {
			case f.Access != ReadOnly:
				b.diag(f.Pos, fmt.Sprintf("field %s: a generated column must be read_only, because the database refuses a write to it", f.GoName))
				continue
			case col.Default != "":
				conflict = "default"
			case f.Tags.Identity || col.Identity != "":
				conflict = "identity"
			case f.Tags.Serial:
				conflict = "serial"
			case f.Tags.DBDefault:
				conflict = "dbdefault"
			case f.Tags.Version:
				conflict = "version"
			case f.Tags.SoftDelete != "":
				conflict = "soft_delete"
			case f.Tags.CreatedAt:
				conflict = "created_at"
			case f.Tags.UpdatedAt:
				conflict = "updated_at"
			case f.Tags.IDGen != "" || (m.Config != nil && m.Config.IDGens[f.Column] != ""):
				conflict = "idgen"
			}
			if conflict != "" {
				b.diag(f.Pos, fmt.Sprintf("field %s: generated conflicts with %s", f.GoName, conflict))
			}
		}
	}
}

// columnText merges one of the companion's per-column texts into the columns
// of t, by sorted column name.
func (b *schemaBuilder) columnText(m *Model, t *schema.Table, what string, texts map[string]string, field func(*schema.Column) *string) {
	cols := make([]string, 0, len(texts))
	for col := range texts {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	for _, name := range cols {
		col, ok := t.Column(name)
		if !ok {
			b.diag(m.Pos, fmt.Sprintf("MomoConfig: column %q is not a column of table %s", name, t.Name))
			continue
		}
		dst := field(col)
		switch {
		case *dst == "":
			*dst = texts[name]
		case *dst != texts[name]:
			pos := m.Pos
			for _, f := range m.Fields {
				if f.Column == name {
					pos = f.Pos
				}
			}
			b.diag(pos, fmt.Sprintf("column %q: %s is set by both the tag and MomoConfig", name, what))
		}
	}
}

// mergeObjects appends the managed objects of one model, keeping a repeat of
// an identical object once and reporting a different object of the same key.
func mergeObjects[T any](b *schemaBuilder, dst *[]T, kind string, in []T, key func(T) string, pos token.Position) {
	for _, o := range in {
		k := kind + "\x00" + key(o)
		if prev, ok := b.seen[k]; ok {
			if !reflect.DeepEqual(prev, o) {
				b.diag(pos, fmt.Sprintf("%s %q is declared differently by another model", kind, key(o)))
			}
			continue
		}
		b.seen[k] = o
		*dst = append(*dst, o)
	}
}

// dialectCheck turns the dialect-restricted constructs found in the table
// just built into diagnostics, once per listed dialect.
func (b *schemaBuilder) dialectCheck() {
	for _, d := range b.dialects {
		if d != dialect.SQLite {
			continue
		}
		for _, f := range b.feats {
			b.diag(f.pos, fmt.Sprintf("%s is not supported on %s", f.name, d))
		}
	}
	b.feats = nil
}

// maxConstraintName is the longest constraint name PostgreSQL keeps (NAMEDATALEN-1).
const maxConstraintName = 63

var referentialActions = map[string]bool{
	"CASCADE": true, "RESTRICT": true, "SET NULL": true, "SET DEFAULT": true, "NO ACTION": true,
}

// action normalizes an on_delete or on_update tag value ("set_null" or
// "set null") to its SQL spelling.
func action(v string) (string, bool) {
	if v == "" {
		return "", true
	}
	a := strings.ToUpper(strings.NewReplacer("_", " ", "-", " ").Replace(v))
	return a, referentialActions[a]
}

// addRelations adds the foreign keys of belongs-to relations and the join
// tables of many-to-many relations of m.
func (b *schemaBuilder) addRelations(m *Model) {
	for _, r := range m.Relations {
		if r.Target == nil || b.tableBy[tableKey(r.Target)] != r.Target {
			// An unresolved target is reported where relations are
			// discovered. A view model is not in tableBy either, so a relation
			// that targets one adds no foreign key and no join table: a
			// constraint cannot reference a view.
			continue
		}
		if r.FKName != "" && r.Kind != BelongsTo {
			b.diag(r.Pos, fmt.Sprintf("relation %s: fk_name applies to belongs_to only; has_one and has_many create no constraint", r.GoName))
		}
		switch r.Kind {
		case BelongsTo:
			b.belongsTo(m, r)
		case ManyToMany:
			b.manyToMany(m, r)
		}
	}
	b.checkFKNameConfig(m)
	b.companionForeignKeys(m)
}

// checkFKNameConfig reports FKName calls on a column that is not the key of a
// belongs_to relation that creates a constraint.
func (b *schemaBuilder) checkFKNameConfig(m *Model) {
	if m.Config == nil {
		return
	}
	for col := range m.Config.FKNames {
		found := false
		for _, r := range m.Relations {
			if r.Kind != BelongsTo || r.NoConstraint {
				continue
			}
			for _, f := range m.Fields {
				found = found || (f.GoName == r.FK && f.Column == col)
			}
		}
		if !found {
			b.diag(m.Pos, fmt.Sprintf("MomoConfig: FKName on column %q, which is not the key of a belongs_to relation that creates a constraint", col))
		}
	}
}

// companionForeignKeys adds the foreign keys declared with Config.ForeignKey.
// It runs after every model has its table, so the referenced table and its
// unique key can be checked: the referenced columns must be the table's
// primary key or the columns of a unique constraint or a non-partial unique
// index, in any order.
func (b *schemaBuilder) companionForeignKeys(m *Model) {
	if m.Config == nil || len(m.Config.ForeignKeys) == 0 {
		return
	}
	t := &b.s.Tables[b.index[m]]
	for _, fk := range m.Config.ForeignKeys {
		bad := func(format string, args ...any) {
			b.diag(m.Pos, fmt.Sprintf("MomoConfig: foreign key %q: ", fk.Name)+fmt.Sprintf(format, args...))
		}
		if len(fk.Name) > maxConstraintName {
			bad("the name is longer than %d bytes", maxConstraintName)
			continue
		}
		if slices.ContainsFunc(t.ForeignKeys, func(o schema.ForeignKey) bool { return o.Name == fk.Name }) {
			bad("the name is already used by another foreign key of table %s", t.Name)
			continue
		}
		ns, name := m.Schema, fk.RefTable
		if i := strings.IndexByte(name, '.'); i >= 0 {
			ns, name = name[:i], name[i+1:]
		}
		ref := b.tableBy[ns+"\x00"+name]
		if ref == nil {
			bad("the referenced table %q is not the table of any model", fk.RefTable)
			continue
		}
		target := &b.s.Tables[b.index[ref]]
		missing := false
		for _, c := range fk.RefColumns {
			if _, ok := target.Column(c); !ok {
				bad("column %q is not a column of table %s", c, target.Name)
				missing = true
			}
		}
		if missing {
			continue
		}
		if !coversKey(target, fk.RefColumns) {
			bad("(%s) is neither the primary key of %s nor the columns of a unique constraint or unique index on it", strings.Join(fk.RefColumns, ", "), target.Name)
			continue
		}
		t.ForeignKeys = append(t.ForeignKeys, schema.ForeignKey{
			Name:       fk.Name,
			Columns:    slices.Clone(fk.Columns),
			RefTable:   fk.RefTable,
			RefColumns: slices.Clone(fk.RefColumns),
			OnDelete:   fk.OnDelete,
			OnUpdate:   fk.OnUpdate,
		})
	}
}

// coversKey reports whether cols are exactly the columns of t's primary key,
// of one of its unique constraints or of one of its non-partial unique
// indexes over plain columns, compared as sets.
func coversKey(t *schema.Table, cols []string) bool {
	same := func(key []string) bool {
		if len(key) != len(cols) {
			return false
		}
		a, c := slices.Clone(key), slices.Clone(cols)
		slices.Sort(a)
		slices.Sort(c)
		return slices.Equal(a, c)
	}
	if same(t.PrimaryKey) {
		return true
	}
	for _, u := range t.Uniques {
		if same(u.Columns) {
			return true
		}
	}
	for _, ix := range t.Indexes {
		if !ix.Unique || ix.Where != "" {
			continue
		}
		key := make([]string, 0, len(ix.Parts))
		for _, p := range ix.Parts {
			if p.Column == "" {
				key = nil
				break
			}
			key = append(key, p.Column)
		}
		if key != nil && same(key) {
			return true
		}
	}
	return false
}

func (b *schemaBuilder) belongsTo(m *Model, r *Relation) {
	if r.NoConstraint {
		return
	}
	var local *Field
	for _, f := range m.Fields {
		if f.GoName == r.FK {
			local = f
		}
	}
	if local == nil {
		b.diag(r.Pos, fmt.Sprintf("relation %s: foreign key field %s is not a field of %s", r.GoName, r.FK, m.GoName))
		return
	}
	del, ok1 := action(r.OnDelete)
	if !ok1 {
		b.diag(r.Pos, fmt.Sprintf("relation %s: on_delete %q is not valid", r.GoName, r.OnDelete))
	}
	upd, ok2 := action(r.OnUpdate)
	if !ok2 {
		b.diag(r.Pos, fmt.Sprintf("relation %s: on_update %q is not valid", r.GoName, r.OnUpdate))
	}
	target := &b.s.Tables[b.index[r.Target]]
	if len(target.PrimaryKey) != 1 {
		b.diag(r.Pos, fmt.Sprintf("relation %s: target %s needs a single-column primary key for a foreign key, but its key is (%s); declare a composite foreign key with Config.ForeignKey in MomoConfig", r.GoName, r.Target.GoName, strings.Join(target.PrimaryKey, ", ")))
		return
	}
	if !ok1 || !ok2 {
		return
	}
	name := m.Table + "_" + local.Column + "_fkey"
	var cfg string
	if m.Config != nil {
		cfg = m.Config.FKNames[local.Column]
	}
	switch {
	case r.FKName != "" && cfg != "":
		b.diag(r.Pos, fmt.Sprintf("relation %s: the foreign key name is declared by both the fk_name tag and MomoConfig", r.GoName))
		return
	case r.FKName != "":
		name = r.FKName
	case cfg != "":
		name = cfg
	}
	if len(name) > maxConstraintName {
		b.diag(r.Pos, fmt.Sprintf("relation %s: foreign key name %q is longer than %d bytes", r.GoName, name, maxConstraintName))
		return
	}
	t := &b.s.Tables[b.index[m]]
	for _, fk := range t.ForeignKeys {
		if fk.Name == name {
			b.diag(r.Pos, fmt.Sprintf("relation %s: foreign key name %q is already used by another foreign key of table %s", r.GoName, name, t.Name))
			return
		}
	}
	t.ForeignKeys = append(t.ForeignKeys, schema.ForeignKey{
		Name:       name,
		Columns:    []string{local.Column},
		RefTable:   target.Name,
		RefColumns: []string{target.PrimaryKey[0]},
		OnDelete:   del,
		OnUpdate:   upd,
	})
}

func (b *schemaBuilder) manyToMany(m *Model, r *Relation) {
	if r.Target == m {
		b.diag(r.Pos, fmt.Sprintf("relation %s: many2many to the model's own type is not supported", r.GoName))
		return
	}
	owner, target := b.s.Tables[b.index[m]], b.s.Tables[b.index[r.Target]]
	if len(owner.PrimaryKey) == 0 || len(target.PrimaryKey) == 0 {
		b.diag(r.Pos, fmt.Sprintf("relation %s: both models of a many2many need a primary key", r.GoName))
		return
	}
	for _, rev := range r.Target.Relations {
		if rev.Kind == ManyToMany && rev.Target == m && (rev.JoinTable == "") != (r.JoinTable == "") {
			if r.JoinTable == "" {
				// The side that names the table builds it; this one only reports.
				b.diag(r.Pos, fmt.Sprintf("relation %s: join: is set on the other side of the many2many (%s.%s) but not here; set it on both sides", r.GoName, r.Target.GoName, rev.GoName))
				return
			}
			break
		}
	}
	name := r.JoinTable
	if name == "" {
		name = b.namer.JoinTableName(m.GoName, r.Target.GoName)
	}
	jt := schema.Table{Schema: m.Schema, Name: name, Columns: []schema.Column{}}
	side := func(model *Model, t schema.Table) {
		prefix := b.namer.ColumnName(model.GoName)
		var cols []string
		for _, pk := range t.PrimaryKey {
			src, _ := t.Column(pk)
			c := joinColumnName(b.namer, model, pk)
			jt.Columns = append(jt.Columns, schema.Column{Name: c, Type: src.Type})
			jt.PrimaryKey = append(jt.PrimaryKey, c)
			cols = append(cols, c)
		}
		jt.ForeignKeys = append(jt.ForeignKeys, schema.ForeignKey{
			Name:       name + "_" + prefix + "_fkey",
			Columns:    cols,
			RefTable:   t.Name,
			RefColumns: slices.Clone(t.PrimaryKey),
			OnDelete:   "CASCADE",
		})
	}
	side(m, owner)
	side(r.Target, target)
	key := m.Schema + "\x00" + name
	if other, ok := b.tableBy[key]; ok {
		b.diag(r.Pos, fmt.Sprintf("relation %s: join table %q is already the table of model %s", r.GoName, name, other.GoName))
		return
	}
	if prev, ok := b.seen["join\x00"+key]; ok {
		if !sameColumns(prev.(schema.Table), jt) {
			b.diag(r.Pos, fmt.Sprintf("relation %s: join table %q is declared with a different layout elsewhere", r.GoName, name))
		}
		return
	}
	b.seen["join\x00"+key] = jt
	b.s.Tables = append(b.s.Tables, jt)
}

// sameColumns reports whether two join tables hold the same columns, in any
// order: both sides of one many-to-many declare it from opposite ends.
func sameColumns(a, c schema.Table) bool {
	flat := func(t schema.Table) []string {
		out := make([]string, 0, len(t.Columns))
		for _, col := range t.Columns {
			out = append(out, col.Name+" "+col.Type)
		}
		sort.Strings(out)
		return out
	}
	return slices.Equal(flat(a), flat(c))
}

// finish adds what spans models: the enum types, the extensions the columns
// and indexes need, and the check that every enum a column names exists.
func (b *schemaBuilder) finish() {
	if b.primary == dialect.MySQL {
		b.checkMySQLKeys()
	}
	for _, u := range b.enumUses {
		if _, ok := b.enums[u.name]; !ok {
			b.diag(u.pos, fmt.Sprintf("enum %q is not declared in any MomoConfig", u.name))
		}
	}
	for _, e := range b.enums {
		b.s.Enums = append(b.s.Enums, e.enum)
	}
	if b.vector && b.primary == dialect.Postgres {
		b.s.Extensions = append(b.s.Extensions, schema.Extension{Name: "vector"})
	}
}
