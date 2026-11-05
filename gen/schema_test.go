// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"encoding/json"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/schema"
)

func TestTagVocabulary(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./companion"}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diags: %v", sdiags)
	}
	tbl, ok := s.Table("docs")
	if !ok {
		t.Fatal("docs table missing")
	}
	col, found := tbl.Column("embedding")
	if !found {
		t.Fatal("embedding column missing")
	}
	if col.Type != "vector(3)" {
		t.Fatalf("embedding type=%q", col.Type)
	}
	if _, ok := tbl.Index("docs_title_idx"); !ok {
		t.Fatal("docs_title_idx missing")
	}
	_, sdiags = BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.SQLite}})
	if !strings.Contains(sdiagsString(sdiags), "method:hnsw") && !strings.Contains(sdiagsString(sdiags), "hnsw is not supported on sqlite") {
		t.Fatalf("expected a sqlite diagnostic, got %v", sdiags)
	}
}

func sdiagsString(ds []Diagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.String() + "\n")
	}
	return b.String()
}

// fld builds a persisted field the way discovery does.
func fld(name, goType, tag string) *Field {
	tg, _ := tags.Parse(tag)
	col := tg.Column
	if col == "" {
		col = naming.ToSnake(name)
	}
	return &Field{
		GoName:   name,
		Path:     []int{0},
		Column:   col,
		GoType:   goType,
		Nullable: strings.HasPrefix(goType, "*"),
		Tags:     tg,
		Pos:      token.Position{Filename: "m.go", Line: 10 + len(name), Column: 2},
	}
}

func mdl(goName, table string, fields ...*Field) *Model {
	for i, f := range fields {
		f.Path = []int{i + 1}
	}
	return &Model{
		GoName: goName, PkgPath: "ex/m", Table: table, Fields: fields,
		Pos: token.Position{Filename: "m.go", Line: 3, Column: 6},
		Config: &CompanionResult{
			Defaults: map[string]string{}, Comments: map[string]string{}, Collates: map[string]string{},
		},
	}
}

func pg() Config { return Config{Dialects: []dialect.Name{dialect.Postgres}} }

func build(t *testing.T, cfg Config, ms ...*Model) (*schema.Schema, []Diagnostic) {
	t.Helper()
	return BuildSchema(ms, cfg)
}

func mustCol(t *testing.T, s *schema.Schema, table, col string) *schema.Column {
	t.Helper()
	tb, ok := s.Table(table)
	if !ok {
		t.Fatalf("table %s missing", table)
	}
	c, ok := tb.Column(col)
	if !ok {
		t.Fatalf("column %s.%s missing", table, col)
	}
	return c
}

