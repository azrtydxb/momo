# momo-11-adoption — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

Scope ids: S-91 (momo import gorm|bun|ent), S-92 (momo generate --from-db), S-93 (history importers: Atlas directories, bun_migrations, goose, golang-migrate), S-94 (Go-function migrations, split directive, group rollback), S-95 (migration guides for GORM, Bun and Ent users with compiled code blocks; TestMigrationGuidesComplete).

Research and verification:
- Ent schema loading: `entc/load` (`(*load.Config).Load`) builds the schema package as a Go plugin, which executes user code. This plan reads Ent schemas statically with go/parser instead (decision D-11.1, reported as an ambiguity). Official reference: https://entgo.io/docs/ (schema and entc/load pages).
- goose, golang-migrate, bun and Atlas history layouts: the importer tasks pin the layout by reading the library source in the Go module cache of the exact version the fixture module requires (`go list -m -json <module>` then read the `Dir` field). The docs pages were not reliable at planning time (two returned 404), so the source file path is cited in each importer task's Interfaces section.
- The migration engine, history and checksum formats are the ones defined in plan momo-04-migrations; this plan does not redefine them.

## Goal

Let GORM, Bun, Ent and goose/golang-migrate/Atlas users adopt Momo by converting their models and migration history with reported semantic changes, generating models from an existing database, and writing Go-function migrations, all verified against the source ORM's own schema.

## Architecture

Importers are read-only static analyzers (go/parser, no execution of user code, no reflection) that produce Momo model source through `gen.Writer` (the only file writer in the generator) plus an import report listing every semantic change. History importers read a source tool's history table or directory and emit converted migration files, a baseline snapshot computed by applying the converted SQL to a scratch database, and history rows, so no migration re-runs. Fixture projects that use the real GORM, Bun and Ent libraries live in separate Go modules under `internal/compat/` so the main module's import graph stays free of them.

## Constraints

- Go: minimum 1.26 in go.mod; CI tests 1.26 and 1.27. Dependencies pinned in go.mod/go.sum.
- Generator and migration tooling dependencies (golang.org/x/tools etc.) stay out of the application runtime import graph; benchmark comparison ORMs live in a separate Go module.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- CI: GitHub Actions on free GitHub-hosted runners only (public repo): ubuntu-latest for most jobs, windows-latest for the SQLite/generator suite, PostgreSQL+pgvector as a `pgvector/pgvector` service container, SQLite+sqlite-vec via cgo on the runner; a macos-latest job builds and runs the SQLite suite to cover darwin/cgo. No self-hosted or paid larger runners.
- Persisted formats carry explicit format versions independent of the module version (the imported history rows use the momo_migrations format from plan momo-04).

## Dependencies on other plans (contract names used here)

- momo-02-generator: `gen.Load(patterns []string, cfg Config) (*Graph, []Diagnostic, error)`, `gen.Emit(g *Graph, w *Writer) error`, `gen.Writer`, `Graph{Models []*Model; Schema *schema.Schema}`, `Diagnostic{File string; Line, Col int; Message string}`.
- momo-04-migrations: `migrate.NewEngine`, `migrate.Diff`, `Engine.Inspect`, `Engine.Apply`, `Engine.Baseline`, `Engine.Drift`, `migrate.Dialect` (`Inspect`, `Render`), `migrate.HistoryRow`, `migrate.EnsureHistory`, `migrate.WriteSnapshot`, `migrate.SnapshotFile`, `migrate.ComputeSum`, `migrate.Pending`, `migrate.Statement`, and the directives in `migrate/files.go`.
- momo-03-drivers: `pgxdriver.New`, `sqlitedriver.Open`.
- momo-08-mysql (not yet planned in this set): `migrate/mysql` Inspect. Task 7 is gated on it.
- momo-04-migrations: `migratetest.NewEngine`, `migratetest.WithSum`, `migratetest.FS`, `migratetest.EmptyFS` (Task 3); `Engine.Rollback` with group rollback (Task 19); `migrate.WithDialect` (Task 15).
- Plan momo-03 provides `func (db *DB) OnConn(pc momo.PinnedConn) *momo.DB`, `Driver.Exec`, and `internal/testdb/testopen` (`Postgres(t, opts ...momo.Option) *momo.DB`, `SQLite(t)`, `Each`).
- `internal/testdb` (momo-01) provides URLs only; `*momo.DB` handles come from `internal/testdb/testopen` (momo-03).

## Directory and file layout (owned by this plan)

- `importer/report.go`, `importer/emit.go` (Task 1).
- `importer/gorm/read.go`, `importer/gorm/read_test.go` (Task 2); `importer/bun/read.go` (Task 3); `importer/ent/read.go` (Task 4).
- `importer/testdata/<orm>/...` fixture source packages.
- `internal/compat/gormfixture/go.mod`, `internal/compat/bunfixture/go.mod`, `internal/compat/entfixture/go.mod` (separate modules, Tasks 2 to 4).
- `cmd/momo/import.go`, `cmd/momo/import_test.go` (Task 5).
- `gen/fromdb/mapping.go`, `gen/fromdb/mapping_test.go` (Task 6); `gen/fromdb/emit.go`, `gen/fromdb/fromdb_test.go` (Task 7).
- `importer/history/history.go`, `importer/history/history_test.go` (Task 8); `importer/history/atlas/atlas.go` (Task 9); `importer/history/goose/goose.go` (Task 10); `importer/history/golangmigrate/golangmigrate.go` (Task 11); `importer/history/bun/bun.go` (Task 12).
- `internal/importtest/importtest.go` (Task 9, shared end-to-end check on PostgreSQL), `importer/history/{atlas,goose,golangmigrate,bun}/*_test.go` (Tasks 9 to 12, `TestHistoryImporters` subtests), `internal/compat/{atlasfixture,goosefixture,gmfixture,bunmigfixture}/` (Tasks 9 to 12).
- `migrate/gomigration.go`, `migrate/gomigration_test.go` (Task 13), plus the small hook in `migrate/runner.go` and `migrate/engine.go` owned by plan momo-04.
- `docs/guides/gorm.md`, `docs/guides/bun.md`, `docs/guides/ent.md` (Task 14), `internal/docscheck/guides_test.go` (Task 15; completeness only).

## Task 1: Import report and model emitter

Covers: S-91 (every semantic change reported). Acceptance: TestImportReportListsSemanticChanges.

Files:
- Create: `importer/report.go`, `importer/emit.go`, `importer/report_test.go`.

Interfaces (exact):
```go
package importer

type Kind string // "zero-value-handling", "implicit-save", "go-default", "db-default", "unsupported", "renamed-type", "dropped-field", "naming", "version-mapped"
type Change struct {
    Kind    Kind
    Source  string // file:line:col in the source ORM project
    Model   string
    Field   string
    Message string
}
type Report struct{ Changes []Change }
func (r *Report) Add(c Change)
func (r *Report) Has(kind Kind) bool
func (r *Report) String() string // one line per change, sorted by Source
// Emit writes Momo model source for the models through gen.Writer; it refuses to write when the report has an Unsupported change (returns an error that lists them).
func Emit(models []Model, rep *Report, w *gen.Writer, pkg string) error

type Model struct {
    Name, Table string
    Fields      []Field
    Source      string
}
type Field struct {
    Name, Column, GoType, SQLType string
    Nullable, PK, Unique, Index   bool
    Default, DefaultKind          string // DefaultKind: "db" or "go"
    Source                        string
}
```

Steps:

- [ ] **Step 1: Failing test**

`importer/report_test.go`:
```go
package importer

import (
    "strings"
    "testing"
)

func TestImportReportListsSemanticChanges(t *testing.T) {
    var r Report
    r.Add(Change{Kind: "zero-value-handling", Source: "models/user.go:12:2", Model: "User", Field: "Active", Message: "zero value false is omitted on Create"})
    r.Add(Change{Kind: "go-default", Source: "models/user.go:14:2", Model: "User", Field: "Role", Message: "Go default applied before insert"})
    out := r.String()
    if !strings.Contains(out, "zero-value-handling") || !strings.Contains(out, "go-default") {
        t.Fatalf("report lost a change: %s", out)
    }
    if !r.Has("go-default") || r.Has("implicit-save") {
        t.Fatal("Has reports the wrong kinds")
    }
}
```
Run: `go test ./importer/ -run TestImportReportListsSemanticChanges -count=1`. Expect FAIL with `undefined: Report`.

- [ ] **Step 2: Implement** the types and `String` (sorted by Source with `slices.SortFunc`). `Emit` renders each model as a Go struct embedding `momo.Model`, with `momo:"..."` tags built from Field values, and writes it with `w.Write(path, src)` where `path` is `<pkg>/models.go`; `gen.Writer` gofmts the output. The struct renderer escapes tag values with `strconv.Quote` for the tag text.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(importer): semantic change report and model emitter"`.

## Task 2: GORM static reader and fixture module

Covers: S-91 (gorm). Acceptance: TestImportGORM.

Files:
- Create: `importer/gorm/read.go`, `importer/gorm/read_test.go`, `internal/compat/gormfixture/go.mod`, `internal/compat/gormfixture/models.go`, `internal/compat/gormfixture/gormfixture_test.go`.

