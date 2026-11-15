# fulltext-search — implementation plan

Status: draft
Spec: .procoder/specs/fulltext-search.md

## Goal

Full-text search on PostgreSQL, SQLite (FTS5) and MySQL/MariaDB (FULLTEXT) through a declaration on the model, migrations and inspection for each dialect, one portable query layer in package `fts`, and `momo.HybridSearch` on SQLite and MariaDB.

## Architecture

A new `schema.FullText` description hangs off `schema.Index` (Method `"fulltext"`); the model declares it with the `search` tag or `c.FullText(...)` and every dialect renderer derives its own structures from it (PostgreSQL: stored generated tsvector column plus GIN, SQLite: external-content FTS5 table plus three sync triggers, MySQL: `FULLTEXT` index). The query side is a small `Target`/`Query` pair in package `fts` that renders `@@`, FTS5 `MATCH` or `MATCH ... AGAINST` from one shared query parser (`internal/ftsquery`, a leaf package so that the root package can use it for hybrid search); `momo.HybridSearch` gains two statement templates that fuse the same RRF legs with `UNION ALL`.

Design decisions the spec leaves open, fixed here so every task agrees (each is a mechanism, not a change of any recorded decision Q1 to Q10):

- D1. Engine hook. The spec says the snapshot records only the declaration and the dialect derives the rest (Q2), yet `Engine.Diff` compares two schemas that must both contain the derived objects. A new optional interface `migrate.SchemaExpander` (`ExpandSchema(s *schema.Schema) (*schema.Schema, error)`, pure, idempotent) is applied to the desired schema and to the snapshot schema at every place `SchemaValidator` is called and wherever a snapshot is compared with a live inspection (Diff, AutoMigrate, Drift, Baseline). PostgreSQL expansion adds the generated column; SQLite expansion adds the three sync triggers and renames the index to `<table>_fts`.
- D2. Names. `schema.FTS5Table(table)` is `table + "_fts"`; `schema.TSVectorColumn(index)` is `index + "_tsv"`. Both the renderers and the query layer call these two functions, nothing else spells the names.
- D3. Options a dialect ignores are dropped by `FullText.ForDialect(d)` before comparison: SQLite and MySQL ignore `Config`, PostgreSQL and MySQL ignore `Tokenizer`, PostgreSQL and SQLite ignore `Parser`, SQLite and MySQL ignore `Weights` (they are query-time on SQLite, a no-op on MySQL). So a changed weight on SQLite is no migration.
- D4. One query parser. `internal/ftsquery.Parse(text, syntax)` produces `Parsed{Groups [][]Term, Excluded []Term}` where `Term{Text string, Phrase, Prefix bool}`; `Parsed.PostgresTSQuery()`, `Parsed.FTS5()`, `Parsed.MySQLBoolean()` render it. The PostgreSQL web form without a prefix term still binds the raw text to `websearch_to_tsquery` (spec Q5); only a query containing a prefix term is rendered as `to_tsquery` text with every lexeme quoted.
- D5. On PostgreSQL a declared target writes the configuration as a quoted literal inside `CAST('english' AS regconfig)`, validated against `^[A-Za-z_][A-Za-z0-9_]*$` at declaration time, so the expression matches the generated column and the expression index textually. A caller-supplied `Query.Config(name)` that differs from the declared configuration is `ErrValidation`; a target over a hand-managed tsvector column (`fts.OverVector`) keeps the bound-parameter regconfig of the existing builders.
- D6. Hybrid reads the declaration from `Meta.TableSchema` (already filled by generated code, `meta.go`). `HybridSpec.Text` is checked for membership in the declared columns; the text leg searches all declared columns (FTS5 `MATCH` on the table, `MATCH(c1, c2, ...)` on MariaDB), because the FULLTEXT index and the FTS5 table cover the whole declaration.

## Constraints

- Backward compatible: every current `fts` export keeps its signature and rendered SQL on PostgreSQL. The `ErrUnsupported` behaviour of the PostgreSQL builders on other dialects stays; only what the portable layer adds works elsewhere.
- Security: query text, configuration names, tokenizer names and options are bound parameters wherever the dialect allows it. Where a dialect forces an identifier or literal into the SQL (FTS5 table and column names, `WITH PARSER`, a tokenizer clause in `CREATE VIRTUAL TABLE`), the value is validated against a strict allowlist or quoted with the dialect's identifier quoting. Sensitive-column masking in hook events (`FlagSensitive`) keeps working as it does in `fts` today.
- Dialect versions: PostgreSQL 12 or newer for stored generated columns (11 already has `websearch_to_tsquery`); SQLite with the FTS5 module compiled in (the cgo `driver/sqlitedriver` and the pure-Go `driver/sqlitepure` must each be checked in CI for FTS5 and bm25); MySQL 9.x and MariaDB 11.8, the versions CI runs. Anything unavailable is `ErrUnsupported` at build time or `ErrValidation`, never a raw driver string.
- No new dependency in the root module. A change that adds one takes the nested go.sum diff from CI.
- Verification only on CI. Nothing is built or tested on the laptop.
- Migrations stay forward-only and diff-clean: re-running `diff` after `apply` yields zero changes on every dialect.

## Spec gaps found while planning (decided 2026-11-15, accepted as written; the spec now matches)

- G1. Spec Q6 asks `migrate lint` to warn "for a PostgreSQL search whose config differs from the index". `migrate lint` reads migration SQL and never sees a query, so it cannot know the search config. This plan covers the case at build time instead (an override that differs from the declaration is `ErrValidation`, Task 6) and writes no lint rule for it. No acceptance criterion exists for the lint rule.
- G2. Spec S-7 asks `WITH PARSER ngram` on "MySQL/MariaDB" and the round trip "on both servers". MariaDB 11.8 InnoDB ships no ngram parser. This plan makes a declared `Parser` on MariaDB an `ErrUnsupported` at render time and runs the parser round trip on MySQL only (Task 4).- G3. Spec S-7 asks a FULLTEXT on a non-InnoDB table to fail with `ErrValidation`, but `schema.Table` has no engine and momo creates only InnoDB tables, so the check has nothing to read. Task 4 adds `schema.Table.Engine` (set by the MySQL inspector for a non-InnoDB table, never by a declaration) and the validation rejects a table that carries an engine other than InnoDB and a FullText index.
- G4. Spec Q6 says lint warns "for adding the first InnoDB FULLTEXT index". The lint sees statements, not the table's existing indexes. Task 8 warns on every `CREATE FULLTEXT INDEX` on a table of the parent snapshot, which is a superset of the first-index case.
- G5. Spec S-8 says a PostgreSQL tsvector generated column "maps to a read-only `*types.TSVector`". With D1 the generated column is implied by the declaration, so `--from-db` omits that field (Task 5) instead of emitting a field the expansion would then have to match textually.

## Task 1: Schema description, tag and config declaration

Covers: S-4 (both criteria: `TestGenerateFullTextDeclaration`, `TestSchemaFullTextSnapshotCompat`).

Files:

- Modify `schema/schema.go`: add `FullText *FullText` to `Index`.
- Create `schema/fulltext.go`: `FullText`, `FTS5Table`, `TSVectorColumn`, `ForDialect`, `Validate`, `(*Table).FullTextIndex`.
- Create `schema/fulltext_test.go`.
- Modify `meta_companion.go`: `FullTextOpts` and `(*Config).FullText`.
- Modify `gen/tags/tags.go` and `gen/tags/tags_test.go`: the `search` option.
- Modify `gen/companion.go`: read `c.FullText(...)` and its options.
- Modify `gen/schema.go`: build the index from the tag or the call, reject a second declaration and a non-text field.
- Create `gen/fulltext_test.go` and the fixture packages `gen/testdata/fixtures/fulltexttag`, `fulltextcall`, `fulltextbad`.

Interfaces:

```go
// package schema
type FullText struct {
	Columns    []string `json:"columns"`              // source columns, in order
	Config     string   `json:"config,omitempty"`     // PostgreSQL regconfig
	Weights    []string `json:"weights,omitempty"`    // "A".."D", one per column
	Tokenizer  string   `json:"tokenizer,omitempty"`  // SQLite FTS5 tokenize clause
	Parser     string   `json:"parser,omitempty"`     // MySQL WITH PARSER
	Expression bool     `json:"expression,omitempty"` // PostgreSQL: expression index, no generated column
}
// Index gains: FullText *FullText `json:"fulltext,omitempty"`
func FTS5Table(table string) string                // table + "_fts"
func TSVectorColumn(index string) string           // index + "_tsv"
func (f FullText) Validate() error                 // allowlists below
func (f FullText) ForDialect(d dialect.Name) FullText
func (t *Table) FullTextIndex() (*Index, bool)     // the index with Method "fulltext" and FullText != nil

// package momo (meta_companion.go)
type FullTextOpts struct{}
func (c *Config) FullText(name string, cols ...any) *FullTextOpts
func (o *FullTextOpts) Config(regconfig string) *FullTextOpts
func (o *FullTextOpts) Weights(w ...string) *FullTextOpts
func (o *FullTextOpts) Tokenizer(t string) *FullTextOpts
func (o *FullTextOpts) Parser(p string) *FullTextOpts
func (o *FullTextOpts) Expression() *FullTextOpts

// package tags: Tags gains `Search bool`
```

Validate allowlists: `Columns` non-empty without repeats; `Config` matches `^[A-Za-z_][A-Za-z0-9_]*$`; `Weights` empty or one of `A B C D` per column; `Tokenizer` matches `^(porter )?(unicode61|ascii|trigram)( remove_diacritics [0-2])?$`; `Parser` is `ngram` or `mecab`; `Expression` with `Weights` is invalid.

- [ ] Step 1. Write the failing snapshot test `schema/fulltext_test.go`. Expect FAIL with "undefined: FullText".

```go
package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Catches: a FullText field lost by the JSON snapshot, and an old snapshot
// (no "fulltext" key) failing to load.
func TestSchemaFullTextSnapshotCompat(t *testing.T) {
	in := &Schema{FormatVersion: FormatVersion, Dialect: "postgres", Tables: []Table{{
		Name: "articles",
		Columns: []Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: "text"}, {Name: "body", Type: "text"}},
		Indexes: []Index{{
			Name: "articles_search", Method: "fulltext",
			Parts: []IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &FullText{Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"},
				Tokenizer: "porter unicode61", Parser: "ngram", Expression: true},
		}},
	}}}
	var buf bytes.Buffer
	if err := in.Write(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	got := out.Tables[0].Indexes[0].FullText
	if got == nil || got.Config != "english" || got.Weights[1] != "B" || got.Tokenizer != "porter unicode61" || got.Parser != "ngram" || !got.Expression {
		t.Fatalf("full-text description lost in the round trip: %+v", got)
	}
	old := `{"format_version":1,"dialect":"postgres","tables":[{"name":"t","columns":[{"name":"id","type":"bigint"}],"indexes":[{"name":"i","parts":[{"column":"id"}]}]}]}`
	if _, err := Read(strings.NewReader(old)); err != nil {
		t.Fatalf("snapshot written before this change: %v", err)
	}
	if !json.Valid([]byte(old)) {
		t.Fatal("fixture is not JSON")
	}
}

// Catches: a hostile configuration, tokenizer or parser reaching rendered SQL.
func TestFullTextValidate(t *testing.T) {
	ok := FullText{Columns: []string{"a"}, Config: "english", Tokenizer: "porter unicode61", Parser: "ngram"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid description: %v", err)
	}
	bad := []FullText{
		{},
		{Columns: []string{"a", "a"}},
		{Columns: []string{"a"}, Config: "english'); DROP TABLE x;--"},
		{Columns: []string{"a"}, Tokenizer: "unicode61'); DROP TABLE x;--"},
		{Columns: []string{"a"}, Parser: "ngram; x"},
		{Columns: []string{"a"}, Weights: []string{"A", "B"}},
		{Columns: []string{"a"}, Weights: []string{"E"}},
		{Columns: []string{"a"}, Weights: []string{"A"}, Expression: true},
	}
	for i, f := range bad {
		if err := f.Validate(); err == nil {
			t.Errorf("case %d accepted: %+v", i, f)
		}
	}
	if FTS5Table("articles") != "articles_fts" || TSVectorColumn("articles_search") != "articles_search_tsv" {
		t.Fatal("name functions drifted")
	}
}
```

- [ ] Step 2. Implement `schema/fulltext.go` and the `FullText` field on `Index` exactly as in Interfaces. `ForDialect` clears the options of D3. `Validate` uses `regexp.MustCompile` constants for the three allowlists. `FullTextIndex` scans `t.Indexes` for `Method == "fulltext" && FullText != nil`.
- [ ] Step 3. Write the failing generator test `gen/fulltext_test.go` and the three fixtures. Expect FAIL with "undefined: FullTextOpts" (compile of the `fulltextcall` fixture) and, once that compiles, "unknown tag option search".

Fixture `gen/testdata/fixtures/fulltexttag/ok.go`:

```go
package fulltexttag

import "github.com/azrtydxb/momo"

type Article struct {
	momo.Model `momo:"table:articles"`
	ID         int64  `momo:"pk,identity"`
	Title      string `momo:"search"`
	Body       string `momo:"search"`
}
```

Fixture `gen/testdata/fixtures/fulltextcall/ok.go`:

```go
package fulltextcall

import "github.com/azrtydxb/momo"

type Article struct {
	momo.Model `momo:"table:articles"`
	ID         int64 `momo:"pk,identity"`
	Title      string
	Body       string
}

func (a *Article) MomoConfig(c *momo.Config) {
	c.FullText("articles_search", &a.Title, &a.Body)
}
```

Fixture `gen/testdata/fixtures/fulltextbad/bad.go` (one model with `search` on an `int64` field named `Views`, one model whose config names a field of another struct, one model with the tag on a field and a `c.FullText` call):

```go
package fulltextbad

import "github.com/azrtydxb/momo"

type Counter struct {
	momo.Model `momo:"table:counters"`
	ID         int64 `momo:"pk,identity"`
	Views      int64 `momo:"search"`
}

type Both struct {
	momo.Model `momo:"table:boths"`
	ID         int64  `momo:"pk,identity"`
	Title      string `momo:"search"`
	Body       string
}

func (b *Both) MomoConfig(c *momo.Config) {
	c.FullText("boths_search", &b.Title, &b.Body)
}
```