func TestTagEffectsOnColumns(t *testing.T) {
	m := mdl("Thing", "things",
		fld("ID", "int64", "pk,identity"),
		fld("Name", "string", "notnull,size:40"),
		fld("Nick", "*string", ""),
		fld("Bio", "string", "nullable"),
		fld("NullName", "sql.NullString", ""),
		fld("Age", "sql.Null[int32]", ""),
		fld("Score", "float64", "precision:10,scale:2"),
		fld("Raw", "string", "type:citext,default:''"),
		fld("Status", "string", "enum:thing_status,collate:C,comment:the status"),
		fld("Email", "string", "check:length(email) > 3"),
		fld("Emb", "vector.Vector", "dim:1536"),
		fld("Half", "vector.HalfVector", "dim:8"),
		fld("Sparse", "vector.SparseVector", "dim:9"),
		fld("Flags", "vector.Bits", "dim:16"),
		fld("Born", "time.Time", "created_at"),
	)
	m.Config.Enums = []schema.Enum{{Name: "thing_status", Values: []string{"a", "b"}}}
	s, diags := build(t, pg(), m)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	tb, _ := s.Table("things")
	if len(tb.PrimaryKey) != 1 || tb.PrimaryKey[0] != "id" {
		t.Fatalf("pk=%v", tb.PrimaryKey)
	}
	cases := []struct {
		col      string
		typ      string
		nullable bool
	}{
		{"id", "bigint", false}, {"name", "varchar(40)", false}, {"nick", "text", true},
		{"bio", "text", true}, {"null_name", "text", true}, {"age", "integer", true},
		{"score", "numeric(10,2)", false}, {"raw", "citext", false},
		{"status", "thing_status", false}, {"email", "text", false},
		{"emb", "vector(1536)", false}, {"half", "halfvec(8)", false},
		{"sparse", "sparsevec(9)", false}, {"flags", "bit(16)", false},
		{"born", "timestamptz", false},
	}
	for _, c := range cases {
		got := mustCol(t, s, "things", c.col)
		if got.Type != c.typ || got.Nullable != c.nullable {
			t.Errorf("%s: type=%q nullable=%v, want %q %v", c.col, got.Type, got.Nullable, c.typ, c.nullable)
		}
	}
	if c := mustCol(t, s, "things", "id"); c.Identity != "by_default" {
		t.Errorf("identity=%q", c.Identity)
	}
	if c := mustCol(t, s, "things", "raw"); c.Default != "''" {
		t.Errorf("default=%q", c.Default)
	}
	st := mustCol(t, s, "things", "status")
	if st.Enum != "thing_status" || st.Collate != "C" || st.Comment != "the status" {
		t.Errorf("status=%+v", st)
	}
	if len(tb.Checks) != 1 || tb.Checks[0].Name != "things_email_check" || tb.Checks[0].Expr != "length(email) > 3" {
		t.Errorf("checks=%+v", tb.Checks)
	}
	if len(s.Extensions) != 1 || s.Extensions[0].Name != "vector" {
		t.Errorf("extensions=%+v", s.Extensions)
	}
	if len(s.Enums) != 1 || s.Enums[0].Name != "thing_status" {
		t.Errorf("enums=%+v", s.Enums)
	}
}

func TestTagEffectsOnIndexes(t *testing.T) {
	m := mdl("Thing", "things",
		fld("ID", "int64", "pk"),
		fld("A", "string", "index"),
		fld("B", "string", "index:by_b,sort:desc,nulls:last"),
		fld("C", "string", "unique"),
		fld("D", "string", "unique:uniq_d"),
		fld("E", "vector.Vector", "dim:3,index:emb,method:hnsw"),
		fld("F", "string", "index,unique"),
	)
	s, diags := build(t, pg(), m)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	tb, _ := s.Table("things")
	want := map[string]struct {
		unique bool
		part   schema.IndexPart
		method string
	}{
		"things_a_idx": {part: schema.IndexPart{Column: "a"}},
		"by_b":         {part: schema.IndexPart{Column: "b", Desc: true, Nulls: "last"}},
		"things_c_key": {unique: true, part: schema.IndexPart{Column: "c"}},
		"uniq_d":       {unique: true, part: schema.IndexPart{Column: "d"}},
		"emb":          {part: schema.IndexPart{Column: "e"}, method: "hnsw"},
		"things_f_idx": {part: schema.IndexPart{Column: "f"}},
		"things_f_key": {unique: true, part: schema.IndexPart{Column: "f"}},
	}
	if len(tb.Indexes) != len(want) {
		t.Fatalf("indexes=%+v", tb.Indexes)
	}
	for name, w := range want {
		ix, ok := tb.Index(name)
		if !ok {
			t.Errorf("index %s missing", name)
			continue
		}
		if ix.Unique != w.unique || ix.Method != w.method || len(ix.Parts) != 1 || ix.Parts[0] != w.part {
			t.Errorf("%s = %+v", name, ix)
		}
	}
	for i := 1; i < len(tb.Indexes); i++ {
		if tb.Indexes[i-1].Name > tb.Indexes[i].Name {
			t.Fatalf("indexes not sorted: %v", tb.Indexes)
		}
	}
}