Interfaces (exact):
```go
package gorm
// Read parses the Go files of dir (no execution) and returns the models and the report.
// It reads gorm tags: column, type, size, primaryKey, not null, unique, index, uniqueIndex, default, comment, embedded, embeddedPrefix, autoCreateTime, autoUpdateTime, check. It recognises gorm.Model embedding (ID uint PK; CreatedAt/UpdatedAt/DeletedAt) and TableName() string methods returning a literal.
// Semantic changes always reported: zero-value-handling for every non-pointer non-default field (GORM omits zero values on Create), go-default for default:now-style Go defaults (db vs go kind per GORM's rules), implicit-save for associations (has-one/has-many/belongs-to fields with struct or slice types).
func Read(dir string) ([]importer.Model, *importer.Report, error)
```
GORM default-kind rule: a `default:` tag value is a DB default (SQL literal) unless the field also carries `gorm:"autoCreateTime"`; field with a Go-side initializer in a constructor is out of scope and not read.

Fixture module `internal/compat/gormfixture/go.mod`:
```
module github.com/azrtydxb/momo/internal/compat/gormfixture

go 1.26

require gorm.io/gorm v1.31.0
require gorm.io/driver/sqlite v1.6.0
```
Versions are the pins the implementer confirms at Step 2 (latest releases at least two weeks old), recorded in go.sum.

Steps:

- [ ] **Step 1: Fixture models and failing test**

`internal/compat/gormfixture/models.go`:
```go
package gormfixture

import "gorm.io/gorm"

type Org struct {
    gorm.Model
    Name string `gorm:"size:120;not null;uniqueIndex"`
}

type User struct {
    gorm.Model
    OrgID  uint   `gorm:"not null;index"`
    Org    Org    `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
    Email  string `gorm:"size:255;not null;uniqueIndex:idx_user_email"`
    Active bool   `gorm:"default:true"`
    Score  int    `gorm:"type:bigint;default:0"`
    Note   *string
}

func (User) TableName() string { return "app_users" }
```
`internal/compat/gormfixture/gormfixture_test.go` (the fixture test that runs GORM AutoMigrate to produce the source schema and compares it with the Momo generated models):
```go
package gormfixture

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/importer/gorm"
    "github.com/azrtydxb/momo/migrate"
    msqlite "github.com/azrtydxb/momo/migrate/sqlite"
    "github.com/azrtydxb/momo/schema"
    "gorm.io/driver/sqlite"
    gormdb "gorm.io/gorm"
)

func TestImportGORM(t *testing.T) {
    // 1. the source ORM's own schema
    g, err := gormdb.Open(sqlite.Open("file:gormfix?mode=memory&cache=shared"), &gormdb.Config{})
    if err != nil {
        t.Fatal(err)
    }
    if err := g.AutoMigrate(&Org{}, &User{}); err != nil {
        t.Fatal(err)
    }
    source := inspectGORM(t, g) // reads the GORM-created DB through migrate/sqlite Inspect

    // 2. Momo models read from the source project
    models, rep, err := gorm.Read(".")
    if err != nil {
        t.Fatal(err)
    }
    if !rep.Has("zero-value-handling") || !rep.Has("implicit-save") {
        t.Fatalf("semantic changes not reported: %s", rep)
    }
    desired := importedSchema(t, models) // helper: builds schema.Schema from importer.Model via the same mapping the emitter uses

    // 3. zero diff against the source ORM's schema
    changes, _, err := migrate.Diff(source, desired, migrate.DiffOptions{AllowDestructive: true})
    if err != nil {
        t.Fatal(err)
    }
    if len(changes) != 0 {
        t.Fatalf("imported models differ from the GORM schema: %+v", changes)
    }
}
```
Create the shared helper package `internal/compat/schemafixture/schemafixture.go` (used by Tasks 3 and 4 as well) with `func Inspect(t *testing.T, db *sql.DB) *schema.Schema` (migrate/sqlite Inspect over the connection, via `sqlitedriver.FromSQLDB`) and `func ImportedSchema(models []importer.Model) *schema.Schema` (the same mapping importer.Emit uses, producing a schema.Schema without writing files). Replace the `inspectGORM` and `importedSchema` calls above with `schemafixture.Inspect(t, sqldb)` and `schemafixture.ImportedSchema(models)`.

- [ ] **Step 2: Implement `importer/gorm/read.go`** using go/parser and go/ast over the files in dir; map tag options to importer.Field; build the report for zero-value-handling (one entry per non-pointer field without a default), implicit-save (one per association field), and go-default when a `default:` value is a Go identifier (not a SQL literal). Then run the fixture test; expect PASS.

- [ ] **Step 3: Failing test for the implicit-save report and source positions**

Append to `importer/gorm/read_test.go` (package `gorm`; `Read` skips `_test.go` files, so the fixture's own test file is not parsed):
```go
package gorm

import (
    "regexp"
    "testing"
)

var modelsPosition = regexp.MustCompile(`^models\.go:\d+:\d+$`)

func TestReadReportsImplicitSaveWithPosition(t *testing.T) {
    _, rep, err := Read("../../internal/compat/gormfixture")
    if err != nil {
        t.Fatal(err)
    }
    found := false
    for _, c := range rep.Changes {
        if c.Kind != "implicit-save" || c.Model != "User" || c.Field != "Org" {
            continue
        }
        found = true
        if !modelsPosition.MatchString(c.Source) {
            t.Fatalf("source must be models.go:<line>:<col> relative to the directory, got %q", c.Source)
        }
    }
    if !found {
        t.Fatalf("implicit-save for User.Org not reported: %s", rep)
    }
}
```
Run: `go test ./importer/gorm/ -run TestReadReportsImplicitSaveWithPosition -count=1`. Expect FAIL with `implicit-save for User.Org not reported` until Step 2 reports associations with positions; then PASS.

- [ ] **Step 4: Commit** `git commit -m "feat(importer/gorm): static reader with semantic change report and fixture round trip"`.

## Task 3: Bun static reader and fixture module

Covers: S-91 (bun). Acceptance: TestImportBun.

Files:
- Create: `importer/bun/read.go`, `importer/bun/read_test.go`, `internal/compat/bunfixture/go.mod`, `internal/compat/bunfixture/models.go`, `internal/compat/bunfixture/bunfixture_test.go`.
- Uses: `internal/compat/schemafixture/schemafixture.go` created in Task 2 Step 1 (exports `ImportedSchema(models []importer.Model) *schema.Schema` and `Inspect(t *testing.T, db *sql.DB) *schema.Schema`).

Interfaces (exact): `func Read(dir string) ([]importer.Model, *importer.Report, error)` in package `bun`. Tags read: `bun:"table:...,alias:..."` on the embedded `bun.BaseModel`, column names, `pk`, `unique`, `notnull`, `default:`, `type:`, `nullzero`, `rel:belongs-to`, `rel:has-one`, `rel:has-many`, `m2m:`, `scanonly`, `soft_delete`. Semantic changes: `nullzero` is reported as `zero-value-handling` (Momo writes zero values unless the field is omitted); `default:` is a database default (kind `db`); relation tags are reported as `implicit-save` unless the field carries `scanonly`.

Steps:

- [ ] **Step 1: Create the fixture models**

`internal/compat/bunfixture/models.go`:
```go
package bunfixture

import (
    "time"

    "github.com/uptrace/bun"
)

type Org struct {
    bun.BaseModel `bun:"table:orgs,alias:o"`
    ID            int64  `bun:"id,pk,autoincrement"`
    Name          string `bun:"name,notnull,unique"`
}

type Account struct {
    bun.BaseModel `bun:"table:accounts"`
    ID            int64     `bun:"id,pk,autoincrement"`
    OrgID         int64     `bun:"org_id,notnull"`
    Org           *Org      `bun:"rel:belongs-to,join:org_id=id"`
    Email         string    `bun:"email,notnull,unique"`
    Balance       int64     `bun:"balance,nullzero,default:0"`
    CreatedAt     time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp"`
}
```
The module `internal/compat/bunfixture/go.mod` requires `github.com/uptrace/bun`, `github.com/uptrace/bun/dialect/sqlitedialect` and `github.com/uptrace/bun/driver/sqliteshim` at versions at least two weeks old, pinned in go.sum.

- [ ] **Step 2: Write the failing fixture test**

`internal/compat/bunfixture/bunfixture_test.go`:
```go
package bunfixture

import (
    "context"
    "database/sql"
    "testing"

    "github.com/azrtydxb/momo/importer/bun"
    "github.com/azrtydxb/momo/internal/compat/schemafixture"
    "github.com/azrtydxb/momo/migrate"
    "github.com/uptrace/bun"
    "github.com/uptrace/bun/dialect/sqlitedialect"
    "github.com/uptrace/bun/driver/sqliteshim"
)