Test:

```go
package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func fullTextIndex(t *testing.T, pkg string, d dialect.Name) schema.Index {
	t.Helper()
	g, diags, err := Load([]string{"./" + pkg}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load %s: %v %v", pkg, err, diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{d}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diagnostics: %v", sdiags)
	}
	tbl, ok := s.Table("articles")
	if !ok {
		t.Fatal("articles table missing")
	}
	ix, ok := tbl.FullTextIndex()
	if !ok {
		t.Fatal("no full-text index on articles")
	}
	return *ix
}

// Catches: the tag and the explicit call producing different declarations, and
// a source column order that does not follow the call.
func TestGenerateFullTextDeclaration(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		a := fullTextIndex(t, "fulltexttag", d)
		b := fullTextIndex(t, "fulltextcall", d)
		if a.Name != "articles_search" || a.Method != "fulltext" {
			t.Fatalf("%s: tag form = %+v", d, a)
		}
		if got := strings.Join(a.FullText.Columns, ","); got != "title,body" {
			t.Fatalf("%s: columns = %s", d, got)
		}
		if !schema.Equal(&schema.Schema{Dialect: d, Tables: []schema.Table{{Name: "x", Indexes: []schema.Index{a}}}},
			&schema.Schema{Dialect: d, Tables: []schema.Table{{Name: "x", Indexes: []schema.Index{b}}}}) {
			t.Fatalf("%s: tag and call differ:\n%+v\n%+v", d, a, b)
		}
	}
}

// Catches: the tag accepted on a non-text field, a second declaration
// accepted, and a diagnostic that does not name the field.
func TestGenerateFullTextDeclarationRejected(t *testing.T) {
	g, _, err := Load([]string{"./fulltextbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, diags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	text := diagStrings(diags)
	for _, want := range []string{"field Views", "search needs a text column", "one full-text declaration per table"} {
		if !strings.Contains(text, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, text)
		}
	}
}
```

- [ ] Step 4. Implement. In `gen/tags/tags.go` add `Search bool` to `Tags` and parse the bare option `search` next to `index`. In `meta_companion.go` add `FullTextOpts` and the methods, all no-ops like `IndexOpts`. In `gen/companion.go` extend the receiver check in the chain loop (`want := "FullTextOpts"` when `calls[0].name == "FullText"`), route `FullText` to a new `r.fullText(calls)` that mirrors `r.index` (name from `args[0]`, one `schema.IndexPart{Column: f.Column}` per field, the options `Config`, `Weights`, `Tokenizer`, `Parser`, `Expression` through the same `once` duplicate check) and appends `schema.Index{Name: name, Method: "fulltext", Parts: parts, FullText: &ft}` to `r.res.Indexes`. In `gen/schema.go` `applyIndexTags` collect the fields tagged `search` in field order into one `schema.Index{Name: m.Table + "_search", Method: "fulltext", ...}`; add a helper `isTextType(typ string) bool` (prefix `text`, `varchar`, `char`, `character`, `tinytext`, `mediumtext`, `longtext`, case-insensitive after `schema.NormalizeType`); report `field <GoName>: search needs a text column, it is <type>` per bad field, and `table <t>: one full-text declaration per table` when a table has two. Call `FullText.Validate` and turn its error into a diagnostic naming the index. The Index goes through `addIndex` so the name collision check applies.
- [ ] Step 5. Verify on CI. Push the branch and read the `unit` and `lint` jobs of the run (poll at most every 10 minutes). Expect both green. The generated literal in `gen/emit_meta.go` already renders a `*schema.FullText` (its `lit` handles pointers and string slices); the golden `gen/testdata/golden` stays unchanged.
- [ ] Step 6. Commit in a separate call: `feat(schema): declare full-text search on a model`.

## Task 2: PostgreSQL declarative full-text (generated tsvector plus GIN)

Covers: S-5 (all three criteria: `TestFullTextPostgresMigrationRoundTrip`, `TestFullTextConfigChangeDiff` diff half, `TestTSVectorMigrationRoundTrip` unchanged); the lint half of the config-change criterion is Task 8.

Files:

- Modify `migrate/dialect.go`: the `SchemaExpander` interface and `expand` helper.
- Modify `migrate/engine.go`, `migrate/automigrate.go`, `migrate/drift.go`, `migrate/baseline.go`: call `expand` on the desired and snapshot schemas where `SchemaValidator` is called and where a snapshot is compared with a live one.
- Create `migrate/postgres/fulltext.go`: `ExpandSchema`, the generation expression, the index SQL.
- Modify `migrate/postgres/render.go`: `createIndex` writes `USING gin` for a FullText index; the generated-column change path.
- Create `migrate/postgres/fulltext_test.go` (unit) and `migrate/postgres/fulltext_integration_test.go` (integration).

Interfaces:

```go
// package migrate
type SchemaExpander interface {
	// ExpandSchema returns a copy of s with the objects the dialect derives
	// from declarations added. It is pure and idempotent.
	ExpandSchema(s *schema.Schema) (*schema.Schema, error)
}

// package postgres
func (Dialect) ExpandSchema(s *schema.Schema) (*schema.Schema, error)
func generationExpr(ft schema.FullText) string // setweight(to_tsvector('english', coalesce("title", '')), 'A') || ...
```

Expansion rules for a table with `FullTextIndex()`: validate the description (`ErrValidation` wrapping the `Validate` error, naming table and index); Stored form (default): add the column `schema.Column{Name: schema.TSVectorColumn(ix.Name), Type: "tsvector", Generated: generationExpr(ft)}` unless a column of that name with the same generation expression exists, set `ix.Parts = []IndexPart{{Column: thatColumn}}`; Expression form: `ix.Parts = []IndexPart{{Expr: "to_tsvector('english', coalesce(\"title\", '') || ' ' || coalesce(\"body\", ''))"}}`. The expression for column `c` with weight `w` is `setweight(to_tsvector('<config>', coalesce("<c>", '')), '<w>')`, joined by the concatenation operator; without weights the `setweight` wrapper is omitted. An empty `Config` is `ErrValidation` ("a PostgreSQL full-text declaration needs Config"). PostgreSQL older than 12 cannot be checked at render time; the docs (Task 10) state the floor.

- [ ] Step 1. Write the failing unit test `migrate/postgres/fulltext_test.go`. Expect FAIL with "undefined: Dialect.ExpandSchema" (compile error `d.ExpandSchema undefined`).

```go
package postgres

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func articlesWithFullText(ft schema.FullText) *schema.Schema {
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "title", Type: "text", Nullable: true},
			{Name: "body", Type: "text", Nullable: true},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts: []schema.IndexPart{{Column: "title"}, {Column: "body"}}, FullText: &ft}},
	}}}
}

// Catches: a NULL source column making the whole vector NULL (no coalesce), the
// configuration not pinned as a literal, and the index not being GIN on the
// generated column.
func TestFullTextPostgresRender(t *testing.T) {
	var d Dialect
	s, err := d.ExpandSchema(articlesWithFullText(schema.FullText{
		Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"}}))
	if err != nil {
		t.Fatal(err)
	}
	tbl := s.Tables[0]
	col, ok := tbl.Column("articles_search_tsv")
	if !ok || col.Type != "tsvector" {
		t.Fatalf("generated column missing: %+v", tbl.Columns)
	}
	want := `setweight(to_tsvector('english', coalesce("title", '')), 'A') || setweight(to_tsvector('english', coalesce("body", '')), 'B')`
	if col.Generated != want {
		t.Fatalf("generation expression\n got %s\nwant %s", col.Generated, want)
	}
	again, err := d.ExpandSchema(s)
	if err != nil || !schema.Equal(s, again) {
		t.Fatalf("expansion is not idempotent: %v", err)
	}
	changes, _, err := migrate.Diff(&schema.Schema{Dialect: "postgres"}, s, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := d.Render(changes)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, st := range stmts {
		sql = append(sql, st.SQL)
	}
	all := strings.Join(sql, ";\n")
	for _, frag := range []string{
		`"articles_search_tsv" tsvector GENERATED ALWAYS AS (` + want + `) STORED`,
		`CREATE INDEX "articles_search" ON "articles" USING gin ("articles_search_tsv")`,
	} {
		if !strings.Contains(all, frag) {
			t.Errorf("rendered SQL lacks %q:\n%s", frag, all)
		}
	}
}

// Catches: an empty configuration or a hostile one reaching the generated
// expression.
func TestFullTextPostgresValidation(t *testing.T) {
	var d Dialect
	for _, ft := range []schema.FullText{
		{Columns: []string{"title"}},
		{Columns: []string{"title"}, Config: "english'); DROP TABLE articles;--"},
	} {
		if _, err := d.ExpandSchema(articlesWithFullText(ft)); err == nil {
			t.Errorf("accepted %+v", ft)
		}
	}
}
```

- [ ] Step 2. Implement `migrate/postgres/fulltext.go` per the expansion rules; add the `SchemaExpander` interface and an unexported `expand(d Dialect, s *schema.Schema) (*schema.Schema, error)` in `migrate/dialect.go` that returns `s` unchanged when `d` does not implement it. In `createIndex` (`migrate/postgres/render.go`) treat `ix.FullText != nil` as `Method: "gin"` before the existing method switch.
- [ ] Step 3. Write the failing diff test for the config change in the same file. Expect FAIL with "want 4 statements" (the current path returns `a generated expression cannot be altered in place`).

```go
// Catches: a config change that is refused as "write a custom migration", and
// one that alters the column without dropping the index that depends on it.
func TestFullTextConfigChangeDiff(t *testing.T) {
	var d Dialect
	from, _ := d.ExpandSchema(articlesWithFullText(schema.FullText{Columns: []string{"title", "body"}, Config: "english"}))
	to, _ := d.ExpandSchema(articlesWithFullText(schema.FullText{Columns: []string{"title", "body"}, Config: "simple"}))
	changes, _, err := migrate.Diff(from, to, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.Order(changes)
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := d.Render(ordered)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, st := range stmts {
		sql = append(sql, st.SQL)
	}
	want := []string{
		`DROP INDEX "articles_search"`,
		`ALTER TABLE "articles" DROP COLUMN "articles_search_tsv"`,
		`ALTER TABLE "articles" ADD COLUMN "articles_search_tsv" tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce("title", '')) || to_tsvector('simple', coalesce("body", ''))) STORED`,
		`CREATE INDEX "articles_search" ON "articles" USING gin ("articles_search_tsv")`,
	}
	if strings.Join(sql, "\n") != strings.Join(want, "\n") {
		t.Fatalf("statements:\n%s\nwant:\n%s", strings.Join(sql, "\n"), strings.Join(want, "\n"))
	}
}
```