func TestMixinIndexesAreEnrichedAndKept(t *testing.T) {
	f := fld("Slug", "string", "index,sort:desc")
	m := mdl("Thing", "things", fld("ID", "int64", "pk"), f)
	f.Path = []int{1, 0}
	m.Indexes = []schema.Index{{Name: "things_slug_idx", Parts: []schema.IndexPart{{Column: "slug"}}}}
	s, diags := build(t, pg(), m)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	tb, _ := s.Table("things")
	if len(tb.Indexes) != 1 || !tb.Indexes[0].Parts[0].Desc {
		t.Fatalf("indexes=%+v", tb.Indexes)
	}
}

func TestTagsWithoutSchemaEffectAreAccepted(t *testing.T) {
	m := mdl("Thing", "things",
		fld("ID", "int64", "pk"),
		fld("Deleted", "*time.Time", "soft_delete:timestamp"),
		fld("V", "int64", "version"),
		fld("C", "int64", "created_at:unix"),
		fld("U", "int64", "updated_at:unixmilli"),
		fld("R", "string", "read_only"),
		fld("CO", "string", "create_only"),
		fld("I", "string", "immutable"),
		fld("S", "string", "sensitive"),
	)
	plain := mdl("Plain", "plains",
		fld("ID", "int64", "pk"),
		fld("Deleted", "*time.Time", ""),
		fld("V", "int64", ""),
		fld("C", "int64", ""),
		fld("U", "int64", ""),
		fld("R", "string", ""),
		fld("CO", "string", ""),
		fld("I", "string", ""),
		fld("S", "string", ""),
	)
	s, diags := build(t, pg(), m, plain)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	a, _ := s.Table("things")
	b, _ := s.Table("plains")
	if len(a.Columns) != len(b.Columns) {
		t.Fatal("column count differs")
	}
	for i := range a.Columns {
		if a.Columns[i] != b.Columns[i] {
			t.Errorf("column %d differs: %+v vs %+v", i, a.Columns[i], b.Columns[i])
		}
	}
}

func TestTagMisuseIsDiagnosed(t *testing.T) {
	cases := []struct{ name, goType, tag, want string }{
		{"size on int", "int64", "size:5", "size is only valid on string"},
		{"dim on string", "string", "dim:3", "dim is only valid on a vector"},
		{"sparsevec without dim", "vector.SparseVector", "", "sparsevec requires dim"},
		{"precision on string", "string", "precision:5,scale:1", "precision is only valid on float or decimal"},
		{"scale without precision", "float64", "scale:2", "scale requires precision"},
		{"notnull vs pointer", "*string", "notnull", "notnull conflicts with a nullable"},
		{"notnull vs nullable", "string", "notnull,nullable", "notnull conflicts with a nullable"},
		{"pk nullable", "*int64", "pk", "primary key column"},
		{"sort without index", "string", "sort:desc", "sort requires index or unique"},
		{"nulls without index", "string", "nulls:first", "nulls requires index or unique"},
		{"method without index", "string", "method:gin", "method requires index or unique"},
		{"size with type", "string", "size:3,type:citext", "size conflicts with type"},
		{"unknown go type", "Status", "", "has no SQL mapping"},
		{"undeclared enum", "string", "enum:nope", `enum "nope" is not declared`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := fld("X", c.goType, c.tag)
			_, diags := build(t, pg(), mdl("Thing", "things", fld("ID", "int64", "pk"), f))
			got := sdiagsString(diags)
			if !strings.Contains(got, c.want) {
				t.Fatalf("want %q in:\n%s", c.want, got)
			}
			for _, d := range diags {
				if d.Pos.Filename != "m.go" || d.Pos.Line == 0 {
					t.Fatalf("unpositioned: %v", d)
				}
			}
		})
	}
}