func TestImportBun(t *testing.T) {
    ctx := context.Background()
    sqldb, err := sql.Open(sqliteshim.ShimName, "file:bunfix?mode=memory&cache=shared")
    if err != nil {
        t.Fatal(err)
    }
    db := bun.NewDB(sqldb, sqlitedialect.New())
    for _, m := range []any{(*Org)(nil), (*Account)(nil)} {
        if _, err := db.NewCreateTable().Model(m).Exec(ctx); err != nil {
            t.Fatal(err)
        }
    }
    source := schemafixture.Inspect(t, sqldb)
    models, rep, err := bun.Read(".")
    if err != nil {
        t.Fatal(err)
    }
    if !rep.Has("zero-value-handling") {
        t.Fatalf("nullzero not reported: %s", rep)
    }
    changes, _, err := migrate.Diff(source, schemafixture.ImportedSchema(models), migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if len(changes) != 0 {
        t.Fatalf("imported models differ from the bun schema: %+v", changes)
    }
}
```
Run: `cd internal/compat/bunfixture && go test ./... -run TestImportBun -count=1`. Expect FAIL with `undefined: bun.Read`.

- [ ] **Step 3: Implement `importer/bun/read.go`**

Parse each `.go` file in dir with `go/parser` (mode `parser.ParseComments`), walk `*ast.GenDecl` of type `TypeSpec` whose struct embeds `bun.BaseModel`, take the raw tag literal of `BaseModel` for the table name, and for each field read the raw tag with `strconv.Unquote`, then split on `,` after the `bun:"` prefix. Map `pk`, `unique`, `notnull`, `default:` (kind `db`), `type:`, `nullzero` (add a `zero-value-handling` change), and `rel:` (add an `implicit-save` change). Source positions are `<file>:<line>:<col>` relative to dir. Run the fixture test: `cd internal/compat/bunfixture && go test ./... -run TestImportBun -count=1`. Expect PASS.

- [ ] **Step 4: Unit test for the report positions**

`importer/bun/read_test.go`:
```go
package bun

import (
    "strings"
    "testing"
)

func TestReadReportsNullzeroWithPosition(t *testing.T) {
    _, rep, err := Read("../../internal/compat/bunfixture")
    if err != nil {
        t.Fatal(err)
    }
    found := false
    for _, c := range rep.Changes {
        if c.Kind == "zero-value-handling" && c.Field == "Balance" && strings.Contains(c.Source, "models.go:") {
            found = true
        }
    }
    if !found {
        t.Fatalf("Balance nullzero change with a file position not reported: %s", rep)
    }
}
```
Run: `go test ./importer/bun/ -count=1`. Expect PASS.

- [ ] **Step 5: Commit**

`git commit -m "feat(importer/bun): static reader with semantic change report and fixture round trip"`

## Task 4: Ent static reader and fixture module (no code execution)

Covers: S-91 (ent). Acceptance: TestImportEnt.

Decision D-11.1: the Ent schema is read with go/parser and go/ast; the schema package is never compiled or executed. Recognised builder chains: `field.<Type>("name")` with `.Optional()`, `.Nillable()`, `.Default(v)`, `.DefaultFunc(f)`, `.Unique()`, `.Immutable()`, `.StorageKey(name)`, `.SchemaType(map[string]string{...})`, `.Comment(s)`; `edge.To`, `edge.From` with `.Ref`, `.Unique`, `.Required`; `index.Fields(...)` with `.Unique()`; `entsql.Annotation{Table: ...}`. Any other method is an `unsupported` change at its `file:line:col`, never a guess. `DefaultFunc` is `go-default`; `Default(literal)` is `go-default` (Ent applies it in Go) unless `entsql.Default` is also present (then `db`). An `edge.From` with `.Ref` is reported as `implicit-save` with the explanation that Ent saves an edge only when set explicitly.

Files:
- Create: `importer/ent/read.go`, `importer/ent/read_test.go`, `internal/compat/entfixture/go.mod`, `internal/compat/entfixture/ent/schema/user.go`, `internal/compat/entfixture/ent/schema/org.go`, `internal/compat/entfixture/entfixture_test.go`.

Interfaces (exact): `func Read(dir string) ([]importer.Model, *importer.Report, error)` in package `ent`. `unsupported` changes block `importer.Emit`.

Steps:

- [ ] **Step 1: Create the fixture schema**

`internal/compat/entfixture/ent/schema/user.go`:
```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/edge"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type User struct{ ent.Schema }

func (User) Fields() []ent.Field {
    return []ent.Field{
        field.String("email").Unique().Immutable(),
        field.String("name").Optional().Nillable(),
        field.Bool("active").Default(true),
        field.Int("score").Default(0),
    }
}

func (User) Edges() []ent.Edge {
    return []ent.Edge{edge.From("org", Org.Type).Ref("users").Unique().Required()}
}

func (User) Indexes() []ent.Index {
    return []ent.Index{index.Fields("name").Unique()}
}
```
`internal/compat/entfixture/ent/schema/org.go`:
```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/edge"
    "entgo.io/ent/schema/field"
)

type Org struct{ ent.Schema }

func (Org) Fields() []ent.Field {
    return []ent.Field{field.String("slug").Unique()}
}

func (Org) Edges() []ent.Edge {
    return []ent.Edge{edge.To("users", User.Type)}
}
```
The fixture module requires `entgo.io/ent` and `modernc.org/sqlite` (pure Go, registered as driver name `sqlite`), pinned at versions at least two weeks old. Generate the ent client once with `go run entgo.io/ent/cmd/ent generate ./ent/schema` from the fixture directory; the generated `ent/` package files are committed.

- [ ] **Step 2: Write the failing fixture test**

`internal/compat/entfixture/entfixture_test.go`:
```go
package entfixture

import (
    "context"
    "database/sql"
    "testing"

    gen "github.com/azrtydxb/momo/internal/compat/entfixture/ent"
    "github.com/azrtydxb/momo/importer/ent"
    "github.com/azrtydxb/momo/internal/compat/schemafixture"
    "github.com/azrtydxb/momo/migrate"
    _ "modernc.org/sqlite"
    entsql "entgo.io/ent/dialect/sql"
    "entgo.io/ent/dialect"
)

func TestImportEnt(t *testing.T) {
    ctx := context.Background()
    sqldb, err := sql.Open("sqlite", "file:entfix?mode=memory&cache=shared")
    if err != nil {
        t.Fatal(err)
    }
    client := gen.NewClient(gen.Driver(entsql.OpenDB(dialect.SQLite, sqldb)))
    if err := client.Schema.Create(ctx); err != nil {
        t.Fatal(err)
    }
    source := schemafixture.Inspect(t, sqldb)
    models, rep, err := ent.Read("./ent/schema")
    if err != nil {
        t.Fatal(err)
    }
    if !rep.Has("implicit-save") || !rep.Has("go-default") {
        t.Fatalf("semantic changes not reported: %s", rep)
    }
    changes, _, err := migrate.Diff(source, schemafixture.ImportedSchema(models), migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if len(changes) != 0 {
        t.Fatalf("imported models differ from the Ent schema: %+v", changes)
    }
}
```
Run: `cd internal/compat/entfixture && go test ./... -run TestImportEnt -count=1`. Expect FAIL with `undefined: ent.Read`.

- [ ] **Step 3: Implement `importer/ent/read.go`** per the decision paragraph above, using `go/parser`, walking method bodies `Fields`, `Edges`, `Indexes` and collecting call chains by unwinding `*ast.CallExpr` receivers. Run the fixture test: expect PASS.

- [ ] **Step 4: Failing unsupported-builder test**

`importer/ent/read_test.go`:
```go
package ent

import (
    "os"
    "path/filepath"
    "testing"
)

func TestReadUnsupportedBuilderIsDiagnosed(t *testing.T) {
    dir := t.TempDir()
    src := "package schema\n\nimport (\n\t\"entgo.io/ent\"\n\t\"entgo.io/ent/schema/field\"\n)\n\ntype X struct{ ent.Schema }\n\nfunc (X) Fields() []ent.Field {\n\treturn []ent.Field{field.String(\"x\").Foo()}\n}\n"
    if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o644); err != nil {
        t.Fatal(err)
    }
    _, rep, err := Read(dir)
    if err != nil {
        t.Fatal(err)
    }
    if !rep.Has("unsupported") {
        t.Fatalf("unknown builder method must be reported as unsupported: %s", rep)
    }
}
```
Run: `go test ./importer/ent/ -run TestReadUnsupportedBuilderIsDiagnosed -count=1`. Expect FAIL before Step 3, PASS after.

- [ ] **Step 5: Commit**

`git commit -m "feat(importer/ent): static schema reader with unsupported-builder diagnostics"`

## Task 5: `momo import gorm|bun|ent` command

Covers: S-91 (command contract: writes model files, prints the report, refuses on unsupported). Acceptance: TestImportCommand, TestImportRefusesUnsupported.

Files:
- Create: `cmd/momo/import.go`, `cmd/momo/import_test.go`.
- Modify: `cmd/momo/main.go` (one dispatch case; the file is owned by plan momo-02, report to the lead).

Interfaces (exact):
```go
// momo import <gorm|bun|ent> --src <dir> --out <pkg dir> [--report <file>]
// Exit 0 when models are written; exit 1 when the report has an unsupported change (nothing is written); exit 2 usage.
func runImport(out, errOut io.Writer, args []string) int
```

Steps:

- [ ] **Step 1: Failing test**

`cmd/momo/import_test.go`:
```go
package main

import (
    "bytes"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestImportCommand(t *testing.T) {
    src := filepath.Join("..", "..", "internal", "compat", "gormfixture")
    dst := t.TempDir()
    var out, errOut bytes.Buffer
    code := runImport(&out, &errOut, []string{"gorm", "--src", src, "--out", dst})
    if code != 0 {
        t.Fatalf("exit %d: %s", code, errOut.String())
    }
    b, err := os.ReadFile(filepath.Join(dst, "models.go"))
    if err != nil || !strings.Contains(string(b), "Code generated by momo") {
        t.Fatalf("models.go missing or not marked generated: %v", err)
    }
    if !strings.Contains(out.String(), "zero-value-handling") {
        t.Fatalf("report not printed: %s", out.String())
    }
}

func TestImportRefusesUnsupported(t *testing.T) {
    src := t.TempDir()
    if err := os.WriteFile(filepath.Join(src, "schema.go"), []byte("package schema\nimport \"entgo.io/ent\"\ntype X struct{ ent.Schema }\nfunc (X) Fields() []ent.Field { return nil }\n"), 0o644); err != nil {
    	t.Fatal(err)
    }
    dst := t.TempDir()
    var out, errOut bytes.Buffer
    if code := runImport(&out, &errOut, []string{"ent", "--src", src, "--out", dst}); code != 1 {
        t.Fatalf("unsupported builder must exit 1, got %d", code)
    }
    if _, err := os.Stat(filepath.Join(dst, "models.go")); err == nil {
        t.Fatal("nothing may be written when the report is unsupported")
    }
}
```
Run: `go test ./cmd/momo/ -run 'TestImport' -count=1`. Expect FAIL with `undefined: runImport`.

- [ ] **Step 2: Implement** `runImport` dispatching to the three readers, `importer.Emit`, and printing `rep.String()` to `out`. Emit writes nothing when unsupported.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(cli): momo import for gorm, bun and ent"`.

## Task 6: Type mapping for generate --from-db

Covers: S-92 (mapping per dialect; unmapped types are errors, never silent). Acceptance: TestGenerateFromDBMapping, TestGenerateFromDBUnmapped.

Files:
- Create: `gen/fromdb/mapping.go`, `gen/fromdb/mapping_test.go`.

Interfaces (exact):
```go
package fromdb
type GoType struct{ Expr string; Import string; Nullable bool; Codec string } // Codec names a momo/types or vector type when needed
// MapType returns the Go type for a normalized SQL type (schema.NormalizeType output) on dialect d.
// Unknown types return ErrUnmapped wrapping the table, column and type; never a fallback to string.
var ErrUnmapped = errors.New("fromdb: unmapped column type")
func MapType(d dialect.Name, sqlType string) (GoType, error)
```
Mapping table (exact, PostgreSQL): `smallint`→`int16`; `integer`→`int32`; `bigint`→`int64`; `real`→`float32`; `double precision`→`float64`; `numeric(...)`→`types.Decimal`? (not in types package: map to `string` with Codec "numeric" and a report note `decimal-as-string` — as an explicit mapping, not silent); `boolean`→`bool`; `text`, `character varying(...)`, `character(...)`→`string`; `bytea`→`[]byte`; `uuid`→`types.UUID`; `date`→`types.Date`; `time without time zone`→`types.TimeOfDay`; `timestamp without time zone`→`time.Time`; `timestamp with time zone`→`time.Time`; `jsonb`, `json`→`types.JSON[json.RawMessage]`; `integer[]`→`[]int32`; `text[]`→`[]string`; `bigint[]`→`[]int64`; `vector(N)`→`vector.Vector`; `halfvec(N)`→`vector.HalfVector`; `sparsevec(N)`→`vector.SparseVector`; `bit(N)`→`vector.Bits`; enums (named in schema.Column.Enum)→named string type generated in the package (Codec "enum"). SQLite: `INTEGER`→`int64`; `REAL`→`float64`; `TEXT`→`string`; `BLOB`→`[]byte`; `NUMERIC`→`float64` with report note; `BOOLEAN`→`bool`; `DATETIME`/`TIMESTAMP`→`time.Time`.

Steps:

- [ ] **Step 1: Failing tests**

`gen/fromdb/mapping_test.go`:
```go
package fromdb

import (
    "errors"
    "testing"

    "github.com/azrtydxb/momo/dialect"
)

func TestGenerateFromDBMapping(t *testing.T) {
    cases := []struct {
        d    dialect.Name
        in   string
        want string
    }{
        {dialect.Postgres, "bigint", "int64"},
        {dialect.Postgres, "timestamp with time zone", "time.Time"},
        {dialect.Postgres, "vector(1536)", "vector.Vector"},
        {dialect.Postgres, "text[]", "[]string"},
        {dialect.SQLite, "INTEGER", "int64"},
        {dialect.SQLite, "TEXT", "string"},
    }
    for _, c := range cases {
        got, err := MapType(c.d, c.in)
        if err != nil || got.Expr != c.want {
            t.Errorf("MapType(%s, %q) = %q, %v; want %q", c.d, c.in, got.Expr, err, c.want)
        }
    }
}

func TestGenerateFromDBUnmapped(t *testing.T) {
    _, err := MapType(dialect.Postgres, "geometry(Point,4326)")
    if !errors.Is(err, ErrUnmapped) {
        t.Fatalf("unknown type must be ErrUnmapped, got %v", err)
    }
}
```
Run: `go test ./gen/fromdb/ -count=1`. Expect FAIL with `undefined: MapType`.

- [ ] **Step 2: Implement** `MapType` with the tables above (input normalized through `schema.NormalizeType` first, arrays and vector dimensions parsed by the same helper). Run; expect PASS. Commit `git commit -m "feat(gen/fromdb): explicit per-dialect type mapping with unmapped errors"`.

## Task 7: generate --from-db emit and round trip

Covers: S-92 (generated models diff to zero against the inspected database on SQLite and PostgreSQL; MySQL once plan momo-08 lands). Acceptance: TestGenerateFromDB.

Files:
- Create: `gen/fromdb/emit.go`, `gen/fromdb/fromdb_test.go`.
- Modify: `cmd/momo/generate.go` (plan momo-02 owns it; this task adds `--from-db <env-name>` and `--out <pkg dir>`; report the change to the lead).

Interfaces (exact):
```go
package fromdb
// FromDB inspects the database (migrate Dialect.Inspect on the configured schemas) and returns importer.Model values plus a report.
// Column types that need a note (numeric mapped to string) are reported as kind "renamed-type".
// Tables without a primary key are emitted without one and reported as kind "unsupported", so Generate refuses.
func FromDB(ctx context.Context, c momo.PinnedConn, d dialect.Name, schemas []string) ([]importer.Model, *importer.Report, error)
// Generate writes models through importer.Emit into pkg. It refuses when the report is unsupported.
func Generate(ctx context.Context, c momo.PinnedConn, d dialect.Name, schemas []string, pkg string, w *gen.Writer) (*importer.Report, error)
```

Steps:

- [ ] **Step 1: Write the failing round-trip test**

`gen/fromdb/fromdb_test.go`:
```go
package fromdb

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
    "github.com/azrtydxb/momo/gen"
    "github.com/azrtydxb/momo/internal/testdb"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
)

var sqliteDDL = []string{
    `CREATE TABLE docs (id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT '', score REAL, meta TEXT)`,
}

var postgresDDL = []string{
    `CREATE EXTENSION IF NOT EXISTS vector`,
    `CREATE TYPE doc_state AS ENUM ('draft','live')`,
    `CREATE TABLE docs (org_id bigint NOT NULL, id bigint NOT NULL, state doc_state NOT NULL DEFAULT 'draft', emb vector(3), meta jsonb, amount numeric(10,2), PRIMARY KEY (org_id, id))`,
}

func TestGenerateFromDB(t *testing.T) {
    cases := []struct {
        name string
        open func(*testing.T) *momo.DB
        d    dialect.Name
        ddl  []string
    }{
        {"sqlite", testopen.SQLite, dialect.SQLite, sqliteDDL},
        {"postgres", testopen.Postgres, dialect.Postgres, postgresDDL},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            db := tc.open(t)
            ctx := context.Background()
            for _, s := range tc.ddl {
                if _, err := db.Driver().Exec(ctx, s, nil); err != nil {
                    t.Fatal(err)
                }
            }
            c, err := db.Pin(ctx)
            if err != nil {
                t.Fatal(err)
            }
            defer c.Close()
            // PostgreSQL tests run in the per-test schema (docs/ARCHITECTURE.md, "Per-test isolation"), never in "public".
            var schemas []string
            if tc.d == dialect.Postgres {
                schemas = []string{testdb.Schema(t)}
            }
            inspected, err := migrate.InspectWith(ctx, c, tc.d, migrate.InspectOptions{Schemas: schemas})
            if err != nil {
                t.Fatal(err)
            }
            out := filepath.Join("testdata", "out-"+tc.name)
            if err := os.MkdirAll(out, 0o755); err != nil {
                t.Fatal(err)
            }
            t.Cleanup(func() { os.RemoveAll(out) })
            if _, err := Generate(ctx, c, tc.d, schemas, "out-"+tc.name+"/models", gen.NewWriter(".")); err != nil {
                t.Fatal(err)
            }
            g, diags, err := gen.Load([]string{"./" + out + "/models"}, gen.Config{})
            if err != nil || len(diags) != 0 {
                t.Fatalf("generated package does not load: %v %v", err, diags)
            }
            if !schema.Equal(g.Schema, inspected) {
                t.Fatal("generated models differ from the inspected database")
            }
        })
    }
}
```
`migrate.InspectWith(ctx, c, d, opts)` is a helper added in plan momo-04's `migrate/engine.go` (selects the registered dialect by name and calls its Inspect) and is referenced here as an allowed addition; report it to the lead. Run: `go test ./gen/fromdb/ -run TestGenerateFromDB -count=1`. Expect FAIL with `undefined: Generate`.

- [ ] **Step 2: Implement `FromDB` and `Generate`**

`FromDB` calls the inspected schema, maps each column with `MapType` from Task 6, builds `importer.Model` values (one per table, Go struct name from the table name with the naming preset of momo.yaml), and sets `Field.PK` from `Table.PrimaryKey`. `Generate` calls `importer.Emit`.

- [ ] **Step 3: Run, expect PASS for both subtests**

Run: `MOMO_TEST_POSTGRES_URL=... go test ./gen/fromdb/ -run TestGenerateFromDB -count=1 -v`. Expect `--- PASS: TestGenerateFromDB/sqlite` and `--- PASS: TestGenerateFromDB/postgres`. The mysql subtest is added when plan momo-08 lands; until then it does not exist and must not be skipped silently.

- [ ] **Step 4: Commit**

`git commit -m "feat(gen): generate models from an existing database"`

## Task 8: History import framework

Covers: S-93 (baseline plus converted files; no migration re-runs; non-timestamp versions mapped and reported). Acceptance: TestConvertKeepsTimestampsAndMapsOrdinals, TestDirtySourceRefused. The end-to-end TestHistoryImporters subtests are in Tasks 9 to 12 and use `internal/importtest` (Task 9).

Mapping decision (accepted): a source version that is not a 14-digit timestamp maps to `19700101000000` plus its 1-based ordinal in the source list (19700101000001, 19700101000002, ...). Each mapping is recorded in the import report, and the original version is kept in the history `name` column as `<orig>_<name>`. Timestamp versions are kept unchanged. Readers return migrations in ascending source order.

Database: every importer test runs on PostgreSQL through `testopen.Postgres(t)`. A "scratch database" is a second `testopen.Postgres(t)` that starts empty.

Files:
- Create: `importer/history/history.go`, `importer/history/history_test.go`.

Interfaces (exact):
```go
package history

const MappedBase = "19700101000000"

var ErrSourceDirty = errors.New("history: source migration state is dirty; fix it in the source tool first")
var ErrUnsupportedSource = errors.New("history: source contains unsupported migrations")

// Raw is one migration as read by a tool-specific reader (Tasks 9 to 12).
type Raw struct {
    Version string // the source version string
    Name    string
    Up      []byte
    Down    []byte // nil when the source has no down
    NoTx    bool   // the source marked the migration as non-transactional
    Origin  string // source file name
}

type Source struct {
    Tool        string // "atlas", "goose", "golangmigrate", "bun"
    Migrations  []Raw  // ascending source order
    Applied     []string // source versions recorded applied by the source tool
    Dirty       bool
    Unsupported []string // file names the reader cannot convert
}

type Converted struct {
    Version         string // momo version (14-digit UTC timestamp)
    OriginalVersion string
    Name            string
    Up              []byte
    Down            []byte
    NoTx            bool
    Origin          string
}

// Convert refuses with ErrSourceDirty when Dirty, and with ErrUnsupportedSource when Unsupported is non-empty.
// For each mapped version it adds a report change Kind "version-mapped" (Source = Origin,
// Message = "source version X mapped to Y (ordinal N after base 19700101000000)").
func Convert(src Source) ([]Converted, *importer.Report, error)

// Baseline applies each converted migration to the scratch database in order: migrate.SplitStatements, then
// momo.Exec per statement (autocommit). After each migration it inspects the database with d and returns one
// SnapshotFile per converted migration: Version = conv.Version, ParentVersion = the previous version ("" for the first),
// Schema = the inspected schema. Nothing runs on any other database.
func Baseline(ctx context.Context, scratch *momo.DB, d migrate.Dialect, conv []Converted) ([]migrate.SnapshotFile, error)

// WriteImport writes <version>_<name>.up.sql (first line "-- momo:no-transaction" when NoTx; the down file when present),
// <version>_<name>.snapshot.json for each snapshot, and momo.sum for the directory. It never overwrites: files are created with O_EXCL.
func WriteImport(outDir string, conv []Converted, snaps []migrate.SnapshotFile) error

// HistoryRows returns momo_migrations rows: Version; Name (OriginalVersion+"_"+Name when mapped, else Name);
// Checksum = sum.Files[<version>_<name>.up.sql] (empty when sum is nil); Status applied; StatementsApplied 0;
// ApplyID "import:<tool>"; Kind "sql"; AppliedAt now.
func HistoryRows(conv []Converted, sum *migrate.Sum, tool string, now time.Time) []migrate.HistoryRow
```

Steps:

- [ ] **Step 1: Failing test**

`importer/history/history_test.go`:
```go
package history

import (
    "errors"
    "testing"
    "time"
)

func TestConvertKeepsTimestampsAndMapsOrdinals(t *testing.T) {
    src := Source{Tool: "goose", Migrations: []Raw{
        {Version: "00001", Name: "init", Up: []byte("CREATE TABLE a (x int);"), Down: []byte("DROP TABLE a;"), Origin: "00001_init.sql"},
        {Version: "20260101000000", Name: "more", Up: []byte("CREATE TABLE b (x int);"), Origin: "20260101000000_more.sql"},
    }}
    conv, rep, err := Convert(src)
    if err != nil {
        t.Fatal(err)
    }
    if conv[0].Version != "19700101000001" || conv[0].OriginalVersion != "00001" || conv[0].Down == nil {
        t.Fatalf("mapped version wrong: %+v", conv[0])
    }
    if conv[1].Version != "20260101000000" {
        t.Fatalf("timestamp version must be kept: %+v", conv[1])
    }
    if !rep.Has("version-mapped") {
        t.Fatalf("mapping not recorded in the report: %s", rep)
    }
    again, _, err := Convert(src)
    if err != nil {
        t.Fatal(err)
    }
    if string(again[0].Up) != string(conv[0].Up) || again[0].Version != conv[0].Version {
        t.Fatal("conversion must be deterministic")
    }
    rows := HistoryRows(conv, nil, "goose", time.Unix(0, 0))
    if rows[0].Name != "00001_init" {
        t.Fatalf("original version must be kept in the name column: %q", rows[0].Name)
    }
}

func TestDirtySourceRefused(t *testing.T) {
    _, _, err := Convert(Source{Tool: "golangmigrate", Dirty: true})
    if !errors.Is(err, ErrSourceDirty) {
        t.Fatalf("dirty source must be refused, got %v", err)
    }
}
```
Run: `go test ./importer/history/ -count=1`. Expect FAIL with `undefined: Convert`.

- [ ] **Step 2: Implement**

`Convert` builds `Converted` values in source order; a mapped version uses `MappedBase` plus `index+1`; each mapping adds a `version-mapped` change to the report. `Baseline` runs the statements on the scratch database as described in Interfaces, inspecting after each migration. `WriteImport` and `HistoryRows` follow Interfaces. Run the test: expect PASS.

- [ ] **Step 3: Commit**

`git commit -m "feat(importer/history): conversion with mapped versions, baseline and history rows"`

## Task 9: Atlas directory importer and the shared importer check

Covers: S-93 (Atlas directories; the shared check that every importer's TestHistoryImporters subtest uses). Acceptance: TestHistoryImporters/atlas.

Files:
- Create: `importer/history/atlas/atlas.go`, `importer/history/atlas/atlas_test.go`.
- Create: `internal/importtest/importtest.go` (shared check, used by Tasks 9 to 12).
- Create fixtures: `internal/compat/atlasfixture/migrations/20260101000000_init.sql`, `internal/compat/atlasfixture/migrations/20260101000001_more.sql`, `internal/compat/atlasfixture/migrations/atlas.sum`.

Fixture contents (committed verbatim):

`internal/compat/atlasfixture/migrations/20260101000000_init.sql`:
```sql
CREATE TABLE a (x int);
```

`internal/compat/atlasfixture/migrations/20260101000001_more.sql`:
```sql
CREATE TABLE b (x int);
```

`internal/compat/atlasfixture/migrations/atlas.sum`: produced once by the pinned Atlas CLI (`atlas migrate hash --dir file://internal/compat/atlasfixture/migrations`) and committed. The Atlas CLI is a fixture-generation tool only, not a test dependency; the test reads the committed file.

Interfaces (exact):
```go
package atlas
type Revision struct {
    Version        string
    Applied, Total int
    Error          string
}
// Read returns a history.Source for an Atlas directory: files <version>_<name>.sql in ascending version order;
// Up is the file body; Down is nil. The atlas.sum file is verified with the Atlas directory-hash algorithm
// (read from the ariga.io/atlas source at the pinned version; the file path is recorded in the commit body);
// a mismatch returns an error naming the file. revisions are the rows of atlas_schema_revisions as the caller read them:
// a version is Applied when Applied == Total and Error is empty; Dirty is true when any row has a non-empty Error or Applied < Total.
func Read(dir fs.FS, revisions []Revision) (history.Source, error)
```

`internal/importtest/importtest.go` (literal):
```go
// Package importtest is the shared end-to-end check for history importers.
package importtest

import (
    "context"
    "os"
    "testing"
    "time"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/importer/history"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/migrate/migratetest"
    mpg "github.com/azrtydxb/momo/migrate/postgres"
)

// Input is one imported history. The live database already holds the source tool's history table (created by the test);
// Exclude names the source tool's tables so drift does not report them as unmanaged.
type Input struct {
    Tool    string
    Source  history.Source
    Exclude []string
}

// CheckImport converts the source, builds the snapshots on an empty scratch database, applies the converted SQL to live,
// records history rows, and asserts that every converted version reads as applied and that drift is clean.
func CheckImport(t *testing.T, live *momo.DB, in Input) {
    t.Helper()
    ctx := context.Background()
    conv, _, err := history.Convert(in.Source)
    if err != nil {
        t.Fatalf("convert: %v", err)
    }
    scratch := testopen.Postgres(t)
    snaps, err := history.Baseline(ctx, scratch, mpg.New(), conv)
    if err != nil {
        t.Fatalf("baseline: %v", err)
    }
    dir := t.TempDir()
    if err := history.WriteImport(dir, conv, snaps); err != nil {
        t.Fatalf("write import: %v", err)
    }
    fsys := os.DirFS(dir)
    sum, err := migrate.ComputeSum(fsys)
    if err != nil {
        t.Fatal(err)
    }
    for _, cv := range conv {
        stmts, err := migrate.SplitStatements(cv.Up, "postgres")
        if err != nil {
            t.Fatalf("split %s: %v", cv.Version, err)
        }
        for _, stmt := range stmts {
            if _, err := momo.Exec(ctx, live, stmt); err != nil {
                t.Fatalf("apply %s: %v", cv.Version, err)
            }
        }
    }
    conn, err := live.Pin(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer conn.Close()
    if err := migrate.EnsureHistory(ctx, conn, mpg.New()); err != nil {
        t.Fatal(err)
    }
    for _, row := range history.HistoryRows(conv, sum, in.Tool, time.Now().UTC()) {
        if _, err := momo.Exec(ctx, live,
            `INSERT INTO momo_migrations (version, name, checksum, status, statements_applied, error, applied_at, execution_ms, apply_id, kind) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
            row.Version, row.Name, row.Checksum, row.Status, row.StatementsApplied, row.Error, row.AppliedAt, row.ExecutionMS, row.ApplyID, row.Kind); err != nil {
            t.Fatalf("history row %s: %v", row.Version, err)
        }
    }
    e := migratetest.NewEngine(t, mpg.New(), live.Driver(), fsys, migrate.WithConfig(migrate.Config{
        Exclude: append([]string{"momo_migrations", "momo_migration_lock"}, in.Exclude...),
    }))
    states, err := e.Status(ctx)
    if err != nil {
        t.Fatal(err)
    }
    for _, cv := range conv {
        found := false
        for _, st := range states {
            if st.Version == cv.Version {
                found = true
                if st.State != "applied" {
                    t.Fatalf("%s: status %q, want applied", cv.Version, st.State)
                }
            }
        }
        if !found {
            t.Fatalf("%s missing from status", cv.Version)
        }
    }
    rep, err := e.Drift(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if !rep.Clean() {
        t.Fatalf("imported history must drift to zero: %+v", rep)
    }
}
```

`internal/importtest` is a non-test package and imports `testing` for `*testing.T`; it is used only by test files.


Steps:

- [ ] **Step 1: Failing test**

`importer/history/atlas/atlas_test.go` (literal):

```go
package atlas