- [ ] Step 4. Implement the change path. In `migrate/postgres/render.go` where `after.Generated` differs from `before.Generated` (currently the `a generated expression cannot be altered in place` error) check whether the column is the generated column of a FullText index of the same table (the plan's `Change` set holds the index change; use `schema.TSVectorColumn(ix.Name) == column`). For that column render `ALTER TABLE ... DROP COLUMN` then `ALTER TABLE ... ADD COLUMN ... GENERATED ALWAYS AS (...) STORED`; every other generated column keeps the current refusal. The index change is already a drop plus a create because `Index.FullText.Config` differs; in `migrate/order.go` make the drop of an index precede the column change that it depends on (read the existing ordering for dropped indexes and add the generated column to its key). `DROP INDEX` of a FullText index renders as it does for any index.
- [ ] Step 5. Wire the hook. In `Engine.Diff`, `AutoMigrate`, `Drift`/`DriftWith` and `Baseline`, replace `to` and the snapshot schema by `expand(e.opts.dialect, ...)` right after `ValidateSchema` and after `LatestSnapshot` is read. Add `TestExpandSchemaHook` in `migrate/diff_engine_test.go` with a fake dialect implementing `SchemaExpander` that appends a table, asserting `Engine.Diff` plans a `create_table` for it.
- [ ] Step 6. Write the failing integration test `migrate/postgres/fulltext_integration_test.go` (build tag `integration`, in the style of `tsvector_roundtrip_test.go`). Expect FAIL with "relation \"articles_search_tsv\" does not exist" before Step 2 lands, so write it after the unit tests pass and push it with them.

```go
//go:build integration

package postgres_test

import (
	"context"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a second diff after apply that is not empty, and a NULL source
// column hiding a row that has text in the other column.
func TestFullTextPostgresMigrationRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testopen.Postgres(t)
	want := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "title", Type: "text", Nullable: true},
			{Name: "body", Type: "text", Nullable: true},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"}}}},
	}}}
	eng := migratetest.Engine(t, db, "postgres")
	migratetest.DiffApply(ctx, t, eng, want, "fulltext")
	m, err := eng.Diff(ctx, want, "again", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if m != nil {
		t.Fatalf("second diff is not empty: %v", m.Statements)
	}
	if _, err := db.Exec(ctx, `INSERT INTO articles (title, body) VALUES ('red fox', NULL), (NULL, 'red fox')`); err != nil {
		t.Fatal(err)
	}
	var n int
	row := db.QueryRow(ctx, `SELECT count(*) FROM articles WHERE articles_search_tsv @@ websearch_to_tsquery('english', 'fox')`)
	if err := row.Scan(&n); err != nil || n != 2 {
		t.Fatalf("matches = %d (%v), want 2: a NULL source column must not null the vector", n, err)
	}
}
```

If `migratetest` has no `DiffApply`/`Engine` helper of those names, use the helper the existing `tsvector_roundtrip_test.go` calls in the same position; the assertions are what the test pins.

- [ ] Step 7. Verify on CI: `unit`, `lint`, `integration` (the PostgreSQL service) green, with `TestTSVectorMigrationRoundTrip` unchanged and green. Poll at most every 10 minutes.
- [ ] Step 8. Commit in a separate call: `feat(migrate): declarative PostgreSQL full-text index`.

## Task 3: SQLite FTS5 table and sync triggers

Covers: S-6 (all six criteria: `TestFullTextSQLiteMigrationRoundTrip`, `TestFullTextSQLiteSync`, `TestFullTextSQLiteIntegrityAfterUpdateDelete`, `TestFullTextSQLiteNeedsIntegerKey`, `TestFullTextSQLiteDrop`, `TestFullTextSQLiteDriverSupport`).

Files:

- Create `migrate/sqlite/fulltext.go`: `ExpandSchema`, `createFTS5SQL`, `ftsTriggers`, `dropFTS5SQL`.
- Modify `migrate/sqlite/render.go`: `renderChange` for `CreateIndex` and `DropIndex` of a FullText index; `createIndexSQL` unchanged for ordinary indexes.
- Modify `migrate/order.go`: a trigger on a FullText content table is created after the virtual table and dropped before it.
- Modify `migrate/runner.go`: `execStatement` maps the hinted FTS5 failure.
- Create `migrate/sqlite/fulltext_test.go` (unit), `migrate/sqlite/fulltext_integration_test.go` (integration, `cgo && !windows`), `driver/sqlitepure/fulltext_integration_test.go`.

Interfaces:

```go
// package sqlite
func (Dialect) ExpandSchema(s *schema.Schema) (*schema.Schema, error)
func createFTS5SQL(table string, ix schema.Index, pk string) (string, error)
func ftsTriggers(table string, ft schema.FullText, pk string) []schema.Trigger // Definition set, names <fts>_ai, <fts>_ad, <fts>_au
```

The three triggers (names `<fts>_ai`, `<fts>_ad`, `<fts>_au`, with `fts = schema.FTS5Table(table)`), written once in `ftsTriggers` and compared textually on both sides:

```sql
CREATE TRIGGER "articles_fts_ai" AFTER INSERT ON "articles" BEGIN
  INSERT INTO "articles_fts"(rowid, "title", "body") VALUES (new."id", new."title", new."body");
END
CREATE TRIGGER "articles_fts_ad" AFTER DELETE ON "articles" BEGIN
  INSERT INTO "articles_fts"("articles_fts", rowid, "title", "body") VALUES ('delete', old."id", old."title", old."body");
END
CREATE TRIGGER "articles_fts_au" AFTER UPDATE ON "articles" BEGIN
  INSERT INTO "articles_fts"("articles_fts", rowid, "title", "body") VALUES ('delete', old."id", old."title", old."body");
  INSERT INTO "articles_fts"(rowid, "title", "body") VALUES (new."id", new."title", new."body");
END
```

The update trigger issues the `'delete'` command before the insert; the reverse order corrupts the index (`TestFullTextSQLiteIntegrityAfterUpdateDelete` pins it). `createFTS5SQL` renders `CREATE VIRTUAL TABLE "articles_fts" USING fts5("title", "body", content='articles', content_rowid='id'[, tokenize='<tokenizer>'])` followed, as a second statement, by `INSERT INTO "articles_fts"("articles_fts") VALUES('rebuild')`. The tokenizer was validated by `FullText.Validate`; single quotes in `content` and `content_rowid` come from identifiers that are re-checked with `^[A-Za-z_][A-Za-z0-9_]*$` and refused otherwise (`ErrValidation`). The first statement carries `Hint: "requires:fts5"`.

`ExpandSchema` for each table with a FullText index: `ErrValidation` naming the table when `len(PrimaryKey) != 1` or the key column's type does not normalize to `INTEGER`; rename the index to `schema.FTS5Table(table)`; `FullText = FullText.ForDialect(SQLite)`; append the three triggers (skipping any trigger already present with the same name).

- [ ] Step 1. Write the failing unit test `migrate/sqlite/fulltext_test.go`. Expect FAIL with "d.ExpandSchema undefined".

```go
package sqlite

import (
	"strings"
	"testing"

	"errors"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func ftsArticles(pk []string, pkType string) *schema.Schema {
	return &schema.Schema{Dialect: "sqlite", Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: pkType},
			{Name: "title", Type: "TEXT", Nullable: true},
			{Name: "body", Type: "TEXT", Nullable: true},
		},
		PrimaryKey: pk,
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Tokenizer: "porter unicode61"}}},
	}}}
}

// Catches: the update trigger issuing insert before delete (index corruption),
// a missing rebuild after creation, and the tokenizer dropped from the DDL.
func TestFullTextSQLiteRender(t *testing.T) {
	var d Dialect
	to, err := d.ExpandSchema(ftsArticles([]string{"id"}, "INTEGER"))
	if err != nil {
		t.Fatal(err)
	}
	changes, _, err := migrate.Diff(&schema.Schema{Dialect: "sqlite"}, to, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.Order(changes)
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := d.RenderSchema(&schema.Schema{Dialect: "sqlite"}, to, ordered)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, s := range stmts {
		sql = append(sql, s.SQL)
	}
	all := strings.Join(sql, "\n")
	vt := strings.Index(all, `CREATE VIRTUAL TABLE "articles_fts" USING fts5("title", "body", content='articles', content_rowid='id', tokenize='porter unicode61')`)
	rb := strings.Index(all, `INSERT INTO "articles_fts"("articles_fts") VALUES('rebuild')`)
	tr := strings.Index(all, `CREATE TRIGGER "articles_fts_ai"`)
	if vt < 0 || rb < vt || tr < 0 {
		t.Fatalf("create order wrong (vt=%d rebuild=%d trigger=%d):\n%s", vt, rb, tr, all)
	}
	au := all[strings.Index(all, `CREATE TRIGGER "articles_fts_au"`):]
	if strings.Index(au, `VALUES ('delete'`) > strings.Index(au, `VALUES (new."id"`) {
		t.Fatalf("update trigger inserts before it deletes:\n%s", au)
	}
}

// Catches: a composite or non-integer key reaching SQL, and a diagnostic that
// does not name the table.
func TestFullTextSQLiteNeedsIntegerKey(t *testing.T) {
	var d Dialect
	for name, s := range map[string]*schema.Schema{
		"composite": ftsArticles([]string{"id", "title"}, "INTEGER"),
		"text key":  ftsArticles([]string{"id"}, "TEXT"),
	} {
		_, err := d.ExpandSchema(s)
		if err == nil || !strings.Contains(err.Error(), "articles") || !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation naming articles", name, err)
		}
	}
}
```

- [ ] Step 2. Implement `migrate/sqlite/fulltext.go`, then the render paths. `renderChange` for `CreateIndex` with `After.FullText != nil` returns the virtual-table statement plus the rebuild statement; `DropIndex` with `Before.FullText != nil` returns `DROP TABLE "articles_fts"`. The hint `requires:fts5` rides on the virtual-table statement.
- [ ] Step 3. Order. Write the failing test `TestFullTextSQLiteDrop` in the same file: expect FAIL with "drop table before triggers".

```go
// Catches: the virtual table dropped while its triggers still point at it, and
// a shadow object left behind after the declaration is removed.
func TestFullTextSQLiteDrop(t *testing.T) {
	var d Dialect
	from, _ := d.ExpandSchema(ftsArticles([]string{"id"}, "INTEGER"))
	to := ftsArticles([]string{"id"}, "INTEGER")
	to.Tables[0].Indexes = nil
	changes, _, err := migrate.Diff(from, to, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.Order(changes)
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := d.RenderSchema(from, to, ordered)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, s := range stmts {
		sql = append(sql, s.SQL)
	}
	all := strings.Join(sql, "\n")
	lastTrigger := strings.LastIndex(all, `DROP TRIGGER "articles_fts_`)
	dropTable := strings.Index(all, `DROP TABLE "articles_fts"`)
	if lastTrigger < 0 || dropTable < lastTrigger {
		t.Fatalf("triggers must drop before the virtual table:\n%s", all)
	}
}
```

In `migrate/order.go` add the rule: a `DropTrigger` whose table has a `DropIndex` of a FullText index in the same plan sorts before that index, and a `CreateTrigger` sorts after the `CreateIndex` of a FullText index of the same table. Read the existing key function for the trigger and index groups and extend it; do not add a second pass.

- [ ] Step 4. Map the missing module. In `execStatement` (`migrate/runner.go`), when `s.Hint == "requires:fts5"` and the error text contains `no such module: fts5`, return `&momo.Error{Kind: momo.ErrUnsupported, Op: "migrate", Err: errors.New("SQLite was built without the FTS5 module; full-text search needs FTS5")}`. Add the unit test `TestExecStatementFTS5Missing` in `migrate/runner_test.go` with a fake `execer` returning `errors.New("no such module: fts5")`; expect FAIL with "kind = nil". The test asserts `errors.Is(err, momo.ErrUnsupported)` and that the message contains `FTS5` and not `no such module`.
- [ ] Step 5. Write the integration tests `migrate/sqlite/fulltext_integration_test.go` (`//go:build integration && cgo && !windows`, package `sqlite_test`). They run on real SQLite through `testopen.SQLite`.

```go
// Catches: a second diff after apply that is not empty, a stale or missing row
// after an insert, update or delete, and an integrity-check failure after an
// update followed by a delete.
func TestFullTextSQLiteMigrationRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	want := ftsDeclared() // the schema of ftsArticles in package sqlite_test, with INTEGER primary key
	eng := newEngine(t, db)
	applyDiff(ctx, t, eng, want, "fts")
	if m, err := eng.Diff(ctx, want, "again", migrate.DiffOptions{}); err != nil || m != nil {
		t.Fatalf("second diff = %v, %v; want none", m, err)
	}
	search := func(q string) []int64 {
		rows, err := db.Query(ctx, `SELECT rowid FROM articles_fts WHERE articles_fts MATCH ? ORDER BY rowid`, q)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		return ids
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO articles (id, title, body) VALUES (1, 'red fox', 'quick'), (2, 'blue whale', 'slow')`)
	if got := search(`"fox"`); len(got) != 1 || got[0] != 1 {
		t.Fatalf("after insert: %v", got)
	}
	exec(`UPDATE articles SET title = 'green frog' WHERE id = 1`)
	if got := search(`"fox"`); len(got) != 0 {
		t.Fatalf("after update the old token is still indexed: %v", got)
	}
	if got := search(`"frog"`); len(got) != 1 {
		t.Fatalf("after update the new token is missing: %v", got)
	}
	exec(`DELETE FROM articles WHERE id = 2`)
	if got := search(`"whale"`); len(got) != 0 {
		t.Fatalf("after delete: %v", got)
	}
	exec(`INSERT INTO articles_fts(articles_fts) VALUES('integrity-check')`)
}
```

Add `TestFullTextSQLiteSync` (the insert, update, delete assertions above as separate subtests), `TestFullTextSQLiteIntegrityAfterUpdateDelete` (update then delete then `integrity-check`), and `TestFullTextSQLiteBaseTableRebuild` (add a `NOT NULL DEFAULT` column to `articles` so the engine rebuilds the table, then search still returns the row). `TestFullTextSQLiteDriverSupport` runs the probe `CREATE VIRTUAL TABLE p USING fts5(a); SELECT bm25(p) FROM p WHERE p MATCH 'x'` once through `driver/sqlitedriver` and once through `driver/sqlitepure` (in `driver/sqlitepure/fulltext_integration_test.go`), skipping with `t.Skip("driver without FTS5: <reason>")` only when the error is the mapped `ErrUnsupported`, and failing when the message contains `no such module`.

- [ ] Step 6. Verify on CI: `unit`, `lint`, `integration` (SQLite runs in the same job as PostgreSQL), plus `macos` and `windows` for the build tags. Poll at most every 10 minutes.
- [ ] Step 7. Commit in a separate call: `feat(migrate): FTS5 table and sync triggers on SQLite`.

## Task 4: MySQL and MariaDB FULLTEXT

Covers: S-7 (all three criteria: `TestFullTextMySQLMigrationRoundTrip`, `TestFullTextMySQLRenderValidation`; the match builder criterion is Task 7).

Files:

- Modify `schema/schema.go`: `Table.Engine string` (`json:"engine,omitempty"`).
- Modify `migrate/mysql/render.go`: `createIndex` for `FullText`, `WITH PARSER`.
- Modify `migrate/mysql/validate.go`: engine, column type and unique checks.
- Modify `migrate/mysql/inspect.go`: read the engine of a table that is not InnoDB (used by Task 5 as well).
- Create `migrate/mysql/fulltext.go`: `ExpandSchema` (applies `ForDialect`), the type check.
- Create `migrate/mysql/fulltext_test.go` and `migrate/mysql/fulltext_integration_test.go`.

Interfaces:

```go
func (Dialect) ExpandSchema(s *schema.Schema) (*schema.Schema, error) // FullText = FullText.ForDialect(MySQL)
func (Dialect) ValidateSchema(s *schema.Schema) error                  // existing; gains the full-text checks
```

Rules: every column of the FullText index must be CHAR, VARCHAR or TEXT family (`char`, `varchar`, `tinytext`, `text`, `mediumtext`, `longtext`), else `ErrValidation` naming table and column; `Unique` on a FULLTEXT index is `ErrValidation` (already in `createIndex`, now also in validation so the diff fails before rendering); a table with `Engine != "" && !strings.EqualFold(Engine, "InnoDB")` is `ErrValidation`; `Parser` must be `ngram` or `mecab`, and on MariaDB (`dialect.FlavorMariaDB`) any `Parser` is `ErrUnsupported` ("MariaDB InnoDB has no full-text parser plugin"); more than 16 columns is `ErrValidation`. Render: a statement of the form CREATE FULLTEXT INDEX name ON table (columns) WITH PARSER ngram, as the unit test below pins it; the parser name is from the allowlist, never quoted user text. Because `Render(plan)` receives no flavor, `ExpandSchema` is flavor-agnostic and the MariaDB parser refusal lives in the `Dialect` value created for MariaDB (`mysql.New(dialect.FlavorMariaDB)` or the existing constructor; read `migrate/mysql/dialect.go` for how the flavor reaches `Render`).

- [ ] Step 1. Write the failing unit test `migrate/mysql/fulltext_test.go`. Expect FAIL with "undefined: schema.Table.Engine" (field `Engine` missing) and, once it exists, `ValidateSchema` returning nil.

```go
package mysql

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func ftTable(colType, engine string, unique bool, parser string) *schema.Schema {
	return &schema.Schema{Dialect: "mysql", Tables: []schema.Table{{
		Name: "articles", Engine: engine,
		Columns: []schema.Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: colType}, {Name: "body", Type: "text"}},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext", Unique: unique,
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Parser: parser}}},
	}}}
}