func TestCompanionMergedWithTags(t *testing.T) {
	m := mdl("Doc", "docs",
		fld("ID", "int64", "pk"),
		fld("Title", "string", "index,check:length(title) > 0"),
		fld("Body", "string", ""),
	)
	m.Config.Indexes = []schema.Index{{Name: "docs_body_idx", Parts: []schema.IndexPart{{Column: "body"}}, Where: "body <> ''"}}
	m.Config.Uniques = []schema.Unique{{Name: "docs_title_key", Columns: []string{"title"}}}
	m.Config.Checks = []schema.Check{{Name: "docs_body_nonempty", Expr: "length(body) > 0"}}
	m.Config.Enums = []schema.Enum{{Name: "b_enum", Values: []string{"x"}}, {Name: "a_enum", Values: []string{"y"}}}
	m.Config.Defaults["body"] = "''"
	m.Config.Comments["title"] = "the title"
	m.Config.Collates["title"] = "C"
	m.Config.RLS = &schema.RLS{Enabled: true}
	m.Config.Partition = &schema.Partition{Strategy: "range", Key: "RANGE (id)"}
	m.Config.Views = []schema.View{{Name: "v2", Query: "select 1"}, {Name: "v1", Query: "select 2"}}
	m.Config.Policies = []schema.Policy{{Table: "docs", Name: "p", Command: "ALL", Permissive: true}}
	s, diags := build(t, pg(), m)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	tb, _ := s.Table("docs")
	if _, ok := tb.Index("docs_title_idx"); !ok {
		t.Error("tag index missing")
	}
	if ix, ok := tb.Index("docs_body_idx"); !ok || ix.Where != "body <> ''" {
		t.Errorf("companion index missing: %+v", ix)
	}
	if len(tb.Uniques) != 1 || len(tb.Checks) != 2 {
		t.Errorf("uniques=%v checks=%v", tb.Uniques, tb.Checks)
	}
	if c := mustCol(t, s, "docs", "body"); c.Default != "''" {
		t.Errorf("default=%q", c.Default)
	}
	if c := mustCol(t, s, "docs", "title"); c.Comment != "the title" || c.Collate != "C" {
		t.Errorf("title=%+v", c)
	}
	if tb.RLS == nil || !tb.RLS.Enabled || tb.Partition == nil {
		t.Errorf("rls=%v partition=%v", tb.RLS, tb.Partition)
	}
	if len(s.Enums) != 2 || s.Enums[0].Name != "a_enum" {
		t.Errorf("enums=%+v", s.Enums)
	}
	if len(s.Views) != 2 || s.Views[0].Name != "v1" || len(s.Policies) != 1 {
		t.Errorf("views=%+v policies=%+v", s.Views, s.Policies)
	}
}

func TestCompanionConflictsAreDiagnosed(t *testing.T) {
	base := func() *Model {
		return mdl("Doc", "docs",
			fld("ID", "int64", "pk"),
			fld("Title", "string", "index,default:'a',comment:one,collate:C,check:x > 1"),
		)
	}
	cases := []struct {
		name string
		mut  func(m *Model)
		want string
		line int
	}{
		{"default", func(m *Model) { m.Config.Defaults["title"] = "'b'" }, `column "title": default is set by both`, 15},
		{"comment", func(m *Model) { m.Config.Comments["title"] = "two" }, `column "title": comment is set by both`, 15},
		{"collate", func(m *Model) { m.Config.Collates["title"] = "POSIX" }, `column "title": collate is set by both`, 15},
		{"index name", func(m *Model) {
			m.Config.Indexes = []schema.Index{{Name: "docs_title_idx", Parts: []schema.IndexPart{{Column: "title"}}}}
		}, `index name "docs_title_idx" is declared by both`, 3},
		{"unique vs index name", func(m *Model) {
			m.Config.Uniques = []schema.Unique{{Name: "docs_title_idx", Columns: []string{"title"}}}
		}, `index name "docs_title_idx" is declared by both`, 3},
		{"check name", func(m *Model) {
			m.Config.Checks = []schema.Check{{Name: "docs_title_check", Expr: "y"}}
		}, `check name "docs_title_check" is declared by both`, 3},
		{"unknown column", func(m *Model) { m.Config.Comments["ghost"] = "x" }, `column "ghost" is not a column of table docs`, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := base()
			c.mut(m)
			_, diags := build(t, pg(), m)
			var hit *Diagnostic
			for i := range diags {
				if strings.Contains(diags[i].Msg, c.want) {
					hit = &diags[i]
				}
			}
			if hit == nil {
				t.Fatalf("want %q in:\n%s", c.want, sdiagsString(diags))
			}
			if hit.Pos.Filename != "m.go" || hit.Pos.Line != c.line {
				t.Fatalf("position %v, want m.go:%d", hit.Pos, c.line)
			}
		})
	}
}