import (
    "os"
    "testing"

    "github.com/azrtydxb/momo/internal/importtest"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHistoryImporters(t *testing.T) {
    t.Run("atlas", func(t *testing.T) {
        src, err := Read(os.DirFS("../../../internal/compat/atlasfixture/migrations"), []Revision{
            {Version: "20260101000000", Applied: 1, Total: 1},
            {Version: "20260101000001", Applied: 1, Total: 1},
        })
        if err != nil {
            t.Fatal(err)
        }
        if len(src.Applied) != 2 || src.Dirty {
            t.Fatalf("revisions not read: applied=%v dirty=%v", src.Applied, src.Dirty)
        }
        live := testopen.Postgres(t)
        importtest.CheckImport(t, live, importtest.Input{Tool: "atlas", Source: src})
    })
}
```


Create the fixtures listed under Files. Run: `go test ./importer/history/atlas/ -run TestHistoryImporters/atlas -count=1`. Expect FAIL with `undefined: Read`.

- [ ] **Step 2: Implement `Read` and the atlas.sum check**

List the directory, parse `<version>_<name>.sql` names with a regexp `^(\d+)_([a-z0-9_]+)\.sql$`, sort by version, verify `atlas.sum` with the Atlas algorithm, and derive `Applied` and `Dirty` from `revisions` as in Interfaces. Run the test: expect PASS.

- [ ] **Step 3: Commit**

`git commit -m "feat(importer/atlas): read Atlas directories with sum verification and the shared importer check"`

## Task 10: goose importer

Covers: S-93 (goose). Acceptance: TestHistoryImporters/goose.

Files:
- Create: `importer/history/goose/goose.go`, `importer/history/goose/goose_test.go`.
- Create fixtures: `internal/compat/goosefixture/migrations/00001_init.sql`, `internal/compat/goosefixture/migrations/00002_more.sql`, `internal/compat/goosefixture/migrations/00003_notx.sql`.

Fixture contents (committed verbatim):

`internal/compat/goosefixture/migrations/00001_init.sql`:
```sql
-- +goose Up
CREATE TABLE a (x int);

-- +goose Down
DROP TABLE a;
```

`internal/compat/goosefixture/migrations/00002_more.sql`:
```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE b (x int);
INSERT INTO b VALUES (1);
-- +goose StatementEnd
```

`internal/compat/goosefixture/migrations/00003_notx.sql`:
```sql
-- +goose Up
-- +goose NO TRANSACTION
CREATE TABLE c (x int);
```

Interfaces (exact):
```go
package goose
// splitGoose splits a goose file body into up and down. It handles -- +goose StatementBegin / StatementEnd (a
// statement block containing semicolons is kept whole) and reports -- +goose NO TRANSACTION through the second return of ParseFile.
func splitGoose(body []byte) (up, down []byte, err error)
// Read returns a history.Source. Versions come from the leading number of each file name, kept as written ("00001").
// NoTx is set from the NO TRANSACTION annotation. Applied versions come from the table: the latest row per version_id with
// is_applied true; version_id is matched to the file whose leading number equals it. Row version_id 0 (goose's initial row) is ignored.
// The table layout (id, version_id, is_applied, tstamp) and the annotation grammar are pinned by reading
// github.com/pressly/goose/v3 at the pinned version in the module cache; the file path is recorded in the commit body.
func Read(dir fs.FS, c momo.PinnedConn, table string) (history.Source, error)
```

The `live` database holds only the goose table before `CheckImport` runs; `CheckImport` applies the converted SQL and checks status and drift.

Steps:

- [ ] **Step 1: Failing test**

`importer/history/goose/goose_test.go` (literal):

```go
package goose

import (
    "context"
    "os"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/importtest"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHistoryImporters(t *testing.T) {
    t.Run("goose", func(t *testing.T) {
        ctx := context.Background()
        live := testopen.Postgres(t)
        if _, err := momo.Exec(ctx, live, `CREATE TABLE goose_db_version (id serial PRIMARY KEY, version_id bigint NOT NULL, is_applied boolean NOT NULL, tstamp timestamp NULL DEFAULT now())`); err != nil {
            t.Fatal(err)
        }
        if _, err := momo.Exec(ctx, live, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, true), (1, true), (2, true), (3, true)`); err != nil {
            t.Fatal(err)
        }
        conn, err := live.Pin(ctx)
        if err != nil {
            t.Fatal(err)
        }
        defer conn.Close()
        src, err := Read(os.DirFS("../../../internal/compat/goosefixture/migrations"), conn, "goose_db_version")
        if err != nil {
            t.Fatal(err)
        }
        if len(src.Applied) != 3 {
            t.Fatalf("goose version table not read: applied=%v", src.Applied)
        }
        if !src.Migrations[2].NoTx {
            t.Fatal("NO TRANSACTION annotation not read")
        }
        importtest.CheckImport(t, live, importtest.Input{Tool: "goose", Source: src, Exclude: []string{"goose_db_version"}})
    })
}
```


Create the fixtures listed under Files. Run: `go test ./importer/history/goose/ -run TestHistoryImporters/goose -count=1`. Expect FAIL with `undefined: Read`.

- [ ] **Step 2: Implement**

`splitGoose` scans the body line by line: `-- +goose Up` and `-- +goose Down` switch the target section; `-- +goose StatementBegin` and `-- +goose StatementEnd` bracket a block that is kept verbatim; `-- +goose NO TRANSACTION` sets NoTx. `Read` lists the directory, builds `Raw` values, reads the goose table with `c.Query` (`SELECT version_id FROM goose_db_version WHERE is_applied ORDER BY id`, latest row per version kept), maps each version_id to the file whose leading number equals it, and returns `Applied` in file order. Run the test: expect PASS.

- [ ] **Step 3: Commit**

`git commit -m "feat(importer/goose): read goose directories, annotations and version table"`

## Task 11: golang-migrate importer

Covers: S-93 (golang-migrate). Acceptance: TestHistoryImporters/golangmigrate.

Files:
- Create: `importer/history/golangmigrate/golangmigrate.go`, `importer/history/golangmigrate/golangmigrate_test.go`.
- Create fixtures: `internal/compat/gmfixture/migrations/1_create_a.up.sql`, `internal/compat/gmfixture/migrations/1_create_a.down.sql`, `internal/compat/gmfixture/migrations/2_create_b.up.sql`.

Fixture contents (committed verbatim):

`internal/compat/gmfixture/migrations/1_create_a.up.sql`: `CREATE TABLE a (x int);`
`internal/compat/gmfixture/migrations/1_create_a.down.sql`: `DROP TABLE a;`
`internal/compat/gmfixture/migrations/2_create_b.up.sql`: `CREATE TABLE b (x int);`

Interfaces (exact):
```go
package golangmigrate
// Read returns a history.Source. Files are <version>_<title>.up.sql and <version>_<title>.down.sql; the version is the
// integer prefix as written. Applied state comes from the table with columns version bigint PRIMARY KEY and dirty boolean:
// Applied lists the version when the row exists and dirty is false; Dirty is true when the row is dirty.
// The table layout is pinned by reading the schema_migrations creation statement in the golang-migrate database driver
// source at the pinned version (module cache); the file path is recorded in the commit body.
func Read(dir fs.FS, c momo.PinnedConn, table string) (history.Source, error)
```

The first subtest's acceptance name is `TestHistoryImporters/golangmigrate`. The second subtest name `golangmigrate/dirty-refused` is a subtest of the same top-level test and its name starts with `golangmigrate`; run both with `-run 'TestHistoryImporters/golangmigrate'`.

Steps:

- [ ] **Step 1: Failing test**

`importer/history/golangmigrate/golangmigrate_test.go` (literal):

```go
package golangmigrate

import (
    "context"
    "errors"
    "os"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/importer/history"
    "github.com/azrtydxb/momo/internal/importtest"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHistoryImporters(t *testing.T) {
    fixtures := os.DirFS("../../../internal/compat/gmfixture/migrations")
    ctx := context.Background()

    t.Run("golangmigrate", func(t *testing.T) {
        live := testopen.Postgres(t)
        if _, err := momo.Exec(ctx, live, `CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`); err != nil {
            t.Fatal(err)
        }
        if _, err := momo.Exec(ctx, live, `INSERT INTO schema_migrations (version, dirty) VALUES (2, false)`); err != nil {
            t.Fatal(err)
        }
        conn, err := live.Pin(ctx)
        if err != nil {
            t.Fatal(err)
        }
        defer conn.Close()
        src, err := Read(fixtures, conn, "schema_migrations")
        if err != nil {
            t.Fatal(err)
        }
        if src.Dirty || len(src.Applied) != 1 {
            t.Fatalf("clean state misread: dirty=%v applied=%v", src.Dirty, src.Applied)
        }
        importtest.CheckImport(t, live, importtest.Input{Tool: "golangmigrate", Source: src, Exclude: []string{"schema_migrations"}})
    })

    t.Run("golangmigrate/dirty-refused", func(t *testing.T) {
        live := testopen.Postgres(t)
        if _, err := momo.Exec(ctx, live, `CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`); err != nil {
            t.Fatal(err)
        }
        if _, err := momo.Exec(ctx, live, `INSERT INTO schema_migrations (version, dirty) VALUES (1, true)`); err != nil {
            t.Fatal(err)
        }
        conn, err := live.Pin(ctx)
        if err != nil {
            t.Fatal(err)
        }
        defer conn.Close()
        src, err := Read(fixtures, conn, "schema_migrations")
        if err != nil {
            t.Fatal(err)
        }
        if !src.Dirty {
            t.Fatal("dirty row not read")
        }
        if _, _, err := history.Convert(src); !errors.Is(err, history.ErrSourceDirty) {
            t.Fatalf("dirty source must be refused, got %v", err)
        }
    })
}
```


Create the fixtures listed under Files. Run: `go test ./importer/history/golangmigrate/ -run TestHistoryImporters/golangmigrate -count=1`. Expect FAIL with `undefined: Read`.

- [ ] **Step 2: Implement `Read`**

List the directory, parse the integer prefix of each name with `strconv.ParseInt`, pair `.up.sql` and `.down.sql` by version, and query `SELECT version, dirty FROM <table>` through `c.Query` (table name quoted with `dialect.For(dialect.Postgres).QuoteIdent`). A zero-row result means nothing is applied. Run the test: expect PASS.

- [ ] **Step 3: Commit**

`git commit -m "feat(importer/golang-migrate): read migration directories and schema_migrations"`

## Task 12: Bun migration history importer

Covers: S-93 (bun_migrations). Acceptance: TestHistoryImporters/bun.

Files:
- Create: `importer/history/bun/bun.go`, `importer/history/bun/bun_test.go`.
- Create fixture: `internal/compat/bunmigfixture/migrations/20260101000000_init.up.sql`.

Fixture contents (committed verbatim): `internal/compat/bunmigfixture/migrations/20260101000000_init.up.sql`: `CREATE TABLE a (x int);`

Interfaces (exact):
```go
package bun
// Read returns a history.Source for bun's SQL migrations: files <timestamp>_<name>.up.sql and optional .down.sql.
// bun's Go-function migrations (.go files) are not imported: each one is listed in Source.Unsupported, and Convert refuses the source.
// Applied state comes from the bun_migrations table (columns id, name, group_id, migrated_at); the name column holds the
// version prefix. bun_migration_locks is ignored. The table layout is pinned by reading github.com/uptrace/bun/migrate at
// the pinned version in the module cache; the file path is recorded in the commit body.
func Read(dir fs.FS, c momo.PinnedConn) (history.Source, error)
```

The acceptance name `TestHistoryImporters/bun` selects the first subtest; `bun/go-function-refused` is run with `-run 'TestHistoryImporters/bun'`.

Steps:

- [ ] **Step 1: Failing test**

`importer/history/bun/bun_test.go` (literal):

```go
package bun

import (
    "context"
    "errors"
    "os"
    "testing"
    "testing/fstest"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/importer/history"
    "github.com/azrtydxb/momo/internal/importtest"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHistoryImporters(t *testing.T) {
    ctx := context.Background()

    t.Run("bun", func(t *testing.T) {
        live := testopen.Postgres(t)
        if _, err := momo.Exec(ctx, live, `CREATE TABLE bun_migrations (id BIGSERIAL PRIMARY KEY, name VARCHAR(180), group_id INTEGER NOT NULL DEFAULT 0, migrated_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
            t.Fatal(err)
        }
        if _, err := momo.Exec(ctx, live, `INSERT INTO bun_migrations (name, group_id) VALUES ('20260101000000_init', 1)`); err != nil {
            t.Fatal(err)
        }
        conn, err := live.Pin(ctx)
        if err != nil {
            t.Fatal(err)
        }
        defer conn.Close()
        src, err := Read(os.DirFS("../../../internal/compat/bunmigfixture/migrations"), conn)
        if err != nil {
            t.Fatal(err)
        }
        if len(src.Applied) != 1 {
            t.Fatalf("bun_migrations not read: applied=%v", src.Applied)
        }
        importtest.CheckImport(t, live, importtest.Input{Tool: "bun", Source: src, Exclude: []string{"bun_migrations"}})
    })

    t.Run("bun/go-function-refused", func(t *testing.T) {
        live := testopen.Postgres(t)
        if _, err := momo.Exec(ctx, live, `CREATE TABLE bun_migrations (id BIGSERIAL PRIMARY KEY, name VARCHAR(180), group_id INTEGER NOT NULL DEFAULT 0, migrated_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
            t.Fatal(err)
        }
        conn, err := live.Pin(ctx)
        if err != nil {
            t.Fatal(err)
        }
        defer conn.Close()
        fsys := fstest.MapFS{
            "20260101000000_go_step.go": {Data: []byte("package migrations\n")},
        }
        src, err := Read(fsys, conn)
        if err != nil {
            t.Fatal(err)
        }
        if len(src.Unsupported) != 1 {
            t.Fatalf("Go-function migration not listed as unsupported: %+v", src.Unsupported)
        }
        if _, _, err := history.Convert(src); !errors.Is(err, history.ErrUnsupportedSource) {
            t.Fatalf("Go-function migration must be refused, got %v", err)
        }
    })
}
```


Create the fixtures listed under Files. Run: `go test ./importer/history/bun/ -run TestHistoryImporters/bun -count=1`. Expect FAIL with `undefined: Read`.

- [ ] **Step 2: Implement `Read`**

List the directory: `.up.sql` and `.down.sql` files become `Raw` values, `.go` files become `Source.Unsupported` entries. Read `SELECT name FROM bun_migrations ORDER BY id` through `c.Query` and set `Applied` to the version prefixes (the text before the first `_`). Run the test: expect PASS.

- [ ] **Step 3: Commit**

`git commit -m "feat(importer/bun): read bun SQL migrations and bun_migrations history"`

## Task 13: Go-function migrations

Covers: S-94 (Go-function migrations run once under the lock with history). The split directive is consumed from plan momo-04 Task 4 (not re-specified here). Group rollback is owned by plan momo-04 Task 18, which dispatches Go downs to `GoMigration.Down`. Acceptance: TestGoMigrations.

Files:
- Create: `migrate/gomigration.go`, `migrate/gomigration_test.go`.
- Modify (additive, plan momo-04 files): `migrate/runner.go` (Go branch in RunMigration), `migrate/engine.go` (the WithGoMigrations option and the `goMigrations` field read by Rollback).

Interfaces (exact):
```go
type GoMigration struct {
    Version string // 14-digit UTC timestamp
    Name    string
    NoTx    bool
    Up      func(ctx context.Context, q momo.Querier) error
    Down    func(ctx context.Context, q momo.Querier) error // nil: no down
}
// Up and Down receive the *momo.Tx of the migration transaction when NoTx is false.
// When NoTx is true they receive db.OnConn(conn): a *momo.DB bound to the pinned connection that keeps db's hooks and classifier.
// Raw execution inside the function is momo.Exec(ctx, q, sql, args...).
func WithGoMigrations(ms ...GoMigration) Option
// Checksum: sha256 over Version, a NUL byte, Name, a NUL byte, then "go-func-v1". The function body is not hashed.
// Accepted limit: changing a registered function's logic is not detected; add a new migration instead.
// History: kind "go". Up runs in the same transaction as the applied update unless NoTx.
```
Requires from plan momo-03: `func (db *DB) OnConn(pc momo.PinnedConn) *momo.DB` (contract item 10 of plan momo-04) and `Driver.Exec`.

Steps:

- [ ] **Step 1: Failing test**

`migrate/gomigration_test.go`:
```go
package migrate_test

import (
    "context"
    "testing"
    "testing/fstest"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/migrate/migratetest"
    mpg "github.com/azrtydxb/momo/migrate/postgres"
)

func TestGoMigrations(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    runs := 0
    gm := migrate.GoMigration{
        Version: "20260101000000",
        Name:    "backfill",
        Up: func(ctx context.Context, q momo.Querier) error {
            runs++
            _, err := momo.Exec(ctx, q, `CREATE TABLE g (x int)`)
            return err
        },
        Down: func(ctx context.Context, q momo.Querier) error {
            _, err := momo.Exec(ctx, q, `DROP TABLE g`)
            return err
        },
    }
    fsys := migratetest.WithSum(t, fstest.MapFS{
        "20260101000001_split.up.sql":   {Data: []byte("-- momo:split\nCREATE TABLE s1 (x int); CREATE TABLE s2 (x int);")},
        "20260101000001_split.down.sql": {Data: []byte("DROP TABLE s1; DROP TABLE s2;")},
    })
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, migrate.WithGoMigrations(gm))
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    if runs != 1 {
        t.Fatalf("go migration must run once, ran %d times", runs)
    }
    rows, err := migrate.ReadHistory(ctx, pinned(t, db))
    if err != nil {
        t.Fatal(err)
    }
    seen := map[string]migrate.HistoryRow{}
    for _, r := range rows {
        seen[r.Version] = r
    }
    if seen["20260101000000"].Kind != "go" {
        t.Fatalf("go migration history kind: %+v", seen["20260101000000"])
    }
    if seen["20260101000001"].StatementsApplied != 2 {
        t.Fatalf("split directive from momo-04 Task 4 not honoured: %+v", seen["20260101000001"])
    }
    if _, err := e.Rollback(ctx, migrate.RollbackOptions{Group: true}); err != nil {
        t.Fatal(err)
    }
    if tableExists(t, db, "g") {
        t.Fatal("group rollback must run the Go down")
    }
}
```
Run: `go test ./migrate/ -run TestGoMigrations -count=1`. Expect FAIL with `undefined: migrate.GoMigration`.

- [ ] **Step 2: Implement** the type, the option, the Go branch in RunMigration (the callback receives the transaction, or the pinned DB when NoTx), and the Go dispatch in Rollback (plan momo-04 Task 19). Run the test: expect PASS.

- [ ] **Step 3: Commit**

`git commit -m "feat(migrate): Go-function migrations with history, run once under the lock"`

## Task 14: Migration guides for GORM, Bun and Ent users

Covers: S-95 (guides with call-site cheat sheets and compiled code blocks). Acceptance: TestMigrationGuidesComplete (section presence), (code blocks are compiled by plan momo-13's TestDocsCodeBlocks).

Files:
- Create: `docs/guides/gorm.md`, `docs/guides/bun.md`, `docs/guides/ent.md`, `examples/guides/models/models.go` (the Momo models used by every guide block), `examples/guides/models/momo_gen.go` (generated by `momo generate` from the models file; committed).

Each guide has exactly these level-2 headings in this order: `## Models`, `## Queries`, `## Relations`, `## Hooks`, `## Transactions`, `## Migrations`, `## Errors`. Each code block is a complete Go file (`package main`) that imports `github.com/azrtydxb/momo/examples/guides/models` and compiles on its own (compiled by plan momo-13's TestDocsCodeBlocks); blocks that must not compile are fenced with the info string `go notest` and are excluded from compilation but never used for CI-tested claims. Each guide also has a "Cheat sheet" table mapping the ORM call to the Momo call.

Steps:

- [ ] **Step 1: Failing check**

Run: `for g in gorm bun ent; do grep -c '^## ' docs/guides/$g.md; done`. Expect FAIL (exit code 2 from grep on missing files) before the files exist.

- [ ] **Step 2: Write the guides.** GORM guide: Models (struct tags to Momo `momo:` tags and `MomoConfig`), Queries (`db.Find`/`Where` to `momo.Select[T]`), Relations (`Preload` to `momo.Load` and `.With`), Hooks (`BeforeCreate` to typed model hooks, S-85), Transactions (`db.Transaction` to `db.Tx`, no automatic retry), Migrations (`AutoMigrate` to `momo migrate diff/apply`, AutoMigrate additive only), Errors (`ErrRecordNotFound` to `momo.ErrNotFound`). Bun guide: the same headings mapped from `bun.DB` calls. Ent guide: the same headings mapped from the generated client, with the `momo import ent` path.
- [ ] **Step 3: Run the check**, expect PASS for the headings check (`grep -c` returns 7 each).
- [ ] **Step 4: Commit** `git commit -m "docs: migration and API guides for GORM, Bun and Ent users"`.

## Task 15: Guide section completeness

Covers: S-95 (each guide has the required sections). The code-block extractor is NOT in this plan: plan momo-13's TestDocsCodeBlocks is the only docs code-block extractor and covers the guide pages. Acceptance: TestMigrationGuidesComplete.

Files:
- Create: `internal/docscheck/guides_test.go`.

Interfaces (exact): test-only package `docscheck`, one test function.

Steps:

- [ ] **Step 1: Failing test**

`internal/docscheck/guides_test.go`:
```go
package docscheck

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

var requiredSections = []string{"## Models", "## Queries", "## Relations", "## Hooks", "## Transactions", "## Migrations", "## Errors"}

func TestMigrationGuidesComplete(t *testing.T) {
    for _, g := range []string{"gorm", "bun", "ent"} {
        b, err := os.ReadFile(filepath.Join("..", "..", "docs", "guides", g+".md"))
        if err != nil {
            t.Fatal(err)
        }
        for _, h := range requiredSections {
            if !strings.Contains(string(b), "\n"+h+"\n") {
                t.Errorf("%s guide missing section %q", g, h)
            }
        }
    }
}
```
Run: `go test ./internal/docscheck/ -run TestMigrationGuidesComplete -count=1`. Expect FAIL with `no such file` before Task 14.

- [ ] **Step 2: Run after Task 14, expect PASS**

Run: `go test ./internal/docscheck/ -run TestMigrationGuidesComplete -count=1`. Expect `ok`.

- [ ] **Step 3: Commit**

`git commit -m "test(docs): migration guides have every required section"`

## Coverage check (planning report)

| Scope id | Tasks |
|---|---|
| S-91 | 1, 2, 3, 4, 5 |
| S-92 | 6, 7 |
| S-93 | 8, 9, 10, 11, 12 |
| S-94 | 13 |
| S-95 | 14, 15 |

## Open items for the lead

- D-11.1: Ent is read statically (no execution). The alternative `entc/load` executes the schema package. Confirm before Task 4.
- Task 7 MySQL subtest is blocked on plan momo-08.
- Goose, golang-migrate, bun and Atlas history layouts are pinned from module-cache source by the implementer, not from web docs (two doc pages failed at planning time).