// Catches: the parser dropped from the DDL and the column order changed.
func TestFullTextMySQLRender(t *testing.T) {
	d := Dialect{}
	to := ftTable("varchar(255)", "", false, "ngram")
	changes, _, err := migrate.Diff(&schema.Schema{Dialect: "mysql"}, to, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := d.Render(changes)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, s := range stmts {
		sql = append(sql, s.SQL)
	}
	if !strings.Contains(strings.Join(sql, "\n"), "CREATE FULLTEXT INDEX `articles_search` ON `articles` (`title`, `body`) WITH PARSER ngram") {
		t.Fatalf("rendered:\n%s", strings.Join(sql, "\n"))
	}
}

// Catches: a non-text column, a unique FULLTEXT and a non-InnoDB table
// reaching the server.
func TestFullTextMySQLRenderValidation(t *testing.T) {
	d := Dialect{}
	cases := map[string]*schema.Schema{
		"non-text column": ftTable("bigint", "", false, ""),
		"unique":          ftTable("text", "", true, ""),
		"myisam":          ftTable("text", "MyISAM", false, ""),
		"bad parser":      ftTable("text", "", false, "ngram; DROP TABLE x"),
	}
	for name, s := range cases {
		err := d.ValidateSchema(s)
		if err == nil || !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
	if err := d.ValidateSchema(ftTable("varchar(100)", "InnoDB", false, "ngram")); err != nil {
		t.Errorf("valid declaration refused: %v", err)
	}
}
```

- [ ] Step 2. Implement: the `Engine` field; the validation (wrap every error in `&momo.Error{Kind: momo.ErrValidation, Op: "migrate/mysql", ...}`, which `ValidateSchema` already joins with `errors.Join`); the `createIndex` branch appends ` WITH PARSER <parser>` after the part list when `ix.FullText != nil && ix.FullText.Parser != ""`.
- [ ] Step 3. Write the integration test `migrate/mysql/fulltext_integration_test.go` (`//go:build integration`, `testopen.EachMySQL`). Expect FAIL with "Index differs" until Step 2 and Task 5 inspection land; the test is pushed with Task 5 for the second-diff half and with this task for the apply half (apply only, then `SHOW INDEX`).

```go
// Catches: the index missing on the server after apply, and the parser lost on
// MySQL. MariaDB runs without a parser (no ngram plugin).
func TestFullTextMySQLMigrationRoundTrip(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		parser := ""
		if db.Dialect().Flavor == dialect.FlavorMySQL {
			parser = "ngram"
		}
		want := ftTable("varchar(255)", "", false, parser)
		eng := newMySQLEngine(t, db)
		applyDiff(ctx, t, eng, want, "fulltext")
		var typ string
		row := db.QueryRow(ctx, `SELECT INDEX_TYPE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'articles' AND INDEX_NAME = 'articles_search' LIMIT 1`)
		if err := row.Scan(&typ); err != nil || typ != "FULLTEXT" {
			t.Fatalf("index type = %q (%v), want FULLTEXT", typ, err)
		}
		if parser == "ngram" {
			var ddl, name string
			if err := db.QueryRow(ctx, "SHOW CREATE TABLE articles").Scan(&name, &ddl); err != nil || !strings.Contains(ddl, "WITH PARSER `ngram`") {
				t.Fatalf("parser missing from SHOW CREATE TABLE: %v\n%s", err, ddl)
			}
		}
	})
}
```

Add `TestFullTextMariaDBParserRefused`: on `mariadb118` a declaration with `Parser: "ngram"` returns an error wrapping `momo.ErrUnsupported` from `Diff` and sends no statement (G2).

- [ ] Step 4. Verify on CI: `unit`, `lint`, `integration-mysql` for `mysql84`, `mysql97` and `mariadb118`. Poll at most every 10 minutes. The MariaDB refusal of a declared parser is the decided behaviour (G2).
- [ ] Step 5. Commit in a separate call: `feat(migrate): FULLTEXT declaration on MySQL and MariaDB`.

## Task 5: Inspection and `--from-db`

Covers: S-8 (all three criteria: `TestFullTextInspectRoundTrip`, `TestSQLiteInspectIgnoresShadowTables`, `TestGenerateFromDBFullText`) and the Failure-modes drift line (`TestFullTextSQLiteDriftReportsTrigger`).

Files:

- Modify `migrate/postgres/inspect_index.go` and `migrate/postgres/inspect.go`: fold a GIN index over a generated tsvector column, and a GIN expression index over `to_tsvector`, into `FullText`.
- Create `migrate/postgres/fulltext_inspect.go`: `parseGenerationExpr(expr string) (schema.FullText, bool)`.
- Modify `migrate/sqlite/inspect.go`: read FTS5 virtual tables into an index on the content table.
- Create `migrate/sqlite/fulltext_inspect.go`: `parseFTS5(sql string) (table string, ft schema.FullText, ok bool)`.
- Modify `migrate/mysql/inspect.go`: `Method: "fulltext"` plus `FullText{Columns, Parser}`, and `Table.Engine` for a non-InnoDB table.
- Modify `gen/fromdb/emit.go`: emit `c.FullText(...)` (or the `search` tag when defaults suffice).
- Create `migrate/*/fulltext_inspect_test.go` per dialect, `gen/fromdb/fulltext_test.go`.

Interfaces:

```go
// package postgres
func parseGenerationExpr(expr string) (schema.FullText, bool)
// package sqlite
func parseFTS5(sql string) (content string, ft schema.FullText, ok bool)
```

Rules: PostgreSQL. `pg_get_expr` returns `setweight(to_tsvector('english'::regconfig, COALESCE(title, ''::text)), 'A'::"char") || ...`; `parseGenerationExpr` accepts exactly the shapes `generationExpr` produces after that rewriting (one or more `to_tsvector('<cfg>'::regconfig, COALESCE(<col>, ''::text))` terms, each optionally wrapped in `setweight(..., '<W>'::"char")`, joined by `||`) and returns the columns, the one shared configuration and the weights; any other expression returns `ok=false`, so a trigger-maintained column and a hand-written generated column stay plain columns and a `Method("gin")` index stays `Method: "gin"`. A GIN index whose single part is such a generated column becomes `Index{Name, Method: "fulltext", Parts: [column], FullText: ft}` and keeps the column in `Columns`. A GIN index with a single `to_tsvector('cfg'::regconfig, COALESCE(a, ''::text) || ' '::text || COALESCE(b, ''::text))` expression part becomes `FullText{Columns: [a, b], Config: cfg, Expression: true}`. SQLite. `parseFTS5` reads `CREATE VIRTUAL TABLE "articles_fts" USING fts5("title", "body", content='articles', content_rowid='id', tokenize='porter unicode61')`, returns columns, the content table and the tokenizer; a virtual table that is not `fts5`, or an FTS5 table without `content=`, stays skipped. Shadow tables (`_data`, `_idx`, `_content`, `_docsize`, `_config`) stay in `skippedTables`. MySQL. `INDEX_TYPE = 'FULLTEXT'` rows become `Method: "fulltext"` (already) plus `FullText{Columns: <parts in order>, Parser: <from SHOW CREATE TABLE>}`; the parser is read from the `/*!50100 WITH PARSER `ngram` */` comment for that index.

- [ ] Step 1. Write the failing unit tests. SQLite parse first. Expect FAIL with "undefined: parseFTS5".

```go
package sqlite

import (
	"reflect"
	"testing"
)

// Catches: a non-FTS5 virtual table claimed as a full-text table, a lost
// tokenizer, and a changed column order.
func TestParseFTS5(t *testing.T) {
	content, ft, ok := parseFTS5(`CREATE VIRTUAL TABLE "articles_fts" USING fts5("title", "body", content='articles', content_rowid='id', tokenize='porter unicode61')`)
	if !ok || content != "articles" || !reflect.DeepEqual(ft.Columns, []string{"title", "body"}) || ft.Tokenizer != "porter unicode61" {
		t.Fatalf("got %q %+v %v", content, ft, ok)
	}
	for _, sql := range []string{
		`CREATE VIRTUAL TABLE r USING rtree(id, minx, maxx)`,
		`CREATE VIRTUAL TABLE f USING fts5(a, b)`,
		`CREATE TABLE t (a)`,
	} {
		if _, _, ok := parseFTS5(sql); ok {
			t.Errorf("accepted %s", sql)
		}
	}
}

// Catches: a shadow table or a non-FTS5 virtual table reported as a table.
func TestSQLiteInspectIgnoresShadowTables(t *testing.T) {
	db := openMemory(t) // the helper the existing virtual_test.go uses
	for _, q := range []string{
		`CREATE TABLE articles (id INTEGER PRIMARY KEY, title TEXT, body TEXT)`,
		`CREATE VIRTUAL TABLE articles_fts USING fts5(title, body, content='articles', content_rowid='id')`,
		`CREATE VIRTUAL TABLE geo USING rtree(id, minx, maxx, miny, maxy)`,
	} {
		mustExec(t, db, q)
	}
	s, err := New().Inspect(context.Background(), db, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 1 || s.Tables[0].Name != "articles" {
		t.Fatalf("tables = %+v, want only articles", s.Tables)
	}
	ix, ok := s.Tables[0].FullTextIndex()
	if !ok || ix.Name != "articles_fts" || !reflect.DeepEqual(ix.FullText.Columns, []string{"title", "body"}) {
		t.Fatalf("full-text index = %+v %v", ix, ok)
	}
}
```

`openMemory`, `mustExec` are the helper names used in `migrate/sqlite/virtual_test.go`; open that file and use its actual helpers.

PostgreSQL parse unit test (expect FAIL with "undefined: parseGenerationExpr"):

```go
func TestParseGenerationExpr(t *testing.T) {
	ft, ok := parseGenerationExpr(`(setweight(to_tsvector('english'::regconfig, COALESCE(title, ''::text)), 'A'::"char") || setweight(to_tsvector('english'::regconfig, COALESCE(body, ''::text)), 'B'::"char"))`)
	if !ok || ft.Config != "english" || !reflect.DeepEqual(ft.Columns, []string{"title", "body"}) || !reflect.DeepEqual(ft.Weights, []string{"A", "B"}) {
		t.Fatalf("got %+v %v", ft, ok)
	}
	if _, ok := parseGenerationExpr(`lower(title)`); ok {
		t.Fatal("an unrelated generation expression was claimed")
	}
}
```

- [ ] Step 2. Implement the three inspectors and the `FullText` fields. The PostgreSQL `Dialect.Inspect` runs the fold after indexes and generated columns are read (both are available in `inspect_index.go`'s caller); a hand-written trigger recipe (`TestTSVectorMigrationRoundTrip`) stays unfolded.
- [ ] Step 3. Write the failing integration test `TestFullTextInspectRoundTrip` per dialect (`testopen.EachWithMySQL`, build tag `integration`): declare the structure with the dialect's `ExpandSchema`, apply it through the engine, then `Inspect` and `schema.Equal(expand(declared), inspected)` after `FullText.ForDialect`. Expect FAIL with "inspection misses the full-text structure" before Step 2. Add `TestFullTextSQLiteDriftReportsTrigger`: apply, `DROP TRIGGER articles_fts_au`, `eng.Drift(ctx)` and assert the report is not `Clean()` and names the trigger; and `TestFullTextPostgresExpressionInspect` for the expression form.
- [ ] Step 4. Write the failing generator test `gen/fromdb/fulltext_test.go`. Expect FAIL with "no c.FullText call emitted".

```go
// Catches: the declaration not emitted by --from-db, the PostgreSQL generated
// column emitted as a model field next to the declaration, and models that do
// not diff clean against the database.
func TestGenerateFromDBFullText(t *testing.T) {
	s := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "title", Type: "text"}, {Name: "body", Type: "text"},
			{Name: "articles_search_tsv", Type: "tsvector", Generated: "x"},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "articles_search_tsv"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Config: "simple"}}},
	}}}
	files, err := Generate(s, Options{Package: "models"})
	if err != nil {
		t.Fatal(err)
	}
	src := string(files["articles.go"])
	if !strings.Contains(src, `c.FullText("articles_search", &a.Title, &a.Body).Config("simple")`) {
		t.Fatalf("no c.FullText call:\n%s", src)
	}
	if strings.Contains(src, "ArticlesSearchTsv") {
		t.Fatalf("the generated column was emitted as a field:\n%s", src)
	}
}
```

Use the `Generate` entry point and file map that `gen/fromdb/fromdb_test.go` already uses (read its first test and mirror the call). Add `TestGenerateFromDBFullTextSearchTag`: with `Config` `english` default... the tag form is emitted only when the declaration is `FullText{Columns, Config: ""}` on a non-PostgreSQL dialect and the index is named `<table>_search`; otherwise the call form. The test uses a MySQL schema and expects `momo:"search"` on both fields.

- [ ] Step 5. Implement the emitter in `gen/fromdb/emit.go` next to `index()`: for an index with `Method == "fulltext" && FullText != nil` write `c.FullText(name, &a.F1, ...)` plus `.Config(..)`, `.Weights(..)`, `.Tokenizer(..)`, `.Parser(..)`, `.Expression()` for the set options, and skip the PostgreSQL column named `schema.TSVectorColumn(ix.Name)` when the index is a stored-form FullText. The `fulltext`/`spatial` skip in `gen/schema.go` (the index-column check) stays.
- [ ] Step 6. Verify on CI: `unit`, `lint`, `integration`, `integration-mysql`. Poll at most every 10 minutes.
- [ ] Step 7. Commit in a separate call: `feat(migrate): inspect full-text structures and emit them from the database`.

## Task 6: Portable query layer (PostgreSQL and SQLite) and the shared parser

Covers: S-1 (golden; wrong dialect), S-2 (score ordering, not normalised), S-3 (all three), S-9 (phrase and prefix, exclusion, snippet on PostgreSQL and SQLite), S-11 (hook masking, existing tests unchanged) for PostgreSQL and SQLite. MySQL rendering of the same functions is Task 7.

Files:

- Create `internal/ftsquery/parse.go`, `internal/ftsquery/render.go`, `internal/ftsquery/ftsquery_test.go`, `internal/ftsquery/fuzz_test.go`.
- Create `fts/query.go` (`Query`, `Web`, `Plain`, `Raw`, `Config`), `fts/target.go` (`Target`, `Over`, `OverVector`), `fts/portable.go` (`Search`, `Score`, `Snippet`, `SnippetOpts`), `fts/dialect.go` (`FTS5Match`, `BM25`).
- Create `fts/portable_test.go`, `fts/surface_test.go`, `fts/portable_integration_test.go`.
- Modify `driver/sqlitedriver/classify.go`, `driver/sqlitepure/classify.go`: `no such table: <x>_fts` becomes `ErrUnsupported` naming the table.

Interfaces:

```go
// package ftsquery
type Syntax uint8
const (Web Syntax = iota; Plain; Raw)
type Term struct {
	Text   string
	Phrase bool // quoted: Text holds several words
	Prefix bool // trailing *
}
type Parsed struct {
	Groups   [][]Term // OR-separated groups, each a conjunction
	Excluded []Term
}
func Parse(text string, syn Syntax) Parsed
func (p Parsed) Empty() bool                 // no positive term: the search is false, the score 0
func (p Parsed) HasPrefix() bool
func (p Parsed) PostgresTSQuery() string     // to_tsquery text, every lexeme quoted
func (p Parsed) FTS5() string                // "a" "b", ("a" "b") OR ("c") NOT "d"
func (p Parsed) MySQLBoolean() string        // Task 7

// package fts
type Query struct{ /* text, syntax, config */ }
func Web(text string) Query
func Plain(text string) Query
func Raw(text string) Query
func (q Query) Config(name string) Query

type Target struct{ /* table, columns, declaration, kind */ }
func Over[T any, PT momo.Modeler[T]](cols ...momo.Column[string]) Target
func OverVector(col momo.Column[types.TSVector], config string) Target

func Search(t Target, q Query) momo.Predicate
func Score(t Target, q Query) Expr[float64]
type SnippetOpts struct {
	Column   momo.Column[string]
	Open     string // default "<b>"
	Close    string // default "</b>"
	Ellipsis string // SQLite only, default "..."
	Tokens   int    // SQLite only, 1..64, default 16
}
func Snippet(t Target, q Query, o SnippetOpts) Expr[string]

func FTS5Match(table string, q string) momo.Predicate  // "table" MATCH ?
func BM25(table string, weights ...float64) Expr[float64] // bm25("table", w...) , lower is better
```

Parse rules (Web): tokens are separated by Unicode space; a token starting with `"` opens a phrase that runs to the next `"` (an unclosed quote runs to the end); `-word` (leading hyphen directly followed by a letter or digit) is an exclusion; the bare token `OR` (case-sensitive, between two terms) starts a new group; a trailing `*` on a word is a prefix; every other character that is not a letter, digit, `_` or `'` is dropped from words and from phrases; a term left with no letter or digit is dropped. Plain: every word a term in one group, no phrases, no exclusions, no prefix. Raw: `Parse` is not used; the text passes through unchanged.

Renderings. PostgreSQL: `Web` without a prefix term binds the raw text to `websearch_to_tsquery(CAST(<cfg> AS regconfig), $n)`; with a prefix, `PostgresTSQuery()` builds `'a' & 'b':* | 'c' & ! 'd'`-style text with each lexeme quoted by doubling `'` and `\`, phrases as `'d' <-> 'e'`, and binds it to `to_tsquery`. `Plain` uses `plainto_tsquery`. `Raw` uses `to_tsquery`. SQLite: terms are FTS5 strings `"x"` with `"` doubled, prefix `"x"*`, a group of several terms `("a" "b")`, groups joined by the keyword OR, exclusions appended as ` NOT "x"` after the positive expression; a query with no positive term is `Empty()`. Raw passes `MATCH ?` the text as given.

PostgreSQL render: declared stored target: `"articles"."articles_search_tsv" @@ websearch_to_tsquery(CAST('english' AS regconfig), $1)`; declared expression target: `to_tsvector(CAST('english' AS regconfig), COALESCE("articles"."title", '') || ' ' || COALESCE("articles"."body", '')) @@ ...`; `OverVector`: the existing bound-parameter form. SQLite render: `"articles"."id" IN (SELECT rowid FROM "articles_fts" WHERE "articles_fts" MATCH ?)`; `Score`: `(SELECT -bm25("articles_fts", 1, 0.4) FROM "articles_fts" WHERE "articles_fts" MATCH ? AND "articles_fts".rowid = "articles"."id")` (weights `A=1, B=0.4, C=0.2, D=0.1` from the declaration, none when undeclared); `Snippet`: `(SELECT snippet("articles_fts", <index of Column>, ?, ?, ?, <tokens>) FROM "articles_fts" WHERE "articles_fts" MATCH ? AND "articles_fts".rowid = "articles"."id")`. PostgreSQL `Score` is `ts_rank_cd(<vector>, <tsquery>)`, `Snippet` is `ts_headline(CAST('english' AS regconfig), "articles"."title", <tsquery>, $n)` with options `StartSel=<Open>, StopSel=<Close>` bound.

Empty or whitespace-only text, and `Parsed.Empty()`: `Search` writes `1 = 0` and `Score` writes `0`, with no MATCH and no argument bound. An override `Query.Config(name)` that differs from the declared configuration on a declared target is `ErrValidation` ("search config %q differs from the index config %q"). A target whose columns are not exactly the columns of the model's declaration (`schema.Table.FullTextIndex`), or a model without one, is `ErrValidation` naming the columns. Every builder calls `sensFlags` as the existing builders do so bound arguments are masked.

- [ ] Step 1. Write the failing parser test `internal/ftsquery/ftsquery_test.go`. Expect FAIL with "undefined: Parse".

```go
package ftsquery

import (
	"reflect"
	"testing"
)

// Catches: operator characters of any dialect surviving into a term, the
// exclusion attached to the wrong term, and the OR split lost.
func TestParseAndRender(t *testing.T) {
	cases := []struct {
		in           string
		fts5, pgText string
		empty        bool
	}{
		{in: `red fox`, fts5: `"red" "fox"`},
		{in: `"red fox" -hen`, fts5: `"red fox" NOT "hen"`},
		{in: `fox OR hound`, fts5: `("fox") OR ("hound")`},
		{in: `a b OR c`, fts5: `("a" "b") OR ("c")`},
		{in: `fo*`, fts5: `"fo"*`, pgText: `'fo':*`},
		{in: `NEAR( (`, empty: true},
		{in: `-spam`, empty: true},
		{in: `   `, empty: true},
		{in: `a"b`, fts5: `"a" "b"`},
		{in: `col:val ^x`, fts5: `"colval" "x"`},
	}
	for _, c := range cases {
		p := Parse(c.in, Web)
		if p.Empty() != c.empty {
			t.Errorf("%q: Empty() = %v", c.in, p.Empty())
			continue
		}
		if c.empty {
			continue
		}
		if got := p.FTS5(); got != c.fts5 {
			t.Errorf("%q: FTS5() = %s, want %s", c.in, got, c.fts5)
		}
		if c.pgText != "" && p.PostgresTSQuery() != c.pgText {
			t.Errorf("%q: PostgresTSQuery() = %s, want %s", c.in, p.PostgresTSQuery(), c.pgText)
		}
	}
	if got := Parse(`go orm`, Plain).Groups; !reflect.DeepEqual(got, [][]Term{{{Text: "go"}, {Text: "orm"}}}) {
		t.Errorf("plain groups = %+v", got)
	}
}
```

Fuzz test `internal/ftsquery/fuzz_test.go` (`FuzzFullTextWebQuery`): for any string, `Parse` must not panic, `FTS5()` must contain only characters inside double-quoted strings plus the fixed tokens ` `, `(`, `)`, `OR`, `NOT`, `*`; assert by stripping `"..."` spans (with `""` as an escaped quote) and checking the remainder matches `^[ ()*]*((OR|NOT)[ ()*]*)*$`; `PostgresTSQuery()` must have balanced single quotes.

- [ ] Step 2. Implement `internal/ftsquery`. No imports outside the standard library.
- [ ] Step 3. Write the failing golden test `fts/portable_test.go`. Expect FAIL with "undefined: fts.Web".

```go
package fts_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/fts"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/schema"
)

type article struct {
	ID          int64
	Title, Body string
}

var articleMeta = momo.Meta[article]{
	Table:   "articles",
	Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "title"}, {Name: "body"}},
	Ptr:     func(m *article, i int) any { return []any{&m.ID, &m.Title, &m.Body}[i] },
	Value:   func(m *article, i int) any { return []any{m.ID, m.Title, m.Body}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
	TableSchema: &schema.Table{
		Name:       "articles",
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts: []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"}}}},
	},
}

func (a *article) MomoMeta() *momo.Meta[article] { return &articleMeta }

var (
	artID    = momo.NewColumn[int64]("articles", "id", 0)
	artTitle = momo.NewColumn[string]("articles", "title", 0)
	artBody  = momo.NewColumn[string]("articles", "body", 0)
)

func compileFor(t *testing.T, d dialect.Name, e momo.Expr, p momo.Predicate) (momo.Compiled, error) {
	t.Helper()
	db := momo.New(fakedriver.New(d))
	q := momo.Select[article](db).From(momo.TableRef{Name: "articles"}).Columns(e)
	if p != nil {
		q = q.Where(p)
	}
	return q.SQL()
}

// Catches: user text in the SQL string, a changed placeholder order, the
// configuration not pinned as a literal, bm25 not negated, and a changed
// SQL shape on either dialect.
func TestFullTextSearchGolden(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	q := fts.Web(`red fox`)
	cases := []struct {
		name string
		d    dialect.Name
		e    momo.Expr
		p    momo.Predicate
		want string
		args []any
	}{
		{"pg search", dialect.Postgres, momo.CountAll(), fts.Search(target, q),
			`SELECT COUNT(*) FROM "articles" WHERE "articles"."articles_search_tsv" @@ websearch_to_tsquery(CAST('english' AS regconfig), $1)`,
			[]any{"red fox"}},
		{"pg score", dialect.Postgres, fts.Score(target, q).As("score"), nil,
			`SELECT ts_rank_cd("articles"."articles_search_tsv", websearch_to_tsquery(CAST('english' AS regconfig), $1)) AS "score" FROM "articles"`,
			[]any{"red fox"}},
		{"pg prefix", dialect.Postgres, momo.CountAll(), fts.Search(target, fts.Web(`red fo*`)),
			`WHERE "articles"."articles_search_tsv" @@ to_tsquery(CAST('english' AS regconfig), $1)`,
			[]any{`'red' & 'fo':*`}},
		{"sqlite search", dialect.SQLite, momo.CountAll(), fts.Search(target, q),
			`WHERE "articles"."id" IN (SELECT rowid FROM "articles_fts" WHERE "articles_fts" MATCH ?)`,
			[]any{`"red" "fox"`}},
		{"sqlite score", dialect.SQLite, fts.Score(target, q).As("score"), nil,
			`SELECT (SELECT -bm25("articles_fts", 1, 0.4) FROM "articles_fts" WHERE "articles_fts" MATCH ? AND "articles_fts".rowid = "articles"."id") AS "score" FROM "articles"`,
			[]any{`"red" "fox"`}},
	}
	for _, c := range cases {
		got, err := compileFor(t, c.d, c.e, c.p)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !strings.Contains(got.SQL, c.want) {
			t.Errorf("%s: SQL\n got %s\nwant it to contain %s", c.name, got.SQL, c.want)
		}
		if !reflect.DeepEqual(got.Args, c.args) {
			t.Errorf("%s: args %#v, want %#v", c.name, got.Args, c.args)
		}
	}
}

// Catches: user text interpolated into the SQL text on any dialect.
func TestFullTextQueryIsBound(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	const hostile = `x') OR 1=1 --`
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		got, err := compileFor(t, d, momo.CountAll(), fts.Search(target, fts.Web(hostile)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got.SQL, "OR 1=1") || strings.Contains(got.SQL, "x'") {
			t.Errorf("%s: user text reached the SQL: %s", d, got.SQL)
		}
	}
}

// Catches: a MATCH sent for an empty query, and a row returned for it.
func TestFullTextEmptyQuery(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		for _, text := range []string{"", "   ", "-spam", "NEAR( ("} {
			got, err := compileFor(t, d, momo.CountAll(), fts.Search(target, fts.Web(text)))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got.SQL, "MATCH") || strings.Contains(got.SQL, "@@") || !strings.Contains(got.SQL, "1 = 0") || len(got.Args) != 0 {
				t.Errorf("%s %q: %s %v", d, text, got.SQL, got.Args)
			}
			sc, err := compileFor(t, d, fts.Score(target, fts.Web(text)).As("s"), nil)
			if err != nil || !strings.Contains(sc.SQL, "SELECT 0 AS") {
				t.Errorf("%s %q: score = %v %v", d, text, sc.SQL, err)
			}
		}
	}
}