func TestEnumConflictAcrossModels(t *testing.T) {
	a := mdl("A", "as", fld("ID", "int64", "pk"))
	a.Config.Enums = []schema.Enum{{Name: "e", Values: []string{"x"}}}
	b := mdl("B", "bs", fld("ID", "int64", "pk"))
	b.Config.Enums = []schema.Enum{{Name: "e", Values: []string{"x"}}}
	if s, diags := build(t, pg(), a, b); len(diags) != 0 || len(s.Enums) != 1 {
		t.Fatalf("identical enums must merge: %v %v", diags, s.Enums)
	}
	b.Config.Enums = []schema.Enum{{Name: "e", Values: []string{"y"}}}
	_, diags := build(t, pg(), a, b)
	if !strings.Contains(sdiagsString(diags), `enum "e" is declared with different values`) {
		t.Fatalf("diags: %v", diags)
	}
}

func TestDuplicateTableIsDiagnosed(t *testing.T) {
	a := mdl("A", "same", fld("ID", "int64", "pk"))
	b := mdl("B", "same", fld("ID", "int64", "pk"))
	_, diags := build(t, pg(), a, b)
	if !strings.Contains(sdiagsString(diags), `table "same" is produced by models A and B`) {
		t.Fatalf("diags: %v", diags)
	}
}