// Catches: a dialect-specific predicate building SQL on the wrong dialect.
func TestFullTextWrongDialectUnsupported(t *testing.T) {
	cases := []struct {
		d dialect.Name
		p momo.Predicate
	}{
		{dialect.Postgres, fts.FTS5Match("articles_fts", `"x"`)},
		{dialect.MySQL, fts.FTS5Match("articles_fts", `"x"`)},
		{dialect.SQLite, fts.Match(tsv, fts.WebsearchToQuery("english", "x"))},
		{dialect.MySQL, fts.Match(tsv, fts.WebsearchToQuery("english", "x"))},
	}
	for _, c := range cases {
		db := momo.New(fakedriver.New(c.d))
		_, err := momo.Select[article](db).From(momo.TableRef{Name: "articles"}).Where(c.p).SQL()
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", c.d, err)
		}
	}
}

// Catches: a config override that differs from the index config accepted, and
// a target that is not the declared column set accepted.
func TestFullTextTargetValidation(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	_, err := compileFor(t, dialect.Postgres, momo.CountAll(), fts.Search(target, fts.Web("x").Config("simple")))
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("config override: err = %v, want ErrValidation", err)
	}
	_, err = compileFor(t, dialect.SQLite, momo.CountAll(), fts.Search(fts.Over[article](artTitle), fts.Web("x")))
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("undeclared column set: err = %v, want ErrValidation", err)
	}
}
```

The test file imports `strings` and defines `tsv` as the `Column[types.TSVector]` the existing `fts_test.go` already declares; put the new tests in a file in the same package so `tsv` is shared.

- [ ] Step 4. Write the failing API-surface test `fts/surface_test.go` (`TestFullTextScoreNotNormalised`), expect FAIL with "undefined: fts.Score":

```go
// Catches: a portable function growing a numeric score threshold.
func TestFullTextScoreNotNormalised(t *testing.T) {
	for name, fn := range map[string]any{"Search": fts.Search, "Score": fts.Score, "Snippet": fts.Snippet} {
		typ := reflect.TypeOf(fn)
		for i := 0; i < typ.NumIn(); i++ {
			switch typ.In(i).Kind() {
			case reflect.Float32, reflect.Float64:
				t.Errorf("fts.%s takes a numeric parameter %v: scores are not comparable across dialects", name, typ.In(i))
			}
		}
	}
	doc, err := os.ReadFile("../docs/fulltext.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), "comparable only within one dialect and one query") {
		t.Error("docs/fulltext.md lost the statement that scores are comparable only within one dialect and one query")
	}
}
```

This test fails on the docs statement until Task 10; write the docs assertion into `fts/surface_docs_test.go` in Task 10 instead and keep only the reflection loop here.

- [ ] Step 5. Implement `fts/query.go`, `fts/target.go`, `fts/portable.go`, `fts/dialect.go`. `Over` calls `momo.Modeler[T].MomoMeta()` through a nil `PT` as `metaOf` does in the root package; because `metaOf` is unexported, add an exported `momo.MetaTableSchema[T any, PT Modeler[T]]() *schema.Table` to `meta.go`. The `Expr[float64]` and `Expr[string]` come from `newExpr` in `fts/fts.go`. Keep `available` for the PostgreSQL-only builders and add `availableAny(b, what)` accepting PostgreSQL, SQLite and MySQL. In `driver/sqlitedriver/classify.go` and `driver/sqlitepure/classify.go` add the `no such table: <name>` case when the name ends in `_fts`: kind `momo.ErrUnsupported`, the message naming the table.
- [ ] Step 6. Write the failing integration tests `fts/portable_integration_test.go` (`//go:build integration`, `testopen.Each`, so PostgreSQL and SQLite here; Task 7 switches the call to `EachWithMySQL`). Seed at least six rows in an `articles` table created through the engine from the declaration (so the structures under test are the migrated ones), then:

```go
// Catches: a dialect returning a row that misses a word, or omitting a row that
// has all of them (S-1), and the portable score ordering being inverted (S-2).
func TestFullTextSearchAllDialects(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedArticles(t, db, d) // six rows: ids 1..6, see below
		target := fts.Over[article](artTitle, artBody)
		ids := func(q fts.Query) []int64 {
			var out []article
			out, err := momo.Select[article](db).From(momo.TableRef{Name: "articles"}).
				Where(fts.Search(target, q)).OrderBy(artID.Asc()).All(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var r []int64
			for _, a := range out {
				r = append(r, a.ID)
			}
			return r
		}
		if got := ids(fts.Web(`red fox`)); !reflect.DeepEqual(got, []int64{1, 4}) {
			t.Fatalf("all words: %v, want [1 4]", got)
		}
		if got := ids(fts.Web(`"red fox" -hen`)); !reflect.DeepEqual(got, []int64{1}) {
			t.Fatalf("phrase and exclusion: %v, want [1]", got)
		}
		if got := ids(fts.Web(`fo*`)); !reflect.DeepEqual(got, []int64{1, 4, 5}) {
			t.Fatalf("prefix: %v, want [1 4 5]", got)
		}
	})
}
```

Seed (title / body): 1 `red fox` / `the red fox jumps`; 2 `blue whale` / `slow and large`; 3 `red hen` / `a red hen and a fox`; 4 `fox red` / `fast animal`; 5 `foxtrot` / `a dance`; 6 `green frog` / NULL. With these rows the expected sets are: `red fox` all words matches 1, 3 and 4 (row 3 has both words in its body); `"red fox" -hen` matches 1 only (the phrase is adjacent in row 1, row 4 has `fox red`, row 3 contains `hen`); `fo*` matches 1, 3, 4 and 5. The assertions in the sketch above must use these sets (`[]int64{1, 3, 4}`, `[]int64{1}`, `[]int64{1, 3, 4, 5}`); the sketch's `[1 4]` and `[1 4 5]` values are superseded by this paragraph. On MySQL the short word `fo*` prefix is shorter than `innodb_ft_min_token_size` (3), so the MySQL subtests assert zero rows for it, as the criterion requires. `TestFullTextScoreOrdering` seeds three rows with 1, 2 and 3 occurrences of `fox` and asserts `OrderBy(fts.Score(target, q).Desc())` returns the ids in 3, 2, 1 order. `TestFullTextWebQueryNeverErrors` runs `a OR b -c "d e" NEAR( (`, `"`, `((`, `-`, `*`, `\`, `'` and 200 random strings from a seeded `math/rand` through `fts.Web` on each dialect and fails on any error. `TestFullTextPhraseAndPrefix`, `TestFullTextExclusion` (negation-only returns zero rows and no error), `TestFullTextSnippet` (marks `<b>` present on PostgreSQL and SQLite), `TestFullTextNulls` (row 6 has a NULL body and is found by its title; a row with both columns NULL is never returned) and `TestFullTextHooksSeeMaskedArgsPortable` (extends `maskCase` from `fts_test.go` to the portable calls with a sensitive title column) each run per dialect with the same seed.

- [ ] Step 7. Verify on CI: `unit`, `lint`, `fuzz-smoke` (add the `FuzzFullTextWebQuery` target to the job's list in `.github/workflows/ci.yml`), `integration`, `compile-fail`. The existing `TestFullTextGolden` and `TestFullTextSearch` stay green unchanged. Poll at most every 10 minutes.
- [ ] Step 8. Commit in a separate call: `feat(fts): portable search, score and snippet on PostgreSQL and SQLite`.

## Task 7: MySQL and MariaDB boolean mode

Covers: S-1 (MySQL/MariaDB halves of the all-dialects, golden and wrong-dialect criteria), S-7 (`TestFullTextMySQLMatchGolden`, `TestFullTextMySQLMissingIndex`), S-9 (MySQL parts, snippet unsupported), S-11 (`TestFullTextTransactionVisibility`, `TestFullTextMySQLMinTokenSize`, MySQL halves of the NULL and hook tests).

Files:

- Modify `internal/ftsquery/render.go`: `Parsed.MySQLBoolean()`.
- Modify `internal/ftsquery/ftsquery_test.go`: MySQL cases.
- Modify `fts/portable.go`: the MySQL branch of `Search` and `Score`, `Snippet` unsupported.
- Create `fts/mysql.go`: `MySQLMode`, `MySQLMatch`, `MySQLRelevance`.
- Modify `driver/mysqldriver/errors.go`: server error 1191 becomes `ErrValidation`.
- Modify `fts/portable_test.go`, `fts/portable_integration_test.go` (switch `testopen.Each` to `testopen.EachWithMySQL` in the tests that must run on the MySQL family).

Interfaces:

```go
type MySQLMode uint8
const (
	NaturalLanguage MySQLMode = iota
	Boolean
	NaturalLanguageWithExpansion
)
func MySQLMatch(cols []momo.Column[string], q string, mode MySQLMode) momo.Predicate  // MATCH(c1, c2) AGAINST (? IN BOOLEAN MODE) > 0
func MySQLRelevance(cols []momo.Column[string], q string, mode MySQLMode) Expr[float64]
func (p Parsed) MySQLBoolean() string // ftsquery
```

MySQL rendering of the portable layer. `Web` text is rendered by `MySQLBoolean()`: each ungrouped word `+word`, prefix `+word*`, phrase `+"d e"`, exclusion `-word`; with an OR the groups are written `(+a +b) (+c)` and the exclusions follow at the top level. Operator characters never reach a word (the parser dropped them). `Plain` is `+a +b`. `Raw` passes the text through. Search renders `MATCH(`title`, `body`) AGAINST (? IN BOOLEAN MODE)`; Score renders the same MATCH expression (the relevance value); `Snippet` records `ErrUnsupported`. The mode text (`IN NATURAL LANGUAGE MODE`, `IN BOOLEAN MODE`, `IN NATURAL LANGUAGE MODE WITH QUERY EXPANSION`) is a fixed string chosen by the `MySQLMode`, never taken from input. A target whose columns differ from the declaration is `ErrValidation` naming the columns (the MySQL index must cover exactly those columns in that order). `MATCH` comparison for Search is written `MATCH(...) AGAINST (...)` bare in a WHERE (a non-zero relevance is true) so that `HAVING` and `ORDER BY` reuse is unambiguous.

- [ ] Step 1. Write the failing parser test cases in `internal/ftsquery/ftsquery_test.go`. Expect FAIL with "p.MySQLBoolean undefined".

```go
func TestMySQLBoolean(t *testing.T) {
	cases := map[string]string{
		`red fox`:          `+red +fox`,
		`"red fox" -hen`:   `+"red fox" -hen`,
		`fox OR hound`:     `(+fox) (+hound)`,
		`a b OR c -d`:      `(+a +b) (+c) -d`,
		`fo*`:              `+fo*`,
		`+x >y <z ~w (v) @u`: `+x +y +z +w +v +u`,
	}
	for in, want := range cases {
		if got := Parse(in, Web).MySQLBoolean(); got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
}
```

- [ ] Step 2. Write the failing golden test (in `fts/portable_test.go`), expect FAIL with "ErrUnsupported" for MySQL:

```go
func TestFullTextMySQLGolden(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	got, err := compileFor(t, dialect.MySQL, momo.CountAll(), fts.Search(target, fts.Web(`red fox -hen`)))
	if err != nil {
		t.Fatal(err)
	}
	want := "WHERE MATCH(`articles`.`title`, `articles`.`body`) AGAINST (? IN BOOLEAN MODE)"
	if !strings.Contains(got.SQL, want) || !reflect.DeepEqual(got.Args, []any{`+red +fox -hen`}) {
		t.Fatalf("SQL %s args %v", got.SQL, got.Args)
	}
	sn := fts.Snippet(target, fts.Web("x"), fts.SnippetOpts{Column: artTitle})
	if _, err := compileFor(t, dialect.MySQL, sn.As("s"), nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("snippet on MySQL: err = %v, want ErrUnsupported", err)
	}
}

// Catches: the mode text or the bound query changing, and the mode taken from
// input.
func TestFullTextMySQLMatchGolden(t *testing.T) {
	cols := []momo.Column[string]{artTitle, artBody}
	for mode, want := range map[fts.MySQLMode]string{
		fts.NaturalLanguage:              "MATCH(`articles`.`title`, `articles`.`body`) AGAINST (? IN NATURAL LANGUAGE MODE)",
		fts.Boolean:                      "MATCH(`articles`.`title`, `articles`.`body`) AGAINST (? IN BOOLEAN MODE)",
		fts.NaturalLanguageWithExpansion: "MATCH(`articles`.`title`, `articles`.`body`) AGAINST (? IN NATURAL LANGUAGE MODE WITH QUERY EXPANSION)",
	} {
		got, err := compileFor(t, dialect.MySQL, momo.CountAll(), fts.MySQLMatch(cols, "+go -orm", mode))
		if err != nil || !strings.Contains(got.SQL, want) || !reflect.DeepEqual(got.Args, []any{"+go -orm"}) {
			t.Errorf("mode %d: %v\n%s\n%v", mode, err, got.SQL, got.Args)
		}
	}
}
```

- [ ] Step 3. Implement `MySQLBoolean`, the MySQL branch, `fts/mysql.go`, and the driver mapping. In `driver/mysqldriver/errors.go` add `1191: momo.ErrValidation, // ER_FT_MATCHING_KEY_NOT_FOUND` to `kinds`; add `TestClassify1191` in `driver/mysqldriver/errors_test.go` constructing `&mysql.MySQLError{Number: 1191}` and asserting `errors.Is(classify(err, ""), momo.ErrValidation)` (expect FAIL with "not ErrValidation" before the map entry).
- [ ] Step 4. Write the failing MySQL integration tests (switch the Task 6 integration tests to `testopen.EachWithMySQL` and add):

```go
// Catches: visibility differing from the documented table: MySQL and MariaDB
// do not see an uncommitted row, PostgreSQL and SQLite do, rollback removes it
// everywhere.
func TestFullTextTransactionVisibility(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedArticles(t, db, d)
		target := fts.Over[article](artTitle, artBody)
		count := func(q momo.Querier) int {
			n, err := momo.Select[article](q).From(momo.TableRef{Name: "articles"}).
				Where(fts.Search(target, fts.Web("zebra"))).Count(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			return int(n)
		}
		visibleInTx := d != dialect.MySQL
		err := db.Tx(context.Background(), func(tx *momo.Tx) error {
			if _, err := momo.Insert(tx, &article{ID: 100, Title: "zebra", Body: "stripes"}).Exec(context.Background()); err != nil {
				return err
			}
			if got := count(tx); (got == 1) != visibleInTx {
				t.Errorf("%s: inside the transaction count = %d, visible = %v", d, got, visibleInTx)
			}
			return errRollback
		})
		if !errors.Is(err, errRollback) || count(db) != 0 {
			t.Fatalf("%s: rollback left the row searchable (err %v)", d, err)
		}
		if _, err := momo.Insert(db, &article{ID: 101, Title: "zebra", Body: "stripes"}).Exec(context.Background()); err != nil {
			t.Fatal(err)
		}
		if count(db) != 1 {
			t.Fatalf("%s: committed row not found", d)
		}
	})
}
```

`d != dialect.MySQL` is true for PostgreSQL and SQLite, and for the MySQL family the test reads `db.Dialect().Flavor` is not needed because both MySQL and MariaDB run InnoDB FULLTEXT with the same visibility; `visibleInTx := d != dialect.MySQL` is evaluated on the dialect name, which is `dialect.MySQL` for MariaDB too. `TestFullTextMySQLMinTokenSize` asserts a two-character word (`go`) in row text returns zero rows through `fts.Search` on the MySQL family and skips (`t.Skip("innodb_ft_min_token_size overridden: <value>")`) when `SELECT @@innodb_ft_min_token_size` is not 3. `TestFullTextMySQLMissingIndex` runs `fts.MySQLMatch([]momo.Column[string]{artBody}, "x", fts.Boolean)` against a table with an index over `title, body` and asserts `errors.Is(err, momo.ErrValidation)`.

- [ ] Step 5. Verify on CI: `unit`, `lint`, `fuzz-smoke`, `integration`, `integration-mysql` (all three servers). Poll at most every 10 minutes.
- [ ] Step 6. Commit in a separate call: `feat(fts): portable search on MySQL and MariaDB boolean mode`.

## Task 8: Migrate lint warnings

Covers: S-5 (the lint half of `TestFullTextConfigChangeDiff`), Q6 lint decisions for the first InnoDB FULLTEXT index and for a changed PostgreSQL configuration; G1 is not covered here.

Files:

- Modify `migrate/lint.go`: two rules and their constants.
- Modify `migrate/lint_test.go`.
- Modify `docs/migrations.md`: the two rows of the lint table (the rest of the docs is Task 10).

Interfaces:

```go
const (
	// RuleFullTextRewrite: ADD COLUMN ... GENERATED ALWAYS AS (... to_tsvector ...) STORED on an existing table (table rewrite under ACCESS EXCLUSIVE).
	RuleFullTextRewrite = "fulltext-column-rewrite"
	// RuleFullTextFirstIndex: CREATE FULLTEXT INDEX on an existing table (the first one adds FTS_DOC_ID and rebuilds the table).
	RuleFullTextFirstIndex = "fulltext-first-index"
)
```

Both are `SeverityWarning`. `lintPostgres` gets the first, matching the lowered statement against `alter table <existing> add column ... generated always as (` containing `to_tsvector(` and ending `stored`; a new `lintMySQL` branch keyed by `dialectName` gets the second, matching `create fulltext index ... on <existing table>`. Both use the existing `existing(table)` check so a statement on a table created in the same migration is quiet.

- [ ] Step 1. Write the failing test in `migrate/lint_test.go`. Expect FAIL with "undefined: RuleFullTextRewrite".

```go
// Catches: a table rewrite or a table rebuild passing lint silently, and a
// warning on a table created in the same migration.
func TestLintFullText(t *testing.T) {
	parents := map[string]bool{"articles": true}
	pg := []Statement{
		{SQL: `ALTER TABLE "articles" ADD COLUMN "articles_search_tsv" tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce("title", ''))) STORED`},
		{SQL: `CREATE TABLE "notes" ("id" bigint, "v" tsvector GENERATED ALWAYS AS (to_tsvector('simple', 'x')) STORED)`},
	}
	got := LintSQL("20260101000000", pg, parents, "postgres")
	if len(got) != 1 || got[0].Rule != RuleFullTextRewrite || got[0].Severity != SeverityWarning || got[0].Statement != 1 {
		t.Fatalf("postgres findings = %+v", got)
	}
	my := []Statement{{SQL: "CREATE FULLTEXT INDEX `articles_search` ON `articles` (`title`, `body`)"}}
	got = LintSQL("20260101000000", my, parents, "mysql")
	if len(got) != 1 || got[0].Rule != RuleFullTextFirstIndex || got[0].Alternative == "" {
		t.Fatalf("mysql findings = %+v", got)
	}
	if got := LintSQL("20260101000000", my, map[string]bool{}, "mysql"); len(got) != 0 {
		t.Fatalf("a table created in the same migration was flagged: %+v", got)
	}
}
```