func TestSQLiteDialectDiagnostics(t *testing.T) {
	m := mdl("Doc", "docs",
		fld("ID", "int64", "pk"),
		fld("E", "vector.Vector", "dim:3,index:e_idx,method:hnsw"),
		fld("H", "vector.HalfVector", "dim:3,index:h_idx,method:ivfflat"),
		fld("S", "vector.SparseVector", "dim:3"),
		fld("B", "vector.Bits", "dim:3"),
		fld("P", "string", "index"),
	)
	m.Config.Indexes = []schema.Index{{Name: "c_idx", Method: "hnsw", Parts: []schema.IndexPart{{Column: "e"}}}}
	_, diags := build(t, Config{Dialects: []dialect.Name{dialect.SQLite}}, m)
	got := sdiagsString(diags)
	for _, w := range []string{"vector is not supported on sqlite", "halfvec is not supported on sqlite",
		"sparsevec is not supported on sqlite", "bit is not supported on sqlite",
		"hnsw is not supported on sqlite", "ivfflat is not supported on sqlite"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if _, d2 := build(t, pg(), m); len(d2) != 0 {
		t.Fatalf("postgres must accept everything: %v", d2)
	}
	if _, d3 := build(t, Config{}, m); len(d3) != 0 {
		t.Fatalf("no dialects listed means no dialect diagnostics: %v", d3)
	}
}

func TestSQLiteTypesAndNoExtensions(t *testing.T) {
	m := mdl("Thing", "things",
		fld("ID", "int64", "pk"),
		fld("Name", "string", ""),
		fld("Score", "float64", ""),
		fld("Blob", "[]byte", ""),
		fld("Status", "string", "enum:st"),
	)
	m.Config.Enums = []schema.Enum{{Name: "st", Values: []string{"a"}}}
	s, diags := build(t, Config{Dialects: []dialect.Name{dialect.SQLite}}, m)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	if s.Dialect != dialect.SQLite || len(s.Extensions) != 0 {
		t.Fatalf("dialect=%s ext=%v", s.Dialect, s.Extensions)
	}
	for col, typ := range map[string]string{"id": "INTEGER", "name": "TEXT", "score": "REAL", "blob": "BLOB", "status": "TEXT"} {
		if c := mustCol(t, s, "things", col); c.Type != typ {
			t.Errorf("%s=%q want %q", col, c.Type, typ)
		}
	}
}

func relModels() (*Model, *Model) {
	doc := mdl("Document", "documents", fld("ID", "int64", "pk"), fld("AuthorID", "int64", ""))
	tag := mdl("Tag", "tags", fld("ID", "string", "pk"))
	return doc, tag
}

func TestBelongsToForeignKey(t *testing.T) {
	doc, author := relModels()
	author.GoName, author.Table = "Author", "authors"
	author.Fields[0].GoType = "int64"
	doc.Relations = []*Relation{{GoName: "Author", Kind: BelongsTo, Target: author, FK: "AuthorID",
		OnDelete: "set_null", OnUpdate: "cascade", Pos: token.Position{Filename: "m.go", Line: 40, Column: 2}}}
	s, diags := build(t, pg(), doc, author)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	tb, _ := s.Table("documents")
	if len(tb.ForeignKeys) != 1 {
		t.Fatalf("fks=%+v", tb.ForeignKeys)
	}
	fk := tb.ForeignKeys[0]
	if fk.Name != "documents_author_id_fkey" || fk.RefTable != "authors" || fk.Columns[0] != "author_id" ||
		fk.RefColumns[0] != "id" || fk.OnDelete != "SET NULL" || fk.OnUpdate != "CASCADE" {
		t.Fatalf("fk=%+v", fk)
	}
	doc.Relations[0].NoConstraint = true
	s, _ = build(t, pg(), doc, author)
	if tb, _ = s.Table("documents"); len(tb.ForeignKeys) != 0 {
		t.Fatalf("no_constraint must omit the fk: %+v", tb.ForeignKeys)
	}
	doc.Relations[0].NoConstraint = false
	doc.Relations[0].FK = "Missing"
	_, diags = build(t, pg(), doc, author)
	if !strings.Contains(sdiagsString(diags), "m.go:40:2: relation Author: foreign key field Missing") {
		t.Fatalf("diags: %v", diags)
	}
	doc.Relations[0].FK = "AuthorID"
	doc.Relations[0].OnDelete = "explode"
	_, diags = build(t, pg(), doc, author)
	if !strings.Contains(sdiagsString(diags), `on_delete "explode" is not valid`) {
		t.Fatalf("diags: %v", diags)
	}
}

func TestManyToManyJoinTable(t *testing.T) {
	doc, tag := relModels()
	doc.Relations = []*Relation{{GoName: "Tags", Kind: ManyToMany, Target: tag}}
	s, diags := build(t, pg(), doc, tag)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	jt, ok := s.Table("documents_tags")
	if !ok {
		t.Fatalf("join table missing: %+v", s.Tables)
	}
	if len(jt.Columns) != 2 || jt.Columns[0].Name != "document_id" || jt.Columns[0].Type != "bigint" ||
		jt.Columns[1].Name != "tag_id" || jt.Columns[1].Type != "text" {
		t.Fatalf("columns=%+v", jt.Columns)
	}
	if got := strings.Join(jt.PrimaryKey, ","); got != "document_id,tag_id" {
		t.Fatalf("pk=%s", got)
	}
	if len(jt.ForeignKeys) != 2 {
		t.Fatalf("fks=%+v", jt.ForeignKeys)
	}
	for _, fk := range jt.ForeignKeys {
		if fk.OnDelete != "CASCADE" {
			t.Errorf("fk=%+v", fk)
		}
	}
	// The inverse declaration must not create a second join table.
	tag.Relations = []*Relation{{GoName: "Documents", Kind: ManyToMany, Target: doc, JoinTable: "documents_tags"}}
	s, diags = build(t, pg(), doc, tag)
	if len(diags) != 0 || len(s.Tables) != 3 {
		t.Fatalf("diags=%v tables=%d", diags, len(s.Tables))
	}
	// An explicit join: name wins.
	doc.Relations[0].JoinTable = "doc_tag_links"
	tag.Relations = nil
	s, _ = build(t, pg(), doc, tag)
	if _, ok := s.Table("doc_tag_links"); !ok {
		t.Fatal("explicit join table name ignored")
	}
}

func TestBuildSchemaIsCanonicalAndDeterministic(t *testing.T) {
	mk := func() []*Model {
		doc, tag := relModels()
		doc.Relations = []*Relation{{GoName: "Tags", Kind: ManyToMany, Target: tag}}
		doc.Fields = append(doc.Fields, fld("Z", "string", "index"), fld("A", "string", "unique"))
		doc.Config.Enums = []schema.Enum{{Name: "z", Values: []string{"1"}}, {Name: "a", Values: []string{"1"}}}
		doc.Config.Checks = []schema.Check{{Name: "zz", Expr: "1=1"}, {Name: "aa", Expr: "1=1"}}
		return []*Model{doc, tag}
	}
	a, _ := BuildSchema(mk(), pg())
	ms := mk()
	ms[0], ms[1] = ms[1], ms[0]
	b, _ := BuildSchema(ms, pg())
	if !schema.Equal(a, b) {
		t.Fatal("model order changed the schema")
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("not byte-identical:\n%s\n%s", ja, jb)
	}
	c, _ := BuildSchema(mk(), pg())
	if !schema.Equal(a, c) {
		t.Fatal("two runs differ")
	}
	sorted := *a
	sorted.Tables = append([]schema.Table(nil), a.Tables...)
	sorted.Sort()
	jsorted, _ := json.Marshal(&sorted)
	if string(jsorted) != string(ja) {
		t.Fatal("output is not already in schema.Sort order")
	}
	if a.Tables[0].Name != "documents" || a.Tables[1].Name != "documents_tags" || a.Tables[2].Name != "tags" {
		t.Fatalf("tables unsorted: %v", a.Tables)
	}
	if a.Tables[0].Columns[0].Name != "id" || a.Tables[0].Columns[2].Name != "z" {
		t.Fatal("column order must stay declaration order")
	}
}

func TestNamedTypesMapLikeTheirUnderlyingType(t *testing.T) {
	g, diags, err := Load([]string{"./namedtype"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sdiags := BuildSchema(g.Models, pg())
	if len(sdiags) != 0 {
		t.Fatalf("diags: %v", sdiags)
	}
	for col, typ := range map[string]string{"id": "bigint", "label": "text", "note": "text", "opt": "text", "created": "timestamptz"} {
		if c := mustCol(t, s, "items", col); c.Type != typ {
			t.Errorf("%s=%q want %q", col, c.Type, typ)
		}
	}
	if !mustCol(t, s, "items", "note").Nullable || !mustCol(t, s, "items", "opt").Nullable {
		t.Error("pointer and sql.Null columns must be nullable")
	}
}

func TestUnsupportedTypeIsPositionedDiagnostic(t *testing.T) {
	g, diags, err := Load([]string{"./unsupportedtype"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "field Value") || !strings.Contains(diags[0].Msg, "complex128") ||
		!strings.HasSuffix(diags[0].Pos.Filename, "unsupported.go") || diags[0].Pos.Line != 10 {
		t.Fatalf("diags: %v", diags)
	}
	if g.Schema == nil {
		t.Fatal("schema must still be returned")
	}
}

func TestLoadBuildsGraphSchema(t *testing.T) {
	g, diags, err := Load([]string{"./companion"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	if _, ok := g.Schema.Table("docs"); !ok {
		t.Fatalf("Graph.Schema lacks docs: %+v", g.Schema)
	}
}