- [ ] Step 2. Implement the two rules; the message names the table and states the cost, the `Alternative` for PostgreSQL is `build the column during a maintenance window (-- momo:no-transaction does not avoid the rewrite)` and for MySQL `add the index in a low-traffic window; the table is copied once`. Add the two rows to the rule table in `docs/migrations.md` next to `sqlite-rebuild`.
- [ ] Step 3. Verify on CI: `unit`, `lint`, and the docs job (the rule table is checked by `internal/docscheck`). Poll at most every 10 minutes.
- [ ] Step 4. Commit in a separate call: `feat(migrate): lint full-text column rewrites and the first FULLTEXT index`.

## Task 9: Hybrid search on SQLite and MariaDB, refusal on MySQL

Covers: S-12 (all eight criteria).

Files:

- Modify `select_hybrid.go`: the dialect branch in `normalize` and `SQL`, two statement templates, the declaration lookup.
- Modify `meta.go`: the `MetaTableSchema` helper if Task 6 did not already add it (it did; reuse it).
- Modify `select_hybrid_test.go`, `select_hybrid_integration_test.go`.
- Create `select_hybrid_portable_test.go`, `select_hybrid_portable_integration_test.go`.

Interfaces:

```go
// HybridSpec is unchanged. Behaviour change in HybridSearch/normalize/SQL:
//   PostgreSQL: unchanged statement and arguments.
//   SQLite and MariaDB: fused statement below; TextConfig ignored.
//   MySQL (no cosine distance in dialect.Vector.Metrics): ErrUnsupported naming the missing distance, no SQL.
```

`normalize` order: querier nil; dialect; a dialect whose `Vector.Metrics` lacks `dialect.MetricCosine` is `ErrUnsupported` with text `hybrid search needs a cosine distance function; MySQL has none`; PostgreSQL keeps its own checks (including `TextConfig` required); on SQLite and MariaDB `Text` must be one of `meta.TableSchema.FullTextIndex()`'s `FullText.Columns`, else `ErrValidation` `hybrid search: column %q has no full-text declaration` naming the column; `ftsquery.Parse(s.Query, ftsquery.Web).Empty()` is not an error (the text leg is then empty by construction and the statement binds a query that matches nothing, see below). The candidate, `RRFK`, `Limit` and dimension checks are the existing ones, shared by all dialects.

SQLite statement (positional `?`, arguments in this order: FTS5 query, text candidates, query vector, vector candidates, RRFK, RRFK, limit; the soft-delete filter binds nothing):

```sql
WITH text_hits AS (
  SELECT pk, row_number() OVER (ORDER BY score ASC, pk ASC) AS rnk
  FROM (
    SELECT t."id" AS pk, bm25("articles_fts"<weights>) AS score
    FROM "articles_fts" JOIN "articles" t ON t."id" = "articles_fts".rowid
    WHERE "articles_fts" MATCH ?<soft>
    ORDER BY score ASC, t."id" ASC
    LIMIT ?
  )
),
vec_hits AS (
  SELECT pk, row_number() OVER (ORDER BY dist ASC, pk ASC) AS rnk
  FROM (
    SELECT t."id" AS pk, vec_distance_cosine(t."emb", ?) AS dist
    FROM "articles" t
    WHERE t."emb" IS NOT NULL<soft>
    ORDER BY dist ASC, t."id" ASC
    LIMIT ?
  )
  WHERE dist IS NOT NULL AND dist = dist
),
fused AS (
  SELECT pk, SUM(term) AS score FROM (
    SELECT pk, 1.0 / (? + rnk) AS term FROM text_hits
    UNION ALL
    SELECT pk, 1.0 / (? + rnk) AS term FROM vec_hits
  ) GROUP BY pk
)
SELECT <cols>, f.score
FROM fused f JOIN "articles" t ON t."id" = f.pk
ORDER BY f.score DESC, f.pk ASC
LIMIT ?
```

MariaDB statement: the same shape with backtick identifiers; the text leg selects ``MATCH(`title`, `body`) AGAINST (? IN BOOLEAN MODE) AS score`` from the base table with `WHERE MATCH(...) AGAINST (? IN BOOLEAN MODE)` and orders `score DESC, pk ASC` (the query binds twice), the vector leg uses ``VEC_DISTANCE_COSINE(t.`emb`, VEC_FromText(?))`` ordered by distance only (so the `VECTOR INDEX` can serve it) with the NaN/NULL filter `dist IS NOT NULL`; the final SELECT is the same. A query with no positive term (`Parsed.Empty()`) produces an empty text leg without sending a MATCH: the leg's WHERE is `1 = 0` and the query-text arguments are not bound (the argument list is then shorter, and the golden for that case pins it); this applies to SQLite and MariaDB alike.

The query text goes through `ftsquery.Parse(s.Query, ftsquery.Web)` and `FTS5()` on SQLite, `MySQLBoolean()` on MariaDB. The vector argument is bound with the helper the vector builders use for the dialect (`wrapMySQLVectorArg` for MariaDB; the plain `Vector.Value()` text form that sqlite-vec accepts for SQLite); read `col_vector_sqlite.go` and `col_vector_mysql.go` and call the same binder. Compose the statement with the same `compile(d, func(b *Builder){...})` plus `b.appendRaw(sql, args)` path the PostgreSQL branch uses; read `appendRaw` to confirm it accepts `?` placeholders for non-PostgreSQL dialects and write the arguments in textual order. The PostgreSQL `hybridSQL` constant is not touched.

- [ ] Step 1. Write the failing unit tests in `select_hybrid_portable_test.go`. Expect FAIL with "hybrid search needs PostgreSQL with pgvector" (the current message on SQLite).

```go
package momo_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/schema"
	"github.com/azrtydxb/momo/vector"
)

type searchDoc = hybridDoc

func init() {
	hybridDocMeta.TableSchema = &schema.Table{
		Name:       "docs",
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "docs_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"body"}}}},
	}
}

// Catches: the SQL shape or argument order of either portable statement
// changing, and the PostgreSQL statement being touched.
func TestHybridSearchGoldenPortable(t *testing.T) {
	spec := hybridSpec()
	spec.TextConfig = "" // ignored off PostgreSQL
	spec.Query = "vector databases"
	sq, err := momo.HybridSearch[searchDoc](momo.New(fakedriver.New(dialect.SQLite)), spec).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		`"docs_fts" MATCH ?`,
		`bm25("docs_fts")`,
		`vec_distance_cosine(t."emb", ?)`,
		`UNION ALL`,
		`GROUP BY pk`,
		`ORDER BY f.score DESC, f.pk ASC`,
	} {
		if !strings.Contains(sq.SQL, frag) {
			t.Errorf("sqlite statement lacks %q:\n%s", frag, sq.SQL)
		}
	}
	if len(sq.Args) != 7 || sq.Args[0] != `"vector" "databases"` || sq.Args[1] != 100 || sq.Args[4] != 100 || sq.Args[5] != 60 || sq.Args[6] != 10 {
		t.Errorf("sqlite args = %#v", sq.Args)
	}
	mq, err := momo.HybridSearch[searchDoc](momo.New(fakedriver.NewFlavor(dialect.MySQL, dialect.FlavorMariaDB)), spec).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		"MATCH(`body`) AGAINST (? IN BOOLEAN MODE)",
		"VEC_DISTANCE_COSINE(t.`emb`, VEC_FromText(?))",
		"UNION ALL",
	} {
		if !strings.Contains(mq.SQL, frag) {
			t.Errorf("mariadb statement lacks %q:\n%s", frag, mq.SQL)
		}
	}
	if !reflect.DeepEqual(mq.Args[0], "+vector +databases") {
		t.Errorf("mariadb text argument = %#v", mq.Args[0])
	}
}

// Catches: MySQL silently degrading to a text-only search, and SQL sent.
func TestHybridSearchMySQLUnsupported(t *testing.T) {
	drv := fakedriver.NewFlavor(dialect.MySQL, dialect.FlavorMySQL)
	db := momo.New(drv)
	_, err := momo.HybridSearch[searchDoc](db, hybridSpec()).All(t.Context())
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "cosine") {
		t.Fatalf("err = %v, want ErrUnsupported naming the cosine distance", err)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("SQL was sent: %v", drv.Calls())
	}
}

// Catches: a text column without a declaration accepted, and a spec validation
// that differs from PostgreSQL.
func TestHybridSpecValidationPortable(t *testing.T) {
	for _, d := range []*fakedriver.Driver{fakedriver.New(dialect.SQLite), fakedriver.NewFlavor(dialect.MySQL, dialect.FlavorMariaDB)} {
		db := momo.New(d)
		bad := hybridSpec()
		bad.Text = momo.NewColumn[string]("docs", "id", 0)
		if _, err := momo.HybridSearch[searchDoc](db, bad).SQL(); !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), `"id"`) {
			t.Errorf("undeclared column: err = %v", err)
		}
		for name, mod := range map[string]func(*momo.HybridSpec){
			"candidates": func(s *momo.HybridSpec) { s.TextCandidates = 1001 },
			"rrfk":       func(s *momo.HybridSpec) { s.RRFK = -1 },
			"limit":      func(s *momo.HybridSpec) { s.Limit = 1001 },
			"dimension":  func(s *momo.HybridSpec) { s.QueryVector = vector.Vector{1, 2} },
		} {
			s := hybridSpec()
			mod(&s)
			if _, err := momo.HybridSearch[searchDoc](db, s).SQL(); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s: err = %v, want ErrValidation", name, err)
			}
		}
	}
}
```

`fakedriver.NewFlavor` stands for the constructor `fakedriver` offers for a MySQL-family flavor; open `internal/fakedriver` and use its actual constructor (the `momo/build_mysql_test.go` tests already create a MariaDB fake). `hybridDocMeta.TableSchema` is assigned in an `init` because `hybridDocMeta` is a package-level variable in `select_hybrid_test.go`.

- [ ] Step 2. Implement the dialect branches. Keep `hybridSQL` for PostgreSQL byte for byte; add `hybridSQLiteSQL`, `hybridMariaDBSQL` and a small `buildHybridPortable(d, meta, s) (sql string, args []any, err error)`. Soft delete uses `softDeletePred(h.meta, "t", false)` exactly as the PostgreSQL branch does.
- [ ] Step 3. Write the failing integration test `select_hybrid_portable_integration_test.go` (`//go:build integration`, `testopen.EachWithMySQL`, skipping `postgres` and `mysql84`/`mysql97` through `if d == dialect.Postgres || db.Dialect().Flavor == dialect.FlavorMySQL { t.Skip(...) }`). Seed ten documents with two-dimensional embeddings and body text so that: documents 1 and 2 match the text, documents 2 and 3 are the nearest vectors, so document 2 is in both legs. Independent expectation computed in the test:

```go
// Catches: the RRF order or any score differing from the sum of 1/(RRFK+rank)
// computed from each leg's own ranking, and a row in one leg outranking a row
// in both.
func TestHybridSearchFusionSQLiteMariaDB(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !hybridCapable(db, d) {
			t.Skip("hybrid needs SQLite with sqlite-vec or MariaDB 11.8")
		}
		seedHybridDocs(t, db, d)
		const k = 60
		res, err := momo.HybridSearch[hdoc](db, momo.HybridSpec{
			Text: hdBody, TextConfig: "english", Query: "fox", Vector: hdEmb,
			QueryVector: vector.Vector{1, 0}, RRFK: k, Limit: 10,
		}).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		textRank := map[int64]int{2: 1, 1: 2} // bm25 / MATCH order of the seed, pinned by seedHybridDocs
		vecRank := map[int64]int{2: 1, 3: 2}  // cosine order of the seed
		want := map[int64]float64{}
		for id, r := range textRank {
			want[id] += 1.0 / float64(k+r)
		}
		for id, r := range vecRank {
			want[id] += 1.0 / float64(k+r)
		}
		if len(res) != len(want) {
			t.Fatalf("rows = %d, want %d", len(res), len(want))
		}
		if res[0].Row.ID != 2 {
			t.Fatalf("the row in both legs is not first: %v", res[0].Row.ID)
		}
		for _, r := range res {
			if math.Abs(r.Score-want[r.Row.ID]) > 1e-9 {
				t.Errorf("id %d: score %v, want %v", r.Row.ID, r.Score, want[r.Row.ID])
			}
		}
	})
}
```

Add `TestHybridSearchTieBreakPortable` (insert the same rows in two different orders into two databases, equal result slices), `TestHybridSearchEmptyTextLegPortable` (query `zzzz`, result is the vector leg with scores `1/(60+rank)`; a row with a NULL embedding never appears), and `TestHybridSearchSoftDeletePortable` (a soft-deleted row in each leg never appears; the fixture model carries a `deleted_at` soft-delete column).

- [ ] Step 4. Verify on CI: `unit`, `lint`, `integration` (SQLite with sqlite-vec), `integration-mysql` (`mariadb118`; the other two servers must skip or see the refusal), and the existing `select_hybrid_test.go` and `select_hybrid_integration_test.go` goldens unchanged. Poll at most every 10 minutes.
- [ ] Step 5. Commit in a separate call: `feat: hybrid search on SQLite and MariaDB`.

## Task 10: Documentation and capability matrix

Covers: S-10 (both criteria), S-12 (docs criterion), and the docs half of S-2 (`TestFullTextScoreNotNormalised`).

Files:

- Modify `dialect/dialect.go`: `FullText bool` and `HybridSearch bool` on `Dialect`, set per dialect and flavor.
- Modify `internal/gendocs/blocks.go`: two rows in the generated capabilities table.
- Modify `docs/capabilities.md`: regenerate the block by hand-equivalent text and edit the two prose rows for full-text and hybrid search (lines 242 and 243 today).
- Modify `docs/fulltext.md`: all three dialects, the comparison table, the limits, hybrid.
- Modify `docs/migrations.md`: the declaration, per-dialect structures, the SQLite `'rebuild'` command, the MySQL `FTS_DOC_ID` rebuild, the PostgreSQL 12 floor.
- Modify `docs/vectors.md`: hybrid on three dialects.
- Create `fts/surface_docs_test.go`.

Interfaces:

```go
// package dialect: Dialect gains
FullText     bool // portable full-text search (package fts)
HybridSearch bool // momo.HybridSearch
```

Values: PostgreSQL `FullText: true, HybridSearch: true`; SQLite `FullText: true, HybridSearch: true`; MySQL flavor `FullText: true, HybridSearch: false`; MariaDB flavor `FullText: true, HybridSearch: true` (set in `MySQLFlavor` next to the `Vector` assignment in `dialect/mysql.go`).

- [ ] Step 1. Write the failing docs-assertion test `fts/surface_docs_test.go`. Expect FAIL with "docs/fulltext.md lost the statement".

```go
package fts_test

import (
	"os"
	"strings"
	"testing"
)

// Catches: the score-comparability statement and the per-dialect comparison
// table being removed from the docs.
func TestFullTextDocsStatements(t *testing.T) {
	doc, err := os.ReadFile("../docs/fulltext.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(doc)
	for _, want := range []string{
		"comparable only within one dialect and one query",
		"| Index type",
		"| Minimum token length",
		"| Transaction visibility",
		"FTS_DOC_ID",
		"'rebuild'",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("docs/fulltext.md lost %q", want)
		}
	}
}
```

- [ ] Step 2. Write the failing generated-table test. Add to `internal/gendocs/gendocs_test.go` an assertion that the generated capabilities block contains the rows `` `FullText` `` and `` `HybridSearch` ``; expect FAIL with "capabilities block lacks FullText". Implement the two fields, then the rows in `internal/gendocs/blocks.go` as `row("`FullText`", func(f family) string { return yesNo(f.D.FullText) })` and `row("`HybridSearch`", ...)` beside `Vector.Storage`. Update the generated block of `docs/capabilities.md` to exactly the text `go generate ./internal/gendocs` would write (the docs check job diffs it; if the rendered column widths differ, copy the diff the job prints).
- [ ] Step 3. Write the docs. `docs/fulltext.md` structure: intro (all three dialects, the portable layer and the dialect builders), declaration (tag and `c.FullText`, one per table, the options and which dialect each applies to), per-dialect structures (PostgreSQL generated column plus GIN and the expression option; SQLite external-content FTS5 plus triggers, `INSERT INTO t_fts(t_fts) VALUES('rebuild')` after a bulk load, the `INSERT OR REPLACE` warning, `recursive_triggers`; MySQL FULLTEXT, `FTS_DOC_ID` rebuild, parser `ngram` on MySQL only), the query API with one `<!-- docscheck:fragment -->` per example (search, score, snippet, raw, MySQL modes), the translation table of Q5, the comparison table with these columns: index type, query syntax, ranking direction, stop words, minimum token length, phrase, prefix, snippet, transaction visibility; the sentence "Scores are comparable only within one dialect and one query; no portable function takes a score threshold"; limits (stop words, `innodb_ft_min_token_size`, committed-only visibility on MySQL, 1 MB tsvector, 16 columns, collation and case); hybrid on the three dialects; the existing PostgreSQL section stays. `docs/vectors.md` hybrid section lists the three dialects and the MySQL refusal. Every Go fence is preceded by the docscheck marker and compiles against the real API; model the fragments on the existing ones in the same file (the fragment harness resolves identifiers such as `articleID` the same way for the new examples).
- [ ] Step 4. Verify on CI: `unit`, `lint`, the docs workflow (`docs.yml`) and `apidiff-docs.yml` (the new exported API is additive). Poll at most every 10 minutes.
- [ ] Step 5. Commit in a separate call: `docs: full-text search on every dialect and hybrid search on SQLite and MariaDB`.

## Acceptance criteria map

| Criterion (spec text, abbreviated)                                          | Test                                                                    | Task                                                                     |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| S-1 portable predicate returns exactly the rows with all words, per dialect | `TestFullTextSearchAllDialects`                                         | 6 (PostgreSQL, SQLite), 7 (MySQL, MariaDB)                               |
| S-1 dialect-specific predicate on the wrong dialect is `ErrUnsupported`     | `TestFullTextWrongDialectUnsupported`                                   | 6                                                                        |
| S-1 rendered SQL and arguments pinned per dialect                           | `TestFullTextSearchGolden`                                              | 6 (PostgreSQL, SQLite), 7 (`TestFullTextMySQLGolden`, MySQL and MariaDB) |
| S-2 portable score orders by occurrences, bm25 negated                      | `TestFullTextScoreOrdering`                                             | 6, 7                                                                     |
| S-2 scores comparable only within a dialect and a query, no threshold API   | `TestFullTextScoreNotNormalised`                                        | 6 (API surface), 10 (docs statement)                                     |
| S-3 hostile and fuzz input never errors                                     | `FuzzFullTextWebQuery`, `TestFullTextWebQueryNeverErrors`               | 6, 7                                                                     |
| S-3 query text always bound                                                 | `TestFullTextQueryIsBound`                                              | 6, 7                                                                     |
| S-3 empty query sends no MATCH                                              | `TestFullTextEmptyQuery`                                                | 6, 7                                                                     |
| S-9 phrase and prefix per dialect, MySQL short prefix matches nothing       | `TestFullTextPhraseAndPrefix`                                           | 6, 7                                                                     |
| S-9 exclusion and negation-only query                                       | `TestFullTextExclusion`                                                 | 6, 7                                                                     |
| S-9 snippet on PostgreSQL and SQLite, `ErrUnsupported` on MySQL             | `TestFullTextSnippet`                                                   | 6 (PostgreSQL, SQLite), 7 (MySQL)                                        |
| S-4 tag equals call, generator rejects non-text and unknown field           | `TestGenerateFullTextDeclaration`                                       | 1                                                                        |
| S-4 snapshot round trip and old snapshot loads                              | `TestSchemaFullTextSnapshotCompat`                                      | 1                                                                        |
| S-5 PostgreSQL generated column, GIN, second diff empty                     | `TestFullTextPostgresMigrationRoundTrip`                                | 2                                                                        |
| S-5 config change alters the column, rebuilds the index, lint flags it      | `TestFullTextConfigChangeDiff`                                          | 2 (diff), 8 (lint)                                                       |
| S-5 existing trigger and GIN recipe unchanged                               | `TestTSVectorMigrationRoundTrip`                                        | 2 (stays green)                                                          |
| S-6 FTS5 table, triggers, second diff empty                                 | `TestFullTextSQLiteMigrationRoundTrip`                                  | 3                                                                        |
| S-6 insert, update, delete change the results                               | `TestFullTextSQLiteSync`                                                | 3                                                                        |
| S-6 integrity-check after update and delete                                 | `TestFullTextSQLiteIntegrityAfterUpdateDelete`                          | 3                                                                        |
| S-6 composite or non-integer key is `ErrValidation`                         | `TestFullTextSQLiteNeedsIntegerKey`                                     | 3                                                                        |
| S-6 drop order triggers then table                                          | `TestFullTextSQLiteDrop`                                                | 3                                                                        |
| S-6 both SQLite drivers checked for FTS5 and bm25                           | `TestFullTextSQLiteDriverSupport`                                       | 3                                                                        |
| S-7 `CREATE FULLTEXT INDEX` with parser, second diff empty                  | `TestFullTextMySQLMigrationRoundTrip`                                   | 4 (apply), 5 (second diff); parser on MySQL only (G2)                    |
| S-7 non-text, unique and non-InnoDB are `ErrValidation`                     | `TestFullTextMySQLRenderValidation`                                     | 4 (engine via `Table.Engine`, G3)                                        |
| S-7 MySQL match builder modes and missing index maps to `ErrValidation`     | `TestFullTextMySQLMatchGolden`, `TestFullTextMySQLMissingIndex`         | 7                                                                        |
| S-8 inspection per dialect with zero diff                                   | `TestFullTextInspectRoundTrip`                                          | 5                                                                        |
| S-8 shadow tables hidden, non-FTS5 virtual table ignored                    | `TestSQLiteInspectIgnoresShadowTables`                                  | 5                                                                        |
| S-8 `--from-db` emits the declaration                                       | `TestGenerateFromDBFullText`                                            | 5                                                                        |
| S-11 transaction visibility per dialect                                     | `TestFullTextTransactionVisibility`                                     | 7                                                                        |
| S-11 MySQL minimum token size                                               | `TestFullTextMySQLMinTokenSize`                                         | 7                                                                        |
| S-11 NULL columns                                                           | `TestFullTextNulls`                                                     | 6, 7                                                                     |
| S-11 hook masking on the portable calls                                     | `TestFullTextHooksSeeMaskedArgsPortable`                                | 6, 7                                                                     |
| S-11 existing `fts` goldens unchanged                                       | `TestFullTextGolden`, `TestFullTextSearch`                              | 6 (stay green on every task)                                             |
| S-10 `docs/fulltext.md` comparison table, capabilities row                  | docs check job, `TestFullTextDocsStatements`                            | 10                                                                       |
| S-10 `docs/migrations.md` section                                           | docs check job                                                          | 10                                                                       |
| S-12 fusion order and scores on SQLite and MariaDB                          | `TestHybridSearchFusionSQLiteMariaDB`                                   | 9                                                                        |
| S-12 ties break by primary key, insertion order does not matter             | `TestHybridSearchTieBreakPortable`                                      | 9                                                                        |
| S-12 empty text leg and NULL embedding                                      | `TestHybridSearchEmptyTextLegPortable`                                  | 9                                                                        |
| S-12 MySQL 9.x is `ErrUnsupported`, no SQL                                  | `TestHybridSearchMySQLUnsupported`                                      | 9                                                                        |
| S-12 undeclared text column and shared spec validation                      | `TestHybridSpecValidationPortable`                                      | 9                                                                        |
| S-12 goldens pinned, PostgreSQL unchanged                                   | `TestHybridSearchGoldenPortable` and existing `select_hybrid_test.go`   | 9                                                                        |
| S-12 soft-deleted rows excluded from both legs                              | `TestHybridSearchSoftDeletePortable`                                    | 9                                                                        |
| S-12 docs and generated capabilities                                        | docs check job                                                          | 10                                                                       |
| Failure modes: missing FTS5 module is `ErrUnsupported` naming FTS5          | `TestExecStatementFTS5Missing`                                          | 3                                                                        |
| Failure modes: drift reports a missing or altered sync trigger              | `TestFullTextSQLiteDriftReportsTrigger`                                 | 5                                                                        |
| Q6 lint: config change and first FULLTEXT index                             | `TestLintFullText`                                                      | 8                                                                        |
| Q6 lint: a PostgreSQL search config that differs from the index             | `TestFullTextTargetValidation` (build-time `ErrValidation`, not a lint) | 6 (gap G1)                                                               |

No criterion is left without a task. Four items are covered by a stated deviation or addition, listed under "Spec gaps found while planning": G1 (the third lint), G2 (MariaDB parser), G3 (`Table.Engine`), G4 (first-index lint over-approximation).

## Sizing

Ten tasks, ten pull requests, each green and mergeable alone, in this order: 1, then 2, 3 and 4 in parallel (they touch different dialect packages; 2 introduces `migrate.SchemaExpander`, so 3 and 4 branch from main after 2 merges or carry the hook in their own diff only if 2 is still open), then 5, 6 after 1 (6 needs only the schema and `Meta`, so it can start once 1 merges), 7 after 6, 8 after 2 and 4, 9 after 3, 4 and 7, 10 last. Two sprints: the first for Tasks 1 to 5 (schema, three migration dialects, inspection), the second for Tasks 6 to 10 (query layer, lint, hybrid, docs).
