Status: draft
Spec: .procoder/specs/momo.md

## Goal

Complete the typed query and write surface of the root `momo` package (joins, projections, aggregates, subqueries, CTEs, keyset pagination, locking, returning, raw SQL, insert/update/delete/upsert with explicit write states, guardrails, soft delete, timestamps, defaults and validators, hooks, interceptors, codecs and the `types` package, JSON/array and window operators, error redaction, resource guards, result helpers, composition helpers, MERGE and bulk update, and the `momovet` analyzer), each proven per dialect against PostgreSQL and SQLite.

## Architecture

Query and mutation builders in the root package render SQL only through `Builder` (identifiers via `Ident`, values via `Arg`); every feature is either rendered for both dialects or returns `ErrUnsupported` at build time from the `dialect.Dialect` capability flags. Write builders run Go-side defaults and validators, then hooks, then compile; statement-level behavior (guardrails, optimistic locks, soft-delete filters) is expressed in SQL so that a guarded update that matches no row changes nothing. `momovet` is a separate `go/analysis` module-internal package under `cmd/momovet` and never links into the runtime.

## Constraints

Copied verbatim from `.procoder/specs/momo.md` (Constraints section) where they bind this plan:

- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- Generated execution path performs no struct-tag inspection, no reflection over models and no name-based field resolution per query.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- Generator and migration tooling dependencies (golang.org/x/tools etc.) stay out of the application runtime import graph.
- Persisted formats carry explicit format versions (not applicable to this plan beyond documentation).
- Dependencies pinned in go.mod/go.sum.

From the spec In-scope and Out-of-scope sections, binding here:

- "no implicit object-graph saves" (S-5): write builders never save relations.
- "Copying conflicting behaviors from GORM/Bun/Ent (zero-value omission, implicit association saves, implicit per-write transactions ...)" is out of scope: zero values are always written.
- "Compile-time validation of arbitrary raw SQL" is out of scope.

Pinned facts (verified from module sources at pinned versions, see momo-03-drivers Constraints): PostgreSQL bind-parameter limit 65535; SQLite 32766 (`dialect.MaxParams`).

PostgreSQL MERGE version floors: the PostgreSQL 17 MERGE documentation page does not state the version that introduced MERGE or MERGE ... RETURNING. This plan gates MERGE and MERGE RETURNING on the server version at runtime (Task 33) instead of hard-coding a floor; the floors are recorded as an open item in the report.

## Interfaces consumed from momo-01-foundation and momo-03-drivers

From momo-03: `testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name))` and `testopen.Postgres`/`testopen.SQLite`. Every feature test in this plan is written against `Each`, so it runs on PostgreSQL in milestone M1 and on SQLite too once momo-03 Task 14 lands. From momo-01: `After` (keyset, including the nullable-key rule), and the root unit fixture `unitDocument` (this plan does not use it).

Owned by momo-01 and not repeated here: the S-4 unit tests `TestQueryReuseNoLeak` (race), `TestPlaceholdersAndQuoting`, `FuzzBuilder`, `TestCompileFail` and `TestValidExamplesCompile`. This plan consumes from momo-01: `Builder` (`Ident`, `Arg`, `WriteString`), `compile`, the `Select` and `SelectQuery` value semantics (copy-on-write slices), `dialect.Dialect` capability flags, `NewColumn` and `NewNullColumn`, `Meta` and `ColumnMeta`, `Compiled`, and the sentinels `ErrValidation`, `ErrLimitExceeded`, `ErrUnsupported`, `ErrUnfilteredMutation`, `ErrOptimisticLock`, `ErrNotFound`, `ErrBusy`.

## Task 1: Integration fixtures, capabilities and the internal/suite package

Files:

- Create: `/Volumes/DATA/Development/momo/internal/suite/fixture.go` (package `suite`, non-test: driver packages and momo-08 import the suite)
- Create: `/Volumes/DATA/Development/momo/internal/suite/caps.go` (package `suite`)
- Create: `/Volumes/DATA/Development/momo/internal/suite/fixture_test.go` (package `suite_test`, smoke test through `testopen.Each`)

No root-package fixture file is created here: the root `momo_test` package keeps its single unit fixture `unitDocument` (momo-01, `unit_fixtures_test.go`). The integration fixtures live in `internal/suite` only.

Interfaces (package `suite`, exact):

```go
// Caps describes what a dialect and driver support. Suite functions assert ErrUnsupported for
// every capability that is false; they never skip.
type Caps struct {
    Dialect             dialect.Name
    Returning           bool
    Locking             bool
    DefaultInUpdate     bool // UPDATE ... SET col = DEFAULT
    DefaultInValues     bool // INSERT ... VALUES (DEFAULT)
    Merge               bool
    WindowFunctions     bool
    JSON                bool // JSONB operators
    Arrays              bool // array operators
    Copy                bool // driver implements momo.Copier
    Batch               bool // driver implements momo.Batcher
    NonDefaultIsolation bool // dialect.IsolationLevels
    Vectors             bool // driver implements momo.CapabilityReporter with Vectors true
}

// CapabilitiesOf returns the dialect-level capabilities. Driver-level fields (Copy, Batch, Vectors)
// stay false; CapsOf adds them from the driver.
func CapabilitiesOf(d *dialect.Dialect) Caps

// CapsOf returns CapabilitiesOf(db.Dialect()) plus the driver-level capabilities of db.Driver():
// Copy from momo.Copier, Batch from momo.Batcher, Vectors from momo.CapabilityReporter.
func CapsOf(db *momo.DB) Caps

// Document is the shared integration fixture model, the struct the generator emits for it.
type Document struct {
    ID        int64
    TenantID  int64
    Title     string
    Body      *string    // nullable
    Version   int64      // optimistic lock
    DeletedAt *time.Time // soft delete (timestamp)
    CreatedAt time.Time
    UpdatedAt time.Time
    Secret    string     // sensitive
}

// Column descriptors, as generated code would expose them.
var (
    DocID        = momo.NewColumn[int64]("documents", "id", momo.FlagPK|momo.FlagIdentity)
    DocTenantID  = momo.NewColumn[int64]("documents", "tenant_id", 0)
    DocTitle     = momo.NewColumn[string]("documents", "title", 0)
    DocBody      = momo.NewNullColumn[string]("documents", "body", momo.FlagNullable)
    DocStatus    = momo.NewNullColumn[string]("documents", "status", momo.FlagNullable|momo.FlagHasDefault)
    DocVersion   = momo.NewColumn[int64]("documents", "version", momo.FlagVersion)
    DocDeletedAt = momo.NewNullColumn[time.Time]("documents", "deleted_at", momo.FlagSoftDelete|momo.FlagNullable)
    DocCreatedAt = momo.NewColumn[time.Time]("documents", "created_at", momo.FlagCreatedAt)
    DocUpdatedAt = momo.NewColumn[time.Time]("documents", "updated_at", momo.FlagUpdatedAt)
    DocSecret    = momo.NewColumn[string]("documents", "secret", momo.FlagSensitive)
)

// DocumentMeta is the package-level Meta for Document. Table "documents"; columns in order
// id, tenant_id, title, body, version, deleted_at, created_at, updated_at, secret.
// PK []int{0}, Identity 0, Version 4, SoftDelete 5, CreatedAt 6, UpdatedAt 7.
var DocumentMeta *momo.Meta[Document]

// MomoMeta returns DocumentMeta.
func (*Document) MomoMeta() *momo.Meta[Document] { return DocumentMeta }

// CreateDocuments drops and recreates the documents table with the DDL of the dialect in caps.
func CreateDocuments(t testing.TB, db *momo.DB, caps Caps)
```

DDL per dialect (run through `db.Driver().Exec`, `DROP TABLE IF EXISTS documents` first):

- PostgreSQL: `CREATE TABLE documents (id bigint generated always as identity primary key, tenant_id bigint not null, title text not null, body text, status text default 'draft', version bigint not null default 1, deleted_at timestamptz, created_at timestamptz not null default now(), updated_at timestamptz not null default now(), secret text not null default '')`
- SQLite: `CREATE TABLE documents (id integer primary key, tenant_id integer not null, title text not null, body text, status text default 'draft', version integer not null default 1, deleted_at text, created_at text not null default (strftime('%Y-%m-%d %H:%M:%f','now')), updated_at text not null default (strftime('%Y-%m-%d %H:%M:%f','now')), secret text not null default '')`

Dialect flags used by `CapabilitiesOf`: `Returning`, `Locking`, `DefaultInUpdate`, `DefaultInValues`, `Merge`, `WindowFunctions`, `IsolationLevels` (exist in momo-01's dialect), and `JSON`, `Arrays` (added to `dialect.Dialect` by Task 24 of this plan, the contract addition recorded in the report).

Placement and wrapper rule for every feature test (binding for Tasks 2–35): the test body is written as the closure of `testopen.Each`, using `db *momo.DB` and `d dialect.Name`. The body moves into `internal/suite/<area>.go` as `func <Name>(t *testing.T, db *momo.DB, caps Caps)`, where `d` becomes `caps.Dialect` and `suite.CapsOf(db)` becomes `caps`. The `momo_test` file keeps only `func TestName(t *testing.T) { testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.Name(t, db, suite.CapsOf(db)) }) }`. Every suite function begins by calling `CreateDocuments`, so each is independent of the others.

- [ ] **Step 1: Write the failing test** `internal/suite/fixture_test.go`:

```go
package suite_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
    "github.com/azrtydxb/momo/internal/suite"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHarnessCreatesDocuments(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'probe')"); err != nil {
            t.Fatal(err)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test ./internal/suite/ -run TestHarnessCreatesDocuments` → `no required module provides package github.com/azrtydxb/momo/internal/suite`.

- [ ] **Step 3: Implement** `fixture.go` and `caps.go` per Interfaces. `DocumentMeta` is built with `Ptr` and `Value` switches over the nine columns (`Value` returns `m.Body` and `m.DeletedAt` as they are). `CapabilitiesOf` maps the dialect flags one to one. `CapsOf` uses comma-ok assertions on `db.Driver()`.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test ./internal/suite/ -run TestHarnessCreatesDocuments -race` → `ok` on PostgreSQL (SQLite subtest runs once momo-03's SQLite entry exists).

- [ ] **Step 5: Commit.** `test(core): integration fixtures, capabilities and the internal/suite package`

## Task 2: Transactions, error classification and the Scalars helper (S-7, S-8)

Files:

- Create: `/Volumes/DATA/Development/momo/internal/suite/transactions.go` (package `suite`)
- Create: `/Volumes/DATA/Development/momo/internal/suite/errors.go` (package `suite`)
- Create: `/Volumes/DATA/Development/momo/scalars.go` (package `momo`)
- Create: `/Volumes/DATA/Development/momo/transactions_test.go`, `/Volumes/DATA/Development/momo/errors_test.go`, `/Volumes/DATA/Development/momo/scalars_test.go` (package `momo_test` wrappers)

Interfaces:

```go
// package momo (root)
// Scalars renders sql through the raw path (appendRaw: ? binds, ?? literal) and returns the first column of every row as V.
func Scalars[V any](ctx context.Context, q Querier, sql string, args ...any) ([]V, error)

// package suite
func Transactions(t *testing.T, db *momo.DB, caps Caps)
func Errors(t *testing.T, db *momo.DB, caps Caps)
```

`Scalars` renders its SQL exactly as `momo.Exec` does: `compile(q's dialect, func(b *Builder) { b.appendRaw(sql, args) })`. Callers write `?` placeholders on every dialect (the dialect turns each into `$n` on PostgreSQL and keeps `?` on SQLite), `??` is a literal `?`, `Expr` arguments render inline, and an argument-count mismatch is a build error. It then runs the compiled query through `q.conn().query(ctx, "select", "", c)` and scans each row's first column into a `V`. An empty result returns a nil slice and a nil error.

`Transactions` body (behaviour required by S-7): commit on nil return; rollback on error return; a panic inside the callback rolls back, re-raises, and leaves no row; a nested `tx.Tx` savepoint that fails rolls back only its own work; isolation: when `caps.NonDefaultIsolation`, `SHOW transaction_isolation` inside `sql.LevelRepeatableRead` returns `repeatable read`; otherwise `sql.LevelReadUncommitted` returns `momo.ErrUnsupported`.

`Errors` body (behaviour required by S-8, driver-independent): a unique index on `documents(title)` makes a duplicate insert satisfy `errors.Is(err, momo.ErrUniqueViolation)` and `errors.As(err, &me)` with `me.Table == "documents"`; `INSERT ... (title) VALUES (NULL)` satisfies `ErrNotNullViolation`; a table `checked (x integer check (x > 0))` with `x = 0` satisfies `ErrCheckViolation`; an orphan row in `child (parent_id ... references parent(id))` satisfies `ErrForeignKeyViolation`; every classified error has a non-nil `errors.Unwrap` (the driver cause is reachable). The string `CHECK` and `FOREIGN KEY` DDL is written identically for both dialects.

- [ ] **Step 1: Write the failing tests.** `transactions_test.go` and `errors_test.go` (package `momo_test`):

```go
// transactions_test.go
package momo_test

import (
    "testing"

    "github.com/azrtydxb/momo/internal/suite"
)

func TestTransactions(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.Transactions(t, db, suite.CapsOf(db)) })
}
```

```go
// errors_test.go
package momo_test

import (
    "testing"

    "github.com/azrtydxb/momo/internal/suite"
)

func TestErrorClassification(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.Errors(t, db, suite.CapsOf(db)) })
}
```

```go
// scalars_test.go (PostgreSQL only: ::int is PostgreSQL cast syntax)
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestScalarsRawPlaceholders(t *testing.T) {
    db := testopen.Postgres(t)
    got, err := momo.Scalars[int](context.Background(), db, "SELECT ?::int + ?", 1, 2)
    if err != nil || len(got) != 1 || got[0] != 3 {
        t.Fatalf("Scalars = %v, %v; want [3], nil", got, err)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test -run 'TestTransactions|TestErrorClassification|TestScalarsRawPlaceholders' .` → `undefined: suite.Transactions` (and `undefined: momo.Scalars` until Step 3).

- [ ] **Step 3: Implement** `scalars.go`, `internal/suite/transactions.go` and `internal/suite/errors.go` with the bodies described above. Use `db.Tx`, `tx.Tx`, `momo.Exec`, `momo.Scalars`, `momo.Select`. Use `sql.LevelReadUncommitted` and `sql.LevelRepeatableRead` from `database/sql`.

- [ ] **Step 4: Mutation check.** In `Transactions`, change the panic subtest to not re-raise (`recover()` swallowed inside `Tx`), run `go test -run TestTransactions .`. Expected FAIL `panic was swallowed`. Restore.

- [ ] **Step 5: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test -run 'TestTransactions|TestErrorClassification|TestScalarsRawPlaceholders' -race .` → `ok`.

- [ ] **Step 6: Commit.** `test(core): shared transaction and error-classification suites`

## Task 3: Safe names and LIKE escaping (S-43)

Files:

- Create: `/Volumes/DATA/Development/momo/allowlist.go`
- Create: `/Volumes/DATA/Development/momo/allowlist_test.go`

Interfaces:

```go
// Allowlist maps external names to typed column descriptors. Unknown names are an error.
type Allowlist struct{ m map[string]ColumnRef }

// NewAllowlist builds an allowlist keyed by ColumnName().
func NewAllowlist(cols ...ColumnRef) Allowlist

// Column returns the descriptor for name, or a *Error with Kind ErrValidation.
func (a Allowlist) Column(name string) (ColumnRef, error)

// EscapeLike escapes %, _ and the escape character ! (prefixing each with !) so the result matches literally inside a LIKE pattern
// that is rendered with ESCAPE '!' on every dialect (no backslash, so server settings cannot change its meaning).
func EscapeLike(s string) string

// Contains returns a predicate matching rows whose column contains s literally.
func (c Column[T]) Contains(s string) Predicate  // only for string columns: see Task 3 note
```

Note on `Contains`: Go generics do not allow a method only on `Column[string]`; implement `func ContainsLiteral(c Expression[string], s string) Predicate` and render `c LIKE ? ESCAPE '!'` with `EscapeLike(s)` bound as `%` + escaped + `%`. `Like` and `ILike` from the spike stay as they are.

- [ ] **Step 1: Write the failing test** `allowlist_test.go`:

```go
package momo_test

import (
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestColumnByName(t *testing.T) {
    al := momo.NewAllowlist(DocTitle, DocTenantID)
    got, err := al.Column("title")
    if err != nil || got.ColumnName() != "title" {
        t.Fatalf("known name: %v %v", got, err)
    }
    if _, err := al.Column("title; DROP TABLE documents"); !errors.Is(err, momo.ErrValidation) {
        t.Fatalf("unknown name err = %v, want ErrValidation", err)
    }
}

func TestEscapeLike(t *testing.T) {
    if got := momo.EscapeLike(`50%_off!`); got != `50!%!_off!!` {
        t.Fatalf("EscapeLike = %q", got)
    }
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := t.Context()
        for _, title := range []string{"50%_off", "50 off", `50\off`} {
            if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", title); err != nil {
                t.Fatal(err)
            }
        }
        rows, err := momo.Select[Document](db).Where(momo.ContainsLiteral(DocTitle, "%_")).All(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if len(rows) != 1 || rows[0].Title != "50%_off" {
            t.Fatalf("literal match returned %d rows", len(rows))
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run 'TestColumnByName|TestEscapeLike' .` → `undefined: momo.NewAllowlist`.

- [ ] **Step 3: Implement** `allowlist.go` per Interfaces. `ColumnRef` (spike) already provides `ColumnName()`; `ContainsLiteral` takes `Expression[string]`, so `DocTitle` (a `Column[string]`) satisfies it.

- [ ] **Step 4: Run and expect PASS.** `go test -run 'TestColumnByName|TestEscapeLike' -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): name allowlists and literal LIKE escaping`

## Task 4: Joins, aliases and self-joins

Files:

- Create: `/Volumes/DATA/Development/momo/integration_joins_test.go`
- Modify: `/Volumes/DATA/Development/momo/select.go` only if the join test exposes a rendering gap.

Interfaces (existing): `Join(src TableSource, on ...Predicate)`, `LeftJoin`, `CrossJoin`, `Column.As(alias)`, `TableRef{Name, Alias}`.

Test data: documents with tenant_id 1 and 2, and a `parent_id`-free self-join using `DocTenantID` on two aliases.

- [ ] **Step 1: Write the failing test** `integration_joins_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestJoinsAliasesSelfJoin(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        for _, row := range []struct{ tenant int64; title string }{{1, "a"}, {1, "b"}, {2, "c"}} {
            if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", row.tenant, row.title); err != nil {
                t.Fatal(err)
            }
        }
        left := momo.TableRef{Name: "documents", Alias: "l"}
        right := momo.TableRef{Name: "documents", Alias: "r"}
        type pair struct{ L, R string }
        rows, err := momo.Select[pairRow](db).
            From(left).
            Columns(DocTitle.As("l").AsExpr(), DocTitle.As("r").AsExpr()).
            Join(right, DocTenantID.As("l").EqExpr(DocTenantID.As("r")), DocID.As("l").NeExpr(DocID.As("r"))).
            OrderBy(DocTitle.As("l").Asc(), DocTitle.As("r").Asc()).
            All(ctx)
        if err != nil {
            t.Fatal(err)
        }
        want := []pair{{"a", "b"}, {"b", "a"}}
        if len(rows) != len(want) || rows[0] != want[0] || rows[1] != want[1] {
            t.Fatalf("self-join rows = %+v, want %+v", rows, want)
        }
    })
}
```

`pairRow` is a projection type defined in the test with `MomoMeta()` returning a `Meta` with Columns `[{Name:"l_title"},{Name:"r_title"}]`, `Ptr` for fields `L` and `R`, and the `Projection` marker embedded. `AsExpr()` is a new one-line method: `func (c Column[T]) AsExpr() Expression[T]` returning the column itself (a `Column[T]` already implements `Expression[T]`; the method exists so the projection list reads explicitly). Add it in this task.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestJoinsAliasesSelfJoin .` → `undefined: pairRow` before the test type is added; after adding it, the expected failure is `undefined: AsExpr`.

- [ ] **Step 3: Implement** `AsExpr` in `expr.go` and fix any rendering gap in `select.go` so the join qualifiers render as the alias (`"l"."title"`).

- [ ] **Step 4: Run and expect PASS.** `go test -run TestJoinsAliasesSelfJoin -race .` → `ok` on both dialects.

- [ ] **Step 5: Commit.** `test(core): aliased and self joins on both dialects`

## Task 5: Projections, aggregates, GROUP BY and HAVING

Files:

- Create: `/Volumes/DATA/Development/momo/integration_aggregates_test.go`
- Modify: `/Volumes/DATA/Development/momo/expr.go` (adds `Count`, `Sum`, `Avg`, `Min`, `Max` typed expressions)

Interfaces:

```go
func CountAll() Expression[int64]
func Sum[T Number](e Expression[T]) Expression[T]     // Number: ~int64 | ~float64 (spec-listed numeric aggregates)
func Avg(e Expression[float64]) Expression[float64]
func Min[T Ordered](e Expression[T]) Expression[T]
func Max[T Ordered](e Expression[T]) Expression[T]
```

`Ordered` is `cmp.Ordered`. Typed projection row: `tenantCount{TenantID int64; N int64}` with `Projection` marker.

- [ ] **Step 1: Write the failing test** `integration_aggregates_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

type tenantCount struct {
    momo.Projection
    TenantID int64
    N        int64
}

func (*tenantCount) MomoMeta() *momo.Meta[tenantCount] {
    return &momo.Meta[tenantCount]{
        Columns: []momo.ColumnMeta{{Name: "tenant_id"}, {Name: "n"}},
        Ptr: func(m *tenantCount, i int) any {
            if i == 0 {
                return &m.TenantID
            }
            return &m.N
        },
        Value: func(m *tenantCount, i int) any { return nil },
    }
}

func TestAggregatesGroupHaving(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        for _, tenant := range []int64{1, 1, 1, 2} {
            if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (?, 'x')", tenant); err != nil {
                t.Fatal(err)
            }
        }
        rows, err := momo.Select[tenantCount](db).
            Columns(DocTenantID, momo.CountAll()).
            GroupBy(DocTenantID).
            Having(momo.CountAll().Gt(1)).
            OrderBy(DocTenantID.Asc()).
            All(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if len(rows) != 1 || rows[0].TenantID != 1 || rows[0].N != 3 {
            t.Fatalf("rows = %+v, want tenant 1 with 3 documents", rows)
        }
    })
}
```

Note: the `Having` call below requires `Expression[int64].Gt`; add the comparison methods to the generic `Expression` helpers in this task (a `Cmp` function, signature below, or methods on the aggregate type). Pick: aggregates return `Agg[T]` with methods `Gt(v T) Predicate`, `Ge`, `Lt`, `Le`, `Eq`, which the test uses.

```go
Having(momo.CountAll().Gt(1))

func Cmp[T](e Expression[T]) ...
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestAggregatesGroupHaving .` → `undefined: momo.CountAll`.

- [ ] **Step 3: Implement** the aggregates in `expr.go` per Interfaces, with `Agg[T]` implementing `Expression[T]` and comparison methods. Rendering: `COUNT(*)`, `SUM(col)`, etc., identifiers via `Ident`.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestAggregatesGroupHaving -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): typed aggregates with GROUP BY and HAVING`

## Task 6: Subqueries, CTEs (including recursive) and set operations

Files:

- Create: `/Volumes/DATA/Development/momo/integration_subquery_test.go`

Interfaces (existing): `Column[T].InQuery(q Subquery)`, `SelectQuery.With(name, q, cols...)`, `WithRecursive`, `Union(all, qs...)`, `FromSubquery(q, alias)`.

- [ ] **Step 1: Write the failing test:**

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestSubqueriesCTEsRecursive(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        for _, tenant := range []int64{1, 2, 3} {
            if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (?, 'x')", tenant); err != nil {
                t.Fatal(err)
            }
        }
        sub := momo.Select[Document](db).Columns(DocTenantID).Where(DocTenantID.Ge(2))
        n, err := momo.Select[Document](db).Where(DocTenantID.InQuery(sub)).Count(ctx)
        if err != nil || n != 2 {
            t.Fatalf("IN subquery count = %d, %v; want 2", n, err)
        }
        seq := momo.Subquery(`SELECT 1 AS n UNION ALL SELECT n + 1 FROM seq WHERE n < 5`)
        var ints []int64 // single-column projection via momo.Scalars
        ints, err = momo.Scalars[int64](ctx, db, momo.WithRecursive("seq", seq, "n")(`SELECT n FROM seq ORDER BY n`))
        if err != nil {
            t.Fatal(err)
        }
        if len(ints) != 5 || ints[4] != 5 {
            t.Fatalf("recursive CTE returned %v", ints)
        }
    })
}
```

`momo.Subquery(sql)` is a raw `Subquery` whose `AppendSQL` writes the fragment through `appendRaw`; `momo.WithRecursive(name, q, cols...)` returns a function that prefixes `WITH RECURSIVE name(cols) AS (...)` to the given `SELECT`; `momo.Scalars[T]` (introduced in Task 2) is the scan used by the assertion above.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestSubqueriesCTEsRecursive .` → `undefined: momo.Subquery`.

- [ ] **Step 3: Implement** `Subquery` (a raw `Subquery` whose `AppendSQL` writes the fragment through `appendRaw`), `WithRecursive` as a curried helper, and `Scalars[T]`. Recursive CTE rendering: `WITH RECURSIVE seq(n) AS (...)` on both dialects (`dialect.RecursiveCTE` is true for both).

- [ ] **Step 4: Run and expect PASS.** `go test -run TestSubqueriesCTEsRecursive -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): subqueries, recursive CTEs and scalar scans`

## Task 7: Keyset pagination integration tests (consumes momo-01 After)

Files:

- Create: `/Volumes/DATA/Development/momo/internal/suite/keyset.go` (package `suite`, body `KeysetPaginationMixedDirections`)
- Create: `/Volumes/DATA/Development/momo/integration_keyset_test.go` (package `momo_test`, wrapper)

`After` is implemented by momo-01 (`select.go`), including the rule that a nullable sort key returns `ErrUnsupported`. This task adds only integration tests that run `After` against both dialects, so no keyset rendering rule is defined here.

Interfaces (consumed from momo-01, not defined here): `func (sq SelectQuery[T]) After(values ...any) SelectQuery[T]`, where `values` are the sort-key values in `OrderBy` order followed by the primary key.

Package `suite` exports `func KeysetPaginationMixedDirections(t *testing.T, db *momo.DB, caps Caps)` and `func KeysetRejectsNullableKey(t *testing.T, db *momo.DB, caps Caps)`.

- [ ] **Step 1: Write the failing tests.** `integration_keyset_test.go`:

```go
package momo_test

import (
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
    "github.com/azrtydxb/momo/internal/suite"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestKeysetPaginationMixedDirections(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.KeysetPaginationMixedDirections(t, db, suite.CapsOf(db))
    })
}

func TestKeysetRejectsNullableKey(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.KeysetRejectsNullableKey(t, db, suite.CapsOf(db))
    })
}
```

Body of `KeysetPaginationMixedDirections` (`internal/suite/keyset.go`): call `CreateDocuments`; insert ten documents with `momo.Exec` (`tenant_id = i%3`, `version = i%2+1`), every error checked; walk pages of three with `OrderBy(DocVersion.Desc(), DocTenantID.Asc()).Limit(3)` and `After(last.Version, last.TenantID, last.ID)`; fail on a repeated ID, on an error from `All`, or unless all ten rows and at least four pages were seen.

Body of `KeysetRejectsNullableKey`: the query below must return `momo.ErrUnsupported`, checked with `errors.Is`, and the same query over `DocTitle` (non-nullable) must build without error.

```go
momo.Select[Document](db).OrderBy(DocBody.Asc()).After("x", int64(1)).SQL()
```

- [ ] **Step 2: Run and expect FAIL** if momo-01's `After` is absent: `MOMO_TEST_POSTGRES_URL=... go test -run 'Keyset' .` → `undefined: After` or `row N returned twice`. With momo-01 complete the expected result is PASS.

- [ ] **Step 3: Fix defects found by these tests in momo-01's `After`** (in `select.go`), not by adding rendering rules here.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test -run 'Keyset' -race .` → `ok` on PostgreSQL.

- [ ] **Step 5: Commit.** `test(core): keyset integration tests over PostgreSQL`

## Task 8: Locking, optimizer hints and runtime table override (S-79)

Files:

- Create: `/Volumes/DATA/Development/momo/integration_locking_test.go`
- Modify: `/Volumes/DATA/Development/momo/select.go` (adds `IndexedBy`, `WithTableName`)

Interfaces:

```go
// IndexedBy renders SQLite INDEXED BY for the FROM table; PostgreSQL returns ErrUnsupported at build time.
func (sq SelectQuery[T]) IndexedBy(index string) SelectQuery[T]

// WithTableName replaces the FROM table name at runtime. The name is quoted; it is never spliced.
func (sq SelectQuery[T]) WithTableName(name string) SelectQuery[T]
```

Lock rules (existing `ForUpdate(opt ...string)`): `NOWAIT` and `SKIP LOCKED` are valid options; SQLite has no row locks, so `ForUpdate` on SQLite returns `ErrUnsupported` at build time (`dialect.Locking` is false for SQLite).

- [ ] **Step 1: Write the failing test** `integration_locking_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestLockingNowaitSkipLocked(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'x')"); err != nil {
            t.Fatal(err)
        }
        q := momo.Select[Document](db).Where(DocTenantID.Eq(1)).ForUpdate("SKIP LOCKED")
        _, err := q.SQL()
        if d == dialect.SQLite {
            if !errors.Is(err, momo.ErrUnsupported) {
                t.Fatalf("sqlite ForUpdate err = %v, want ErrUnsupported", err)
            }
            return
        }
        if err != nil {
            t.Fatal(err)
        }
        tx, err := db.Begin(ctx, momo.TxOptions{})
        if err != nil {
            t.Fatal(err)
        }
        defer tx.Rollback(ctx)
        if _, err := q.All(ctx); err != nil {
            t.Fatal(err)
        }
    })
}

func TestWithTableNameQuoted(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        _, err := momo.Select[Document](db).WithTableName(`documents"; DROP TABLE documents; --`).SQL()
        if err != nil {
            t.Fatal(err)
        }
        c, err := momo.Select[Document](db).WithTableName(`x"y`).SQL()
        if err != nil {
            t.Fatal(err)
        }
        if !strings.Contains(c.SQL, `"x""y"`) {
            t.Fatalf("table name not quoted: %s", c.SQL)
        }
    })
}
```

Imports for the file: add `"strings"` and `"github.com/azrtydxb/momo/dialect"`.

The following test is added to the same file (it uses the same imports plus `strings` and `dialect`):

```go
func TestHintsByDialect(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        c, err := momo.Select[Document](db).IndexedBy(`idx"x`).SQL()
        if d == dialect.SQLite {
            if err != nil || !strings.Contains(c.SQL, `INDEXED BY "idx""x"`) {
                t.Fatalf("sqlite IndexedBy: %v %s", err, c.SQL)
            }
            return
        }
        if !errors.Is(err, momo.ErrUnsupported) {
            t.Fatalf("postgres IndexedBy err = %v, want ErrUnsupported", err)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run 'TestLockingNowaitSkipLocked|TestWithTableNameQuoted' .` → `undefined: WithTableName`.

- [ ] **Step 3: Implement** `IndexedBy` (renders `INDEXED BY "name"` after the FROM table on SQLite; build error `ErrUnsupported` on PostgreSQL) and `WithTableName` (replaces `meta.Table` for the FROM target and the column qualifier; always `Ident`-quoted). The lock check uses `dialect.Locking`.

- [ ] **Step 4: Run and expect PASS.** `go test -run 'TestLockingNowaitSkipLocked|TestWithTableNameQuoted' -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): locking options, indexed-by hint and quoted table override`

## Task 9: RETURNING on insert, update and delete

Files:

- Create: `/Volumes/DATA/Development/momo/returning_test.go`
- (The builders themselves are created in Tasks 15, 18 and 19; this task adds the `Returning` method shared by them as `returning.go`.)

Interfaces:

```go
// Returning appends a RETURNING list. On SQLite it requires library >= 3.35 (checked on connect);
// dialect.Returning is true for both dialects.
type ReturningList struct{ cols []ColumnRef }

func Returning(cols ...ColumnRef) ReturningList
```

`InsertQuery[T].ReturningAll()` etc. are defined in Task 12; this task defines `ReturningList` and `appendReturning(b *Builder, cols []ColumnRef)` (renders `RETURNING "c1", "c2"` unqualified).

- [ ] **Step 1: Write the failing test** `returning_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestReturningRendersAndScans(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        rows, err := momo.Insert[Document](db).
            Values(DocTenantID.Set(7), DocTitle.Set("r")).
            Returning(DocID, DocTitle).
            Rows(ctx)
        if err != nil {
            t.Fatal(err)
        }
        var doc Document
        if !rows.Next() {
            t.Fatal("no RETURNING row")
        }
        if err := rows.ScanModel(&doc); err != nil {
            t.Fatal(err)
        }
        if err := rows.Close(); err != nil {
            t.Fatal(err)
        }
        if doc.ID == 0 || doc.Title != "r" {
            t.Fatalf("returned %+v", doc)
        }
    })
}
```

`Rows(ctx)` returns a `*momo.ModelRows[T]` with `Next() bool` and `ScanModel(*T) error` (added in Task 12 as part of the insert builder); this task adds `ModelRows` and the `Rows` terminal to the insert builder shape required by this test.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestReturningRendersAndScans .` → `undefined: momo.Insert`.

- [ ] **Step 3: Implement** `returning.go`: `Returning(cols ...ColumnRef) ReturningList`, `appendReturning`, and `ModelRows[T]` wrapping `Rows` with `ScanModel` using `Meta.Ptr`. Insert, Update and Delete builders (Tasks 15, 18, 19) call `appendReturning`.

- [ ] **Step 4: Run and expect PASS** after Task 12 lands: `go test -run TestReturningRendersAndScans -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): RETURNING rows for writes`

## Task 10: Raw SQL arguments: named, expanded IN and identifiers (S-77)

Files:

- Create: `/Volumes/DATA/Development/momo/raw_args.go`
- Create: `/Volumes/DATA/Development/momo/raw_args_test.go`

Interfaces:

```go
// Named binds a named argument for @name placeholders in raw SQL.
type Named struct{ Name string; Value any }
func Named(name string, value any) Named // name is the constructor; see note below

// In expands a slice into a parenthesis-free list of bind markers; an empty slice renders NULL-safe false.
type inArg struct{ values any } // unexported; created by In
func In(values any) any // accepts any slice; panics are not used: non-slices yield an error at build time

// Ident renders a quoted identifier argument (never bound as a value).
type identArg struct{ name string }
func Ident(name string) any

// TypedSQL[T] is a raw fragment usable where Expression[T] is expected.
func TypedSQL[T any](fragment string, args ...any) Expression[T]
```

Naming: the value type is `NamedArg{Name, Value}` and the constructor is `Named(name string, value any) NamedArg`; the contract's `momo.Named` is the constructor.

Rules in `appendRaw`: `?` binds a positional value, `??` is a literal `?`, `@name` looks up a `NamedArg` by name (missing name is a build error), an `inArg` expands to `(?, ?, ...)` with one bind per element. An empty `inArg` is a build error for both `IN` and `NOT IN`: `appendRaw` calls `b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("empty IN list in raw SQL; use the typed In which renders a constant")})`. `NULL` makes `NOT IN` never true, so an empty raw list would silently return no rows where all rows are expected; an error is the only honest result. The typed `Column.In` and `Column.NotIn` keep their constant rendering (`FALSE` for an empty IN, `TRUE` for an empty NOT IN; Task 11), and `identArg` renders `Ident`.

- [ ] **Step 1: Write the failing test** `raw_args_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestRawArgs(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        for _, title := range []string{"a", "b", "c"} {
            if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", title); err != nil {
                t.Fatal(err)
            }
        }
        n, err := momo.Exec(ctx, db,
            "UPDATE documents SET secret = @s WHERE title IN (?)",
            momo.Named("s", "x"), momo.In([]string{"a", "b"}))
        if err != nil || n != 2 {
            t.Fatalf("named+In update n=%d err=%v", n, err)
        }
        rows, err := momo.Select[Document](db).
            Where(momo.TypedSQL[bool](`? = ?`, momo.Ident("title"), "a")).
            All(ctx)
        if err != nil || len(rows) != 0 {
            t.Fatalf("identifier-as-value must not match a value: rows=%d err=%v", len(rows), err)
        }
    })
}
```

The second assertion has a fixed expectation: `? = ?` with `Ident("title")` renders `"title" = ?` and binds `"a"`, which compares the identifier as a column reference and matches no row when the value 'a' is not in the title column of the same row... Replace the second part in step 3 with the exact assertion that `Ident` renders `"title"` into SQL text and never appears in Args: `c.Args` must equal `[]any{"a"}`.

The empty raw-IN rule is a unit test in package `momo` (no database), added to the same step:

```go
// raw_empty_in_test.go (package momo)
package momo

import (
    "errors"
    "strings"
    "testing"

    "github.com/azrtydxb/momo/dialect"
)

func TestRawEmptyInIsBuildError(t *testing.T) {
    for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
        for _, sql := range []string{"x IN (?)", "x NOT IN (?)"} {
            _, err := compile(dialect.For(d), func(b *Builder) {
                b.appendRaw(sql, []any{In([]string{})})
            })
            var me *Error
            if !errors.As(err, &me) || me.Kind != ErrValidation || me.Op != "build" ||
                !strings.Contains(me.Error(), "empty IN list in raw SQL") {
                t.Fatalf("%s on %s: err = %v, want a build-time validation error", sql, d, err)
            }
        }
    }
}

func TestRawNonEmptyInBinds(t *testing.T) {
    c, err := compile(dialect.For(dialect.Postgres), func(b *Builder) {
        b.appendRaw("x NOT IN (?)", []any{In([]string{"a", "b"})})
    })
    if err != nil {
        t.Fatal(err)
    }
    if c.SQL != "x NOT IN ($1, $2)" || len(c.Args) != 2 {
        t.Fatalf("SQL = %q args = %v", c.SQL, c.Args)
    }
}
```

- [ ] **Step 1b: Run and expect FAIL.** `go test -run 'TestRawEmptyInIsBuildError|TestRawNonEmptyInBinds' .` → the empty cases currently produce `(NULL)` with no error, so `TestRawEmptyInIsBuildError` fails with `x IN (?) on postgres: err = <nil>`.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestRawArgs .` → `undefined: momo.Named`.

- [ ] **Step 3: Implement** `raw_args.go` and extend `appendRaw` in `builder.go` per the rules above. The identifier assertion in the test is checked by compiling the query: the snippet below followed by `if err != nil { t.Fatal(err) }` must satisfy `strings.Contains(c.SQL, `"title"`)` and `len(c.Args) == 1 && c.Args[0] == "a"`; add that check to the test file together with the `strings` import.

```go
c, err := momo.Select[Document](db).Where(momo.TypedSQL[bool]("? = ?", momo.Ident("title"), "a")).SQL()
```

- [ ] **Step 4: Run and expect PASS.** `go test -run TestRawArgs -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): named arguments, IN expansion and identifier arguments`

## Task 11: Fuzzed raw arguments and typed IN and empty results (S-77, S-8)

Files:

- Create: `/Volumes/DATA/Development/momo/raw_args_fuzz_test.go`
- Create: `/Volumes/DATA/Development/momo/empty_result_test.go`
- Modify: `/Volumes/DATA/Development/momo/expr.go` (typed `In` renders `FALSE` for an empty list)

- [ ] **Step 1: Write the failing tests:**

```go
// raw_args_fuzz_test.go
package momo

import (
    "strings"
    "testing"

    "github.com/azrtydxb/momo/dialect"
)

func FuzzRawArgs(f *testing.F) {
    f.Add("a")
    f.Add(`x"y`)
    f.Fuzz(func(t *testing.T, s string) {
        for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
            c, err := compile(dialect.For(d), func(b *Builder) {
                b.appendRaw("SELECT ?, @n FROM t WHERE x IN (?)", []any{s, NamedArg{Name: "n", Value: s}, In([]string{s})})
            })
            if err != nil {
                t.Fatal(err)
            }
            if strings.Contains(c.SQL, s) && s != "" && !strings.ContainsAny(s, `"`) {
                t.Fatalf("value interpolated into SQL text: %q", c.SQL)
            }
        }
    })
}
```

```go
// empty_result_test.go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
    "github.com/azrtydxb/momo/internal/suite"
    "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestEmptyResultNotNotFoundIntegration(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        rows, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.Eq(99)).All(ctx)
        if err != nil || rows != nil {
            t.Fatalf("All on zero rows = %v, %v; want nil, nil", rows, err)
        }
        empty, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.In()).All(ctx)
        if err != nil || len(empty) != 0 {
            t.Fatalf("empty IN list: %v %v", empty, err)
        }
        if _, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.Eq(99)).One(ctx); !errors.Is(err, momo.ErrNotFound) {
            t.Fatalf("One on zero rows = %v, want ErrNotFound", err)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test -run 'FuzzRawArgs|TestEmptyResultNotNotFoundIntegration' .` (the fuzz seed corpus runs as a unit test; `TestEmptyResultNotNotFoundIntegration` runs on each dialect that has a URL, and skips the dialects without one) → `undefined: NamedArg` or `FALSE` absent from the empty-IN rendering.

- [ ] **Step 3: Implement** the typed `In` change: `inList.AppendSQL` renders `FALSE` when `len(vs) == 0` (and `TRUE` for `NotIn` with an empty list), so `IN ()` is never emitted.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test -run 'FuzzRawArgs|TestEmptyResultNotNotFoundIntegration' -race .` and `go test -run=^$ -fuzz=FuzzRawArgs -fuzztime=10s .` (the fuzz run needs no database) → `ok`.

- [ ] **Step 5: Commit.** `test(core): fuzz raw arguments; empty IN and empty results`

## Task 12: Insert builder: single, model, bulk and conflict handling — S-5

Files:

- Modify: `/Volumes/DATA/Development/momo/insert.go`
- Create: `/Volumes/DATA/Development/momo/insert_test.go`

Interfaces:

```go
func (q InsertQuery[T]) Model(m *T) InsertQuery[T]
func (q InsertQuery[T]) Models(ms []T) InsertQuery[T]
func (q InsertQuery[T]) Omit(cols ...ColumnRef) InsertQuery[T]
func (q InsertQuery[T]) OnConflict(cols ...ColumnRef) ConflictQuery[T]
func (c ConflictQuery[T]) DoNothing() InsertQuery[T]
func (c ConflictQuery[T]) DoUpdate(assigns ...Assignment) InsertQuery[T]
func (q InsertQuery[T]) Returning(cols ...ColumnRef) InsertQuery[T]
func (q InsertQuery[T]) Rows(ctx context.Context) (*ModelRows[T], error)
func (q InsertQuery[T]) Exec(ctx context.Context) (Result, error)
type Result struct{ RowsAffected int64 }
```

Model insert: writes every column except identity, columns in `Omit`, and `FlagReadOnly` columns. `DoUpdate` with `SetExcluded()` renders the excluded row (`EXCLUDED.col` on both dialects; SQLite supports it).

- [ ] **Step 1: Write the failing test** `insert_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestInsertModelOmitAndConflict(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        d := Document{TenantID: 3, Title: "m", Secret: "s"}
        if _, err := momo.Insert[Document](db).Model(&d).Omit(DocBody).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        res, err := momo.Insert[Document](db).
            Values(DocTenantID.Set(3), DocTitle.Set("m")).
            OnConflict(DocID).DoNothing().Exec(ctx)
        if err != nil || res.RowsAffected != 1 {
            t.Fatalf("insert on conflict do nothing with new row: %+v %v", res, err)
        }
        got, err := momo.Select[Document](db).Where(DocTitle.Eq("m")).Count(ctx)
        if err != nil || got != 2 {
            t.Fatalf("count = %d, %v", got, err)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestInsertModelOmitAndConflict .` → `q.Model undefined` (method missing).

- [ ] **Step 3: Implement** the methods above and the rendering: `INSERT INTO t (cols) VALUES (...) ON CONFLICT (pk cols) DO NOTHING` (SQLite and PostgreSQL share this form); `DoUpdate` renders `DO UPDATE SET c = EXCLUDED.c`.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestInsertModelOmitAndConflict -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): model and conflict-aware insert builder`

## Task 13: Bulk insert chunking under the bind-parameter limit — S-5

Files:

- Create: `/Volumes/DATA/Development/momo/bulk_test.go`
- Modify: `/Volumes/DATA/Development/momo/insert.go` (adds chunking in `Models`)

Rule: `Models(ms)` renders multiple `VALUES` tuples per statement such that `rows_per_statement * cols <= dialect.MaxParams`; each chunk is a separate statement executed in order, all within one transaction when the caller passes a `*Tx`, and the RowsAffected total is the sum.

- [ ] **Step 1: Write the failing test** `bulk_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestBulkInsertChunking(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        const n = 40000 // above the SQLite 32766 and below nothing else: forces chunking on both dialects
        ms := make([]Document, n)
        for i := range ms {
            ms[i] = Document{TenantID: int64(i % 5), Title: "bulk"}
        }
        res, err := momo.Insert[Document](db).Omit(DocBody, DocSecret, DocDeletedAt).Models(ms).Exec(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if res.RowsAffected != n {
            t.Fatalf("RowsAffected = %d, want %d", res.RowsAffected, n)
        }
        count, err := momo.Select[Document](db).Count(ctx)
        if err != nil || count != n {
            t.Fatalf("count = %d, %v", count, err)
        }
    })
}
```

Note: 40000 rows × 6 written columns (tenant_id, title, version, created_at, updated_at, secret excluded, body excluded, deleted_at excluded) gives 4 bind columns if the flagged timestamp columns are rendered by the database; the test expects chunking on SQLite (`32766 / 5` rows per statement) because `version`, `created_at`, `updated_at` are written from the model. Chunk size is computed from the actual bound-column count, not the table width.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestBulkInsertChunking .` → on SQLite `too many SQL variables` (the chunk size ignores the dialect limit), or on PostgreSQL the same error at 65535 parameters.

- [ ] **Step 3: Implement** chunking in `Models`: `perStmt := dialect.MaxParams / boundCols`, loop with `slice[i:j]`, reuse one `Compiled` builder per chunk, sum `RowsAffected`.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestBulkInsertChunking -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): chunk bulk inserts under the dialect parameter limit`

## Task 14: COPY bulk path through the driver (S-5, S-9)

Files:

- Create: `/Volumes/DATA/Development/momo/copy_test.go`
- Modify: `/Volumes/DATA/Development/momo/insert.go` (adds `Copy`)

Interfaces:

```go
// Copy streams models through the driver's Copier (PostgreSQL COPY). It returns
// ErrUnsupported when the driver does not implement momo.Copier (SQLite).
func (q InsertQuery[T]) Copy(ctx context.Context, ms []T) (Result, error)
```

- [ ] **Step 1: Write the failing test** `copy_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
)

func TestCopyInsert(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        ms := []Document{{TenantID: 1, Title: "c1"}, {TenantID: 2, Title: "c2"}}
        if d == dialect.SQLite {
            if _, err := momo.Insert[Document](db).Omit(DocBody, DocSecret, DocDeletedAt).Copy(ctx, ms); !errors.Is(err, momo.ErrUnsupported) {
                t.Fatalf("sqlite COPY err = %v, want ErrUnsupported", err)
            }
            return
        }
        res, err := momo.Insert[Document](db).Omit(DocBody, DocSecret, DocDeletedAt).Copy(ctx, ms)
        if err != nil || res.RowsAffected != 2 {
            t.Fatalf("copy: %+v %v", res, err)
        }
        got, err := momo.Select[Document](db).Where(DocTitle.In("c1", "c2")).OrderBy(DocTitle.Asc()).All(ctx)
        if err != nil || len(got) != 2 || got[0].TenantID != 1 || got[1].TenantID != 2 {
            t.Fatalf("copied rows = %+v %v", got, err)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestCopyInsert .` → `undefined: Copy` (method missing).

- [ ] **Step 3: Implement** `Copy`: check `momo.CopierOf(q)` (momo-03 Task 8; it also finds the transaction's Copier, so the insert works inside `db.Tx`); build the column list from the model's non-omitted writable columns and row values from `Meta.Value`; call `CopyFrom(ctx, table, cols, rows)`. Use the `DB`'s driver via `DB.Driver()`.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestCopyInsert -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): COPY insert path through the driver Copier`

## Task 15: Update builder, mutation guardrails and optimistic locking — S-5

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

Files:

- Create: `/Volumes/DATA/Development/momo/update.go`
- Create: `/Volumes/DATA/Development/momo/update_test.go`

Interfaces:

```go
func Update[T any, PT Modeler[T]](q Querier) UpdateQuery[T]
func (u UpdateQuery[T]) Set(assigns ...Assignment) UpdateQuery[T]
func (u UpdateQuery[T]) Where(ps ...Predicate) UpdateQuery[T]
func (u UpdateQuery[T]) Model(m *T) UpdateQuery[T]       // version-checked when the model has FlagVersion
func (u UpdateQuery[T]) AllRows() UpdateQuery[T]         // explicit confirmation for an unfiltered UPDATE
func (u UpdateQuery[T]) Exec(ctx context.Context) (Result, error)
func (u UpdateQuery[T]) SQL() (Compiled, error)
```

Rules: no `Where` and no `AllRows` → `ErrUnfilteredMutation` (build-time, nothing executed). `Model(m)` adds `WHERE pk = ? AND version = ?` and `SET version = version + 1` when a version column exists; zero rows affected → `ErrOptimisticLock` when the row exists with another version, `ErrNotFound` when it does not (checked with a follow-up SELECT inside the same transaction). Updated-at is set only by the statement that matches rows (Task 20).

- [ ] **Step 1: Write the failing test** `update_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestMutationGuardrails(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'g')"); err != nil {
            t.Fatal(err)
        }
        if _, err := momo.Update[Document](db).Set(DocTitle.Set("x")).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
            t.Fatalf("unfiltered update err = %v", err)
        }
        if _, err := momo.Delete[Document](db).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
            t.Fatalf("unfiltered delete err = %v", err)
        }
        res, err := momo.Update[Document](db).Set(DocTitle.Set("all")).AllRows().Exec(ctx)
        if err != nil || res.RowsAffected != 1 {
            t.Fatalf("AllRows update: %+v %v", res, err)
        }
    })
}

func TestOptimisticLock(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'v')"); err != nil {
            t.Fatal(err)
        }
        doc, err := momo.Select[Document](db).First(ctx)
        if err != nil {
            t.Fatal(err)
        }
        stale := doc
        doc.Title = "first"
        if _, err := momo.Update[Document](db).Model(&doc).Exec(ctx); err != nil {
            t.Fatalf("fresh update: %v", err)
        }
        stale.Title = "second"
        if _, err := momo.Update[Document](db).Model(&stale).Exec(ctx); !errors.Is(err, momo.ErrOptimisticLock) {
            t.Fatalf("stale update err = %v, want ErrOptimisticLock", err)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run 'TestMutationGuardrails|TestOptimisticLock' .` → `undefined: momo.Update`.

- [ ] **Step 3: Implement** `update.go` per Interfaces. The guardrail is checked in `SQL()`, before any execution. Optimistic lock: `UPDATE documents SET ..., version = version + 1 WHERE id = ? AND version = ?`; a zero-row result triggers the follow-up `SELECT version FROM documents WHERE id = ?` inside a transaction (`Tx` created by the builder when the caller passes a `DB`), returning `ErrOptimisticLock` or `ErrNotFound`.

- [ ] **Step 4: Run and expect PASS.** `go test -run 'TestMutationGuardrails|TestOptimisticLock' -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): update builder with unfiltered guard and optimistic locking`

## Task 16: Explicit write states: omitted, zero, NULL and DEFAULT (S-5)

Files:

- Create: `/Volumes/DATA/Development/momo/write_states_test.go` (wrappers; bodies live in `internal/suite/writes.go`)
- Create: `/Volumes/DATA/Development/momo/internal/suite/writes.go` (`WriteStates`, `WriteStatesInsert`, `WriteStatesMultiRow`)
- Modify: `/Volumes/DATA/Development/momo/insert.go` (adds `ValuesRows` and the DEFAULT rules)
- Modify: `/Volumes/DATA/Development/momo/update.go` (rejects DEFAULT when `DefaultInUpdate` is false)

Interfaces:

```go
// ValuesRows adds one row per element to a multi-row VALUES list. Each row is a list of assignments.
func (q InsertQuery[T]) ValuesRows(rows ...[]Assignment) InsertQuery[T]

// WriteStates runs the UPDATE states (suite function, used by the Writes group).
func WriteStates(t *testing.T, db *momo.DB, caps Caps)
func WriteStatesInsert(t *testing.T, db *momo.DB, caps Caps)
func WriteStatesMultiRow(t *testing.T, db *momo.DB, caps Caps)
```

Rendering rules, implemented in Step 3:

- An assignment decides the column's state. Omitted (column not in the list): the column is absent from the INSERT column list, or left unchanged in an UPDATE SET list. `setValue`: bound argument, so `""` is stored as the empty string. `setNull` (`SetNull`): `NULL` literal. `setDefault` (`SetDefault`): `DEFAULT`.
- UPDATE `SetDefault` on a dialect with `DefaultInUpdate == false` (SQLite): `UpdateQuery.SQL()` returns `ErrUnsupported` at build time; nothing executes.
- INSERT single row (`Values`), `setDefault` on a dialect with `DefaultInValues == false` (SQLite): the column is omitted from the column list, so the column default applies. On PostgreSQL the rendering is `DEFAULT`.
- INSERT multi-row (`ValuesRows`), SQLite (`DefaultInValues == false`): a column whose assignment is `setDefault` in every row is omitted from the column list. If `setDefault` appears in some rows and a value or NULL in others, or a column is omitted in some rows and given in others, the build returns `ErrUnsupported` (SQLite cannot express a per-row DEFAULT in one VALUES list). PostgreSQL renders `DEFAULT` inline per row, and an omitted column in a row is rendered as `DEFAULT` for that row.

- [ ] **Step 1: Write the failing tests** (bodies in `internal/suite/writes.go`; wrappers in `write_states_test.go`):

```go
// write_states_test.go
package momo_test

import (
    "testing"

    "github.com/azrtydxb/momo/internal/suite"
)

func TestWriteStates(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.WriteStates(t, db, suite.CapsOf(db)) })
}

func TestWriteStatesInsert(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.WriteStatesInsert(t, db, suite.CapsOf(db)) })
}

func TestWriteStatesMultiRow(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.WriteStatesMultiRow(t, db, suite.CapsOf(db)) })
}
```

Bodies (`internal/suite/writes.go`, package `suite`, imports `context`, `database/sql`, `errors`, `testing`, `momo`):

```go
// statusOf reads documents.status for one title. Every error is checked.
func statusOf(t *testing.T, db *momo.DB, title string) sql.NullString {
    t.Helper()
    vals, err := momo.Scalars[sql.NullString](context.Background(), db, "SELECT status FROM documents WHERE title = ?", title)
    if err != nil || len(vals) != 1 {
        t.Fatalf("status of %q: err=%v rows=%d", title, err, len(vals))
    }
    return vals[0]
}
```

`statusOf` passes the SQL with a plain `?` placeholder, like every other raw call; `Scalars` renders it through `appendRaw`, so the same text runs on PostgreSQL (`$1`) and SQLite (`?`). No placeholder helper is needed.

`WriteStates` (UPDATE; four rows seeded with status `'x'` through `momo.Exec`, every error checked):

```go
func WriteStates(t *testing.T, db *momo.DB, caps Caps) {
    CreateDocuments(t, db, caps)
    ctx := context.Background()
    for _, title := range []string{"u-omit", "u-zero", "u-null", "u-default"} {
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title, status) VALUES (1, ?, 'x')", title); err != nil {
            t.Fatal(err)
        }
    }
    // omitted: status is not in SET, so 'x' stays
    if _, err := momo.Update[Document](db).Set(DocTitle.Set("u-omit")).Where(DocTitle.Eq("u-omit")).Exec(ctx); err != nil {
        t.Fatal(err)
    }
    // zero: the empty string is written
    if _, err := momo.Update[Document](db).Set(DocStatus.Set("")).Where(DocTitle.Eq("u-zero")).Exec(ctx); err != nil {
        t.Fatal(err)
    }
    // NULL
    if _, err := momo.Update[Document](db).Set(DocStatus.SetNull()).Where(DocTitle.Eq("u-null")).Exec(ctx); err != nil {
        t.Fatal(err)
    }
    // DEFAULT: PostgreSQL stores 'draft'; SQLite rejects it at build time
    _, err := momo.Update[Document](db).Set(DocStatus.SetDefault()).Where(DocTitle.Eq("u-default")).Exec(ctx)
    if !caps.DefaultInUpdate {
        if !errors.Is(err, momo.ErrUnsupported) {
            t.Fatalf("SET DEFAULT on %s: err = %v, want ErrUnsupported", caps.Dialect, err)
        }
    } else if err != nil {
        t.Fatal(err)
    }
    if got := statusOf(t, db, "u-omit"); !got.Valid || got.String != "x" {
        t.Fatalf("omitted kept %v, want 'x'", got)
    }
    if got := statusOf(t, db, "u-zero"); !got.Valid || got.String != "" {
        t.Fatalf("zero stored %v, want ''", got)
    }
    if got := statusOf(t, db, "u-null"); got.Valid {
        t.Fatalf("NULL stored %v, want NULL", got)
    }
    want := "x"
    if caps.DefaultInUpdate {
        want = "draft"
    }
    if got := statusOf(t, db, "u-default"); !got.Valid || got.String != want {
        t.Fatalf("DEFAULT stored %v, want %q", got, want)
    }
}
```

`WriteStatesInsert` (every error checked; omitted, zero, NULL and DEFAULT are four distinct SQL renderings):

```go
func WriteStatesInsert(t *testing.T, db *momo.DB, caps Caps) {
    CreateDocuments(t, db, caps)
    ctx := context.Background()
    if _, err := momo.Insert[Document](db).Values(DocTenantID.Set(1), DocTitle.Set("i-omit")).Exec(ctx); err != nil {
        t.Fatal(err)
    }
    for _, c := range []struct {
        title string
        a     momo.Assignment
    }{
        {"i-zero", DocStatus.Set("")},
        {"i-null", DocStatus.SetNull()},
        {"i-default", DocStatus.SetDefault()},
    } {
        if _, err := momo.Insert[Document](db).Values(DocTenantID.Set(1), DocTitle.Set(c.title), c.a).Exec(ctx); err != nil {
            t.Fatalf("insert %s: %v", c.title, err)
        }
    }
    want := map[string]sql.NullString{
        "i-omit":    {String: "draft", Valid: true},
        "i-zero":    {String: "", Valid: true},
        "i-null":    {},
        "i-default": {String: "draft", Valid: true},
    }
    for title, w := range want {
        if got := statusOf(t, db, title); got != w {
            t.Fatalf("%s stored %v, want %v", title, got, w)
        }
    }
}
```

`WriteStatesMultiRow`:

```go
func WriteStatesMultiRow(t *testing.T, db *momo.DB, caps Caps) {
    CreateDocuments(t, db, caps)
    ctx := context.Background()
    uniform := momo.Insert[Document](db).ValuesRows(
        []momo.Assignment{DocTenantID.Set(1), DocTitle.Set("m1"), DocStatus.SetDefault()},
        []momo.Assignment{DocTenantID.Set(1), DocTitle.Set("m2"), DocStatus.SetDefault()},
    )
    if _, err := uniform.Exec(ctx); err != nil {
        t.Fatal(err)
    }
    for _, title := range []string{"m1", "m2"} {
        if got := statusOf(t, db, title); !got.Valid || got.String != "draft" {
            t.Fatalf("%s = %v, want 'draft'", title, got)
        }
    }
    mixed := momo.Insert[Document](db).ValuesRows(
        []momo.Assignment{DocTenantID.Set(1), DocTitle.Set("n1"), DocStatus.SetDefault()},
        []momo.Assignment{DocTenantID.Set(1), DocTitle.Set("n2"), DocStatus.Set("y")},
    )
    _, err := mixed.Exec(ctx)
    if !caps.DefaultInValues {
        if !errors.Is(err, momo.ErrUnsupported) {
            t.Fatalf("mixed DEFAULT on %s: err = %v, want ErrUnsupported", caps.Dialect, err)
        }
        return
    }
    if err != nil {
        t.Fatal(err)
    }
    if n1 := statusOf(t, db, "n1"); !n1.Valid || n1.String != "draft" {
        t.Fatalf("n1 = %v, want 'draft'", n1)
    }
    if n2 := statusOf(t, db, "n2"); !n2.Valid || n2.String != "y" {
        t.Fatalf("n2 = %v, want 'y'", n2)
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test -run 'TestWriteStates' .` → `undefined: momo.Insert` before Tasks 12 and 15 land, and `undefined: ValuesRows` after; the expected failure once the builders exist is `SET DEFAULT on sqlite: err = <nil>, want ErrUnsupported` (before the DEFAULT rule is implemented).

- [ ] **Step 3: Implement** the rules listed in the Rendering rules above in `insert.go` and `update.go`. `UpdateQuery.SQL()` checks setDefault against `caps`-independent dialect flags `dialect.DefaultInUpdate`; `InsertQuery` takes `dialect.DefaultInValues`.

- [ ] **Step 4: Mutation check.** In the assignment renderer, map `setValue` with an empty string to the DEFAULT rendering. Run `go test -run TestWriteStates .`. Expected FAIL `zero stored ..., want ''`. Restore.

- [ ] **Step 5: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test -run 'TestWriteStates' -race .` → `ok` on both dialects.

- [ ] **Step 6: Commit.** `test(core): write states proven on UPDATE and INSERT with explicit SQLite rules`

## Task 17: Delete builder and guardrail — S-5

Files:

- Create: `/Volumes/DATA/Development/momo/delete.go`
- Create: `/Volumes/DATA/Development/momo/delete_test.go`

Interfaces: `func Delete[T any, PT Modeler[T]](q Querier) DeleteQuery[T]`, with `Where`, `Model(*T)` (by primary key), `AllRows`, `Exec(ctx) (Result, error)`, `SQL()`, `Returning` (Task 9). Same guardrail as Task 15.

- [ ] **Step 1: Write the failing test** `delete_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestDeleteByModelAndGuard(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'd1'), (1, 'd2')"); err != nil {
            t.Fatal(err)
        }
        d, err := momo.Select[Document](db).Where(DocTitle.Eq("d1")).One(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if res, err := momo.Delete[Document](db).Model(&d).Exec(ctx); err != nil || res.RowsAffected != 1 {
            t.Fatalf("delete by model: %+v %v", res, err)
        }
        if _, err := momo.Delete[Document](db).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
            t.Fatalf("unfiltered delete err = %v", err)
        }
    })
}
```

The code above is the final test.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestDeleteByModelAndGuard .` → `undefined: momo.Delete`.

- [ ] **Step 3: Implement** `delete.go` per Interfaces: `DELETE FROM t WHERE pk = ?` for `Model`; guardrail as in Task 15.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestDeleteByModelAndGuard -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): delete builder with unfiltered guard`

## Task 18: Soft delete variants (S-75) and the soft-delete read filter (S-5)

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

Files:

- Create: `/Volumes/DATA/Development/momo/softdelete.go`
- Create: `/Volumes/DATA/Development/momo/softdelete_test.go`

Interfaces:

```go
// SoftDelete returns an UpdateQuery that marks rows deleted instead of removing them.
// The marker follows the model's soft-delete column: a nullable timestamp (set to now via the
// clock), a Unix integer, or a boolean (set true). Meta.SoftDelete identifies the column.
func SoftDelete[T any, PT Modeler[T]](q Querier) UpdateQuery[T]

// ForceDelete is the hard delete for soft-deleting models.
func ForceDelete[T any, PT Modeler[T]](q Querier) DeleteQuery[T]

// OnlyDeleted narrows a select to soft-deleted rows.
func (sq SelectQuery[T]) OnlyDeleted() SelectQuery[T]
```

Default select filter: `deleted_at IS NULL` (timestamp), `deleted_at IS NULL` (unix, nullable integer), `deleted = FALSE` (boolean marker). Variants are tested in Task 18 with three fixture tables: `documents` (timestamp), `docs_unix` (`deleted_unix bigint null`), `docs_bool` (`deleted boolean not null default false`). The marker kind is recorded as `FlagSoftDelete` plus a Go type: `*time.Time`, `*int64` or `bool`, detected at generation time (hand-written meta here declares `SoftDeleteKind` in the fixture).

- [ ] **Step 1: Write the failing test** `softdelete_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestSoftDelete(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'keep'), (1, 'gone')"); err != nil {
            t.Fatal(err)
        }
        if _, err := momo.SoftDelete[Document](db).Where(DocTitle.Eq("gone")).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        visible, err := momo.Select[Document](db).All(ctx)
        if err != nil {
            t.Fatal(err)
        }
        all, err := momo.Select[Document](db).WithDeleted().All(ctx)
        if err != nil {
            t.Fatal(err)
        }
        only, err := momo.Select[Document](db).OnlyDeleted().All(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if len(visible) != 1 || len(all) != 2 || len(only) != 1 || only[0].Title != "gone" {
            t.Fatalf("visible=%d all=%d only=%d", len(visible), len(all), len(only))
        }
    })
}

func TestSoftDeleteVariants(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        createSoftVariants(t, h) // creates docs_unix (deleted_unix bigint) and docs_bool (deleted boolean) with a unique index on (tenant_id, title) WHERE marker IS NULL / marker = FALSE
        ctx := context.Background()
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (5, 'u')"); err != nil {
            t.Fatal(err)
        }
        if _, err := momo.SoftDelete[Document](db).Where(DocTitle.Eq("u")).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (5, 'u')") // re-create the soft-deleted key
        n, err := momo.Select[Document](db).Where(DocTitle.Eq("u")).Count(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if n != 1 {
            t.Fatalf("visible after re-create = %d, want 1", n)
        }
        if _, err := momo.ForceDelete[Document](db).Where(DocTitle.Eq("u")).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        left, err := momo.Select[Document](db).WithDeleted().Where(DocTitle.Eq("u")).Count(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if left != 0 {
            t.Fatalf("ForceDelete left %d rows", left)
        }
    })
}
```

`createSoftVariants` is defined in the test file and declares the unique index on `documents` for the timestamp variant: `CREATE UNIQUE INDEX ... ON documents (tenant_id, title) WHERE deleted_at IS NULL` (partial index, works on both dialects). The code above is the final test for the timestamp variant; the unix and boolean variants run the same body over `docs_unix` and `docs_bool` by table name through `WithTableName` (Task 8).

- [ ] **Step 2: Run and expect FAIL.** `go test -run 'TestSoftDelete' .` → `undefined: momo.SoftDelete` (or `OnlyDeleted`).

- [ ] **Step 3: Implement** `softdelete.go`: the default filter in `render` for models with `SoftDelete >= 0` unless `WithDeleted` or `OnlyDeleted`; `SoftDelete` renders `UPDATE t SET <marker> = <value> WHERE <filter>` with the value from the clock (`WithClock`, Task 20) for timestamps, `1` for unix (time in seconds), `TRUE` for booleans; `ForceDelete` renders DELETE with no soft filter.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestSoftDelete -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): soft delete with timestamp, unix and boolean markers`

## Task 19: Upsert by conflict target (S-5)

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

Files:

- Create: `/Volumes/DATA/Development/momo/upsert_test.go`
- Modify: `/Volumes/DATA/Development/momo/insert.go`

Interfaces: `func Upsert[T any, PT Modeler[T]](q Querier) InsertQuery[T]` returns an insert whose `OnConflict` defaults to the primary key and whose `DoUpdate` assigns every writable column with `SetExcluded()`.

- [ ] **Step 1: Write the failing test** `upsert_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestUpsertUpdatesAllColumns(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        d := Document{ID: 77, TenantID: 1, Title: "first"}
        if _, err := momo.Upsert[Document](db).Model(&d).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        d.Title = "second"
        d.TenantID = 9
        if _, err := momo.Upsert[Document](db).Model(&d).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        got, err := momo.Select[Document](db).Where(DocID.Eq(77)).One(ctx)
        if err != nil || got.Title != "second" || got.TenantID != 9 {
            t.Fatalf("upsert did not update all columns: %+v %v", got, err)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestUpsertUpdatesAllColumns .` → `undefined: momo.Upsert`.

- [ ] **Step 3: Implement** `Upsert` per Interfaces. On SQLite, explicit IDs insert as given; on PostgreSQL the identity column accepts explicit values with `OVERRIDING SYSTEM VALUE` rendered by the insert when `ID` is set (add that rendering only for `Upsert` with explicit identity values).

- [ ] **Step 4: Run and expect PASS.** `go test -run TestUpsertUpdatesAllColumns -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): upsert by primary key updating all writable columns`

## Task 20: Automatic timestamps and the injectable clock (S-68)

Files:

- Create: `/Volumes/DATA/Development/momo/clock.go`
- Create: `/Volumes/DATA/Development/momo/timestamps_test.go`
- Modify: `/Volumes/DATA/Development/momo/db.go` (adds `WithClock`)

Interfaces:

```go
// WithClock sets the clock used for automatic timestamps and soft-delete markers.
func WithClock(now func() time.Time) Option
```

Rules: insert sets `created_at` and `updated_at` unless the caller assigned them explicitly (explicit assignment wins); update sets `updated_at` only in the statement that matches rows (the SET list is part of the same statement, so a no-match update changes nothing); the clock is read once per statement.

- [ ] **Step 1: Write the failing test** `timestamps_test.go`:

```go
package momo_test

import (
    "context"
    "testing"
    "time"

    "github.com/azrtydxb/momo"
)

func TestAutoTimestamps(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
        db := momo.New(db.Driver(), momo.WithClock(func() time.Time { return fixed }))
        ctx := context.Background()
        if _, err := momo.Insert[Document](db).Values(DocTenantID.Set(1), DocTitle.Set("ts")).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        d, err := momo.Select[Document](db).Where(DocTitle.Eq("ts")).One(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if !d.CreatedAt.Equal(fixed) || !d.UpdatedAt.Equal(fixed) {
            t.Fatalf("insert timestamps = %v %v, want %v", d.CreatedAt, d.UpdatedAt, fixed)
        }
        later := fixed.Add(time.Hour)
        db2 := momo.New(db.Driver(), momo.WithClock(func() time.Time { return later }))
        if _, err := momo.Update[Document](db2).Set(DocTitle.Set("ts2")).Where(DocTitle.Eq("nomatch")).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        d2, err := momo.Select[Document](db).Where(DocTitle.Eq("ts")).One(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if !d2.UpdatedAt.Equal(fixed) {
            t.Fatalf("no-match update changed updated_at to %v", d2.UpdatedAt)
        }
        if _, err := momo.Update[Document](db2).Set(DocTitle.Set("ts2")).Where(DocTitle.Eq("ts")).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        d3, err := momo.Select[Document](db).Where(DocTitle.Eq("ts2")).One(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if !d3.UpdatedAt.Equal(later) || !d3.CreatedAt.Equal(fixed) {
            t.Fatalf("update timestamps = created %v updated %v", d3.CreatedAt, d3.UpdatedAt)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestAutoTimestamps .` → `undefined: momo.WithClock`.

- [ ] **Step 3: Implement** `WithClock` (stores `db.clock`), auto-set in insert rendering for `CreatedAt`/`UpdatedAt` indexes (skipped when the caller assigned that column), and `updated_at = ?` added to the update SET list (not a trigger).

- [ ] **Step 4: Run and expect PASS.** `go test -run TestAutoTimestamps -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): automatic timestamps with an injectable clock`

## Task 21: Go-side defaults and validators before SQL (S-72)

Files:

- Create: `/Volumes/DATA/Development/momo/fields.go`
- Create: `/Volumes/DATA/Development/momo/fields_test.go`
- Modify: `/Volumes/DATA/Development/momo/meta.go` (adds `Fields []FieldHook[T]` to `Meta[T]`)

Interfaces:

```go
// FieldHook attaches Go-side behavior to one column of Meta[T].
type FieldHook[T any] struct {
    Column   int                       // index into Meta.Columns
    Default  func(m *T) any            // applied only when the field is omitted from a write (insert)
    OnUpdate func(m *T) any            // applied to every update that sets the column
    Validate func(v any) error         // run before SQL is built; a non-nil error becomes ErrValidation
}
```

Rules: a default applies only to omitted columns, never to zero values; validators run in the write builders before `SQL()` returns; the failing statement executes nothing (statement-count hook = 0). The validation error is `*Error{Kind: ErrValidation, Op, Table, Column}` wrapping the validator error.

- [ ] **Step 1: Write the failing test** `fields_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
)

type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context { c.n++; return ctx }
func (c *countHook) AfterQuery(ctx context.Context, e *momo.QueryEvent)                  {}

func TestDefaultsAndValidators(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        hook := &countHook{}
        db := momo.New(db.Driver(), momo.WithHook(hook), momo.WithFieldHooks(documentFieldHooks))
        ctx := context.Background()
        if _, err := momo.Insert[Document](db).Values(DocTenantID.Set(1)).Exec(ctx); err == nil {
            t.Fatal("empty title should fail validation")
        } else if !errors.Is(err, momo.ErrValidation) {
            t.Fatalf("err = %v, want ErrValidation", err)
        }
        if hook.n != 0 {
            t.Fatalf("validation ran after SQL was executed (%d statements)", hook.n)
        }
        d := Document{TenantID: 1, Title: "ok"}
        if _, err := momo.Insert[Document](db).Model(&d).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        got, err := momo.Select[Document](db).Where(DocTitle.Eq("ok")).One(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if got.Version != 1 {
            t.Fatalf("default version = %d, want 1", got.Version)
        }
        zero := Document{TenantID: 1, Title: "z", Version: 0}
        if _, err := momo.Insert[Document](db).Model(&zero).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        z, err := momo.Select[Document](db).Where(DocTitle.Eq("z")).One(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if z.Version != 0 {
            t.Fatalf("explicit zero version was replaced by a default: %d", z.Version)
        }
    })
}
```

`documentFieldHooks` is defined in the test file: a `[]momo.FieldHook[Document]` with a `Default` for the version column (`func(*Document) any { return int64(1) }`) only when the field is omitted, and a `Validate` on the title column returning an error for `""`. `WithFieldHooks` is an option added in this task that copies the hooks into `Meta`: because `Meta` is package-level, the option stores them on `DB` and the write builders look them up by `Meta` pointer. The test's `zero` case is only reachable if `Model` writes the explicit zero: that is the required behaviour and it is asserted.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestDefaultsAndValidators .` → `undefined: momo.FieldHook`.

- [ ] **Step 3: Implement** `fields.go`: `FieldHook`, `WithFieldHooks` option, and the write-builder pipeline `apply defaults (omitted only) → OnUpdate (updates) → Validate → compile`. Validation failures return before `Exec`.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestDefaultsAndValidators -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): Go-side defaults and validators run before SQL`

## Task 22: Codecs and the types package (S-73)

Files:

- Create: `/Volumes/DATA/Development/momo/types/codec.go`
- Create: `/Volumes/DATA/Development/momo/types/json.go`
- Create: `/Volumes/DATA/Development/momo/types/date.go`
- Create: `/Volumes/DATA/Development/momo/types/uuid.go`
- Create: `/Volumes/DATA/Development/momo/types/hstore.go`
- Create: `/Volumes/DATA/Development/momo/types/codec_test.go`

Interfaces (package `types`, imports only standard library and `dialect`):

```go
// Codec is the custom-type contract: database/sql scanning, driver value encoding and the SQL type per dialect.
type Codec interface {
    sql.Scanner
    driver.Valuer
    SQLType(d dialect.Name) string
}

// JSON[T] stores any JSON-encodable T: jsonb on PostgreSQL, TEXT on SQLite.
type JSON[T any] struct{ V T }

// Date is a calendar date (no time zone): date on PostgreSQL, TEXT 'YYYY-MM-DD' on SQLite.
type Date struct{ Time time.Time }

// TimeOfDay is a time of day: time on PostgreSQL, TEXT 'HH:MM:SS.nnnnnnnnn' on SQLite.
type TimeOfDay struct{ Nanos int64 }

// UUID is a 16-byte identifier: uuid on PostgreSQL, TEXT canonical form on SQLite.
type UUID [16]byte

// Hstore is map[string]*string: hstore on PostgreSQL, TEXT JSON on SQLite.
type Hstore map[string]*string
```

- [ ] **Step 1: Write the failing test** `types/codec_test.go`:

```go
package types

import (
    "database/sql/driver"
    "testing"
    "time"

    "github.com/azrtydxb/momo/dialect"
)

func TestCodecsRoundTrip(t *testing.T) {
    cases := []struct {
        name  string
        codec Codec
        fresh func() Codec
        want  any
    }{
        {"JSON", &JSON[map[string]int]{V: map[string]int{"a": 1}}, func() Codec { return &JSON[map[string]int]{} }, map[string]int{"a": 1}},
        {"Date", &Date{Time: time.Date(1969, 12, 31, 0, 0, 0, 0, time.UTC)}, func() Codec { return &Date{} }, "1969-12-31"},
        {"TimeOfDay", &TimeOfDay{Nanos: 3723000000000}, func() Codec { return &TimeOfDay{} }, int64(3723000000000)},
        {"UUID", &UUID{1, 2, 3}, func() Codec { return &UUID{} }, UUID{1, 2, 3}},
        {"Hstore", &Hstore{"k": ptr("v"), "n": nil}, func() Codec { return &Hstore{} }, nil},
    }
    for _, c := range cases {
        for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
            if c.codec.SQLType(d) == "" {
                t.Fatalf("%s has no SQL type for %s", c.name, d)
            }
        }
        v, err := c.codec.Value()
        if err != nil {
            t.Fatalf("%s Value: %v", c.name, err)
        }
        back := c.fresh()
        if err := back.Scan(v); err != nil {
            t.Fatalf("%s Scan(%v): %v", c.name, v, err)
        }
        if _, ok := any(back).(driver.Valuer); !ok {
            t.Fatalf("%s does not implement driver.Valuer", c.name)
        }
    }
}

func ptr(s string) *string { return &s }
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./types/ -run TestCodecsRoundTrip` → `undefined: Codec`.

- [ ] **Step 3: Implement** the five files per Interfaces. Date on SQLite stores `2006-01-02` text; `Scan` parses it back. `UUID` stores the canonical 36-char form on SQLite and `uuid` on PostgreSQL; the Value on PostgreSQL is the canonical string. `Hstore` `Value` returns the hstore text form `"k"=>"v", "n"=>NULL`; SQLite stores it as TEXT; `Scan` accepts both the hstore and JSON forms. A dialect-aware `ValueFor(dialect.Name)` codec hook, for JSON storage on SQLite, is deferred to M2 and must be added to ARCHITECTURE when designed. `TimeOfDay` `Value` truncates to microseconds (PostgreSQL precision) and `Scan` reads `24:00:00` as end of day.

- [ ] **Step 4: Run and expect PASS.** `go test ./types/ -race` → `ok`.

- [ ] **Step 5: Commit.** `feat(types): codecs for JSON, Date, TimeOfDay, UUID and Hstore`

## Task 23: Documented type mapping with round-trip test per mapping (S-31)

Files:

- Create: `/Volumes/DATA/Development/momo/docs/TYPE_MAPPING.md`
- Create: `/Volumes/DATA/Development/momo/type_mapping_test.go`

Mapping table (the document and the test are generated from the same table in the test file to keep them in sync; the test asserts the doc contains each row):

| Go type                 | PostgreSQL                         | SQLite                                   |
| ----------------------- | ---------------------------------- | ---------------------------------------- |
| int64                   | bigint                             | INTEGER                                  |
| int32                   | integer                            | INTEGER                                  |
| float64                 | double precision                   | REAL                                     |
| string                  | text                               | TEXT                                     |
| bool                    | boolean                            | INTEGER (0/1)                            |
| []byte                  | bytea                              | BLOB                                     |
| time.Time (timestamp)   | timestamp (without time zone, UTC) | TEXT (RFC3339Nano, UTC)                  |
| time.Time (timestamptz) | timestamptz                        | TEXT (RFC3339Nano with offset preserved) |
| types.Date              | date                               | TEXT                                     |
| types.UUID              | uuid                               | TEXT                                     |
| types.JSON[T]           | jsonb                              | TEXT                                     |
| []string                | text[]                             | TEXT (JSON array)                        |
| decimal (string)        | numeric                            | TEXT                                     |

Edge values: `math.MaxInt64`, pre-1970 time (`1969-07-20T20:17:40Z`), a non-UTC zone for timestamptz (`+05:30`), `NaN` for float64 (PostgreSQL only; SQLite stores NULL-free NaN as NULL is not used: test asserts SQLite rejects NaN by returning ErrValidation).

- [ ] **Step 1: Write the failing test** `type_mapping_test.go`:

```go
package momo_test

import (
    "context"
    "math"
    "os"
    "strings"
    "testing"
    "time"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
)

func TestTypeMapping(t *testing.T) {
    doc, err := os.ReadFile("docs/TYPE_MAPPING.md")
    if err != nil {
        t.Fatal(err)
    }
    for _, row := range []string{"| int64 |", "| time.Time (timestamptz) |", "| []string |"} {
        if !strings.Contains(string(doc), row) {
            t.Fatalf("docs/TYPE_MAPPING.md is missing row %q", row)
        }
    }
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        ctx := context.Background()
        ddl := map[dialect.Name]string{
            dialect.Postgres: "CREATE TABLE tm (i bigint, f double precision, s text, b boolean, ts timestamp, tz timestamptz, arr text[], bin bytea)",
            dialect.SQLite:   "CREATE TABLE tm (i INTEGER, f REAL, s TEXT, b INTEGER, ts TEXT, tz TEXT, arr TEXT, bin BLOB)",
        }
        if _, err := db.Driver().Exec(ctx, ddl[d], nil); err != nil {
            t.Fatal(err)
        }
        pre1970 := time.Date(1969, 7, 20, 20, 17, 40, 0, time.UTC)
        zone := time.Date(2026, 1, 1, 12, 0, 0, 0, time.FixedZone("IST", 5*3600+1800))
        if _, err := momo.Exec(ctx, db, "INSERT INTO tm VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            int64(math.MaxInt64), 1.5, "text", true, pre1970, zone, `["a","b"]`, []byte{0, 1, 255}); err != nil {
            t.Fatal(err)
        }
        var i int64
        var f float64
        var s string
        var b bool
        var ts, tz time.Time
        var bin []byte
        row := db.Driver()
        rows, err := row.Query(ctx, "SELECT i, f, s, b, ts, tz, bin FROM tm", nil)
        if err != nil {
            t.Fatal(err)
        }
        defer rows.Close()
        rows.Next()
        if err := rows.Scan(&i, &f, &s, &b, &ts, &tz, &bin); err != nil {
            t.Fatal(err)
        }
        if i != math.MaxInt64 || !ts.Equal(pre1970) || !tz.Equal(zone) || len(bin) != 3 {
            t.Fatalf("round trip mismatch: i=%d ts=%v tz=%v bin=%v", i, ts, tz, bin)
        }
    })
}
```

The `NaN` edge is asserted in a second function `TestTypeMappingNaN` in the same file: on PostgreSQL `SELECT 'NaN'::float8` round-trips to `math.IsNaN`; on SQLite inserting `math.NaN()` returns `momo.ErrValidation` from a validator registered by the float64 codec. Add it in this task.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestTypeMapping .` → `open docs/TYPE_MAPPING.md: no such file or directory`.

- [ ] **Step 3: Implement** `docs/TYPE_MAPPING.md` with the table above and a short prose section for each edge value, then the NaN validator for SQLite.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestTypeMapping -race .` → `ok`.

- [ ] **Step 5: Commit.** `docs(core): documented Go-to-SQL type mapping with round-trip tests`

## Task 24: JSON and array operators (S-32)

Files:

- Create: `/Volumes/DATA/Development/momo/json_array.go`
- Create: `/Volumes/DATA/Development/momo/json_array_test.go`

Interfaces:

```go
// JSONPath renders col -> 'k' (path element) and col ->> 'k' (text). PostgreSQL only; SQLite
// uses json_extract where equivalent, ErrUnsupported otherwise.
func JSONGet[T any](c ColumnRef, key string) Expression[T]
func JSONText(c ColumnRef, key string) Expression[string]
func JSONContains(c ColumnRef, json string) Predicate        // @> (PostgreSQL); ErrUnsupported on SQLite
func JSONHasKey(c ColumnRef, key string) Predicate            // ? (PostgreSQL); json_type IS NOT NULL on SQLite
func ArrayContains[T any](c ColumnRef, vals ...T) Predicate   // @> ARRAY[...]; PostgreSQL only
func ArrayOverlaps[T any](c ColumnRef, vals ...T) Predicate   // && ; PostgreSQL only
func AnyOf[T any](c ColumnRef, vals ...T) Predicate           // = ANY($n) with one array bind; PostgreSQL only
```

The `?` operator collides with bind markers: the JSON key-exists predicate renders `?` as a literal by writing it through `Builder.WriteString` (trusted text) with the key bound as a value. Key and JSON values are always bound (`Arg`), never interpolated.

- [ ] **Step 1: Write the failing test** `json_array_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
)

func TestJSONAndArrayOperators(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        ctx := context.Background()
        if d == dialect.Postgres {
            if _, err := db.Driver().Exec(ctx, "ALTER TABLE documents ADD COLUMN meta jsonb NOT NULL DEFAULT '{}'::jsonb, ADD COLUMN tags text[] NOT NULL DEFAULT '{}'", nil); err != nil {
                t.Fatal(err)
            }
            if _, err := momo.Exec(ctx, db, `INSERT INTO documents (tenant_id, title, meta, tags) VALUES (1, 'j', '{"color":"red"}', ARRAY['a','b'])`); err != nil {
                t.Fatal(err)
            }
            if _, err := momo.Exec(ctx, db, `INSERT INTO documents (tenant_id, title, meta, tags) VALUES (1, 'k', '{"color":"blue"}', ARRAY['c'])`); err != nil {
                t.Fatal(err)
            }
            n, err := momo.Select[Document](db).Where(momo.JSONContains(docMeta(), `{"color":"red"}`)).Count(ctx)
            if err != nil || n != 1 {
                t.Fatalf("@> count = %d, %v", n, err)
            }
            n, err = momo.Select[Document](db).Where(momo.ArrayOverlaps(docTags(), "b", "z")).Count(ctx)
            if err != nil || n != 1 {
                t.Fatalf("&& count = %d, %v", n, err)
            }
            c, err := momo.Select[Document](db).Where(momo.AnyOf(DocTitle, "j", "k")).SQL()
            if err != nil {
                t.Fatal(err)
            }
            if len(c.Args) != 1 {
                t.Fatalf("= ANY must bind one array argument, got %d", len(c.Args))
            }
            return
        }
        if _, err := momo.Select[Document](db).Where(momo.JSONContains(docMeta(), `{}`)).SQL(); !errors.Is(err, momo.ErrUnsupported) {
            t.Fatalf("sqlite JSONContains err = %v, want ErrUnsupported", err)
        }
    })
}
```

`docMeta()` and `docTags()` return the typed descriptors below, defined in the test file. `DocTitle` is already defined in Task 1.

```go
momo.NewColumn[string]("documents","meta",0)
momo.NewColumn[[]string]("documents","tags",0)
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestJSONAndArrayOperators .` → `undefined: momo.JSONContains`.

- [ ] **Step 3: Implement** `json_array.go` per Interfaces. Gate each operator on dialect flags (`dialect.ILike`-style: add `JSON` and `Arrays` booleans to `dialect.Dialect` — contract addition, see report) and return `ErrUnsupported` at build time on SQLite for the PostgreSQL-only operators.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestJSONAndArrayOperators -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): JSONB and array operators with bound values`

## Task 25: Window functions, partitioned and ordered (S-33, window part only)

Files:

- Create: `/Volumes/DATA/Development/momo/window.go`
- Create: `/Volumes/DATA/Development/momo/window_test.go`

Interfaces:

```go
type WindowSpec struct {
    Partition []ColumnRef
    Order     []Orderable
}

func OverPartition(cols ...ColumnRef) WindowSpec
func (w WindowSpec) OrderBy(items ...Orderable) WindowSpec

func RowNumber(w WindowSpec) Expression[int64]
func Rank(w WindowSpec) Expression[int64]
func DenseRank(w WindowSpec) Expression[int64]
func WindowSum[T Number](e Expression[T], w WindowSpec) Expression[T]
```

Rendering: `ROW_NUMBER() OVER (PARTITION BY ... ORDER BY ...)`. Window functions require `dialect.WindowFunctions` (both dialects true; SQLite >= 3.25).

- [ ] **Step 1: Write the failing test** `window_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

type ranked struct {
    momo.Projection
    ID   int64
    Rank int64
}

func (*ranked) MomoMeta() *momo.Meta[ranked] {
    return &momo.Meta[ranked]{
        Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "rk"}},
        Ptr: func(m *ranked, i int) any {
            if i == 0 {
                return &m.ID
            }
            return &m.Rank
        },
        Value: func(m *ranked, i int) any { return nil },
    }
}

func TestWindowFunctions(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        for _, tc := range []int64{1, 1, 2} {
            if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (?, 'w')", tc); err != nil {
                t.Fatal(err)
            }
        }
        rows, err := momo.Select[ranked](db).
            Columns(DocID, momo.RowNumber(momo.OverPartition(DocTenantID).OrderBy(DocID.Asc())).As("rk")).
            OrderBy(DocID.Asc()).
            All(ctx)
        if err != nil {
            t.Fatal(err)
        }
        want := []int64{1, 2, 1}
        for i, r := range rows {
            if r.Rank != want[i] {
                t.Fatalf("row %d rank = %d, want %d", i, r.Rank, want[i])
            }
        }
    })
}
```

`.As("rk")` on an `Expression` is a new method `func (a Agg[T]) As(alias string) Expr` returning an aliased select-list item; add it with the window file.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestWindowFunctions .` → `undefined: momo.RowNumber`.

- [ ] **Step 3: Implement** `window.go` per Interfaces and the `As` select-list alias.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestWindowFunctions -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): window functions with partitions and ordering`

## Task 26: Error redaction with Detail accessor (S-41)

Files:

- Modify: `/Volumes/DATA/Development/momo/errors.go` (adds an unexported `detail string` field to `Error` and the `Detail() string` method)
- Create: `/Volumes/DATA/Development/momo/errors_redaction_test.go`

Rules: `Error()` never includes the driver's `Detail`; `Detail()` returns the raw detail text captured by the driver classifier (PostgreSQL `Key (email)=(...) already exists.`), and `errors.As` reaches the `*pgconn.PgError` (already true from momo-03 Task 3).

- [ ] **Step 1: Write the failing test** `errors_redaction_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "strings"
    "testing"

    "github.com/jackc/pgx/v5/pgconn"
    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
)

func TestErrorRedaction(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        if _, err := db.Driver().Exec(ctx, "CREATE UNIQUE INDEX documents_title_uq ON documents (title)", nil); err != nil {
            t.Fatal(err)
        }
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'secret@example.com')"); err != nil {
            t.Fatal(err)
        }
        _, err := momo.Insert[Document](db).Values(DocTenantID.Set(1), DocTitle.Set("secret@example.com")).Exec(ctx)
        if !errors.Is(err, momo.ErrUniqueViolation) {
            t.Fatalf("err = %v", err)
        }
        if strings.Contains(err.Error(), "secret@example.com") {
            t.Fatalf("Error() leaked the value: %q", err.Error())
        }
        var me *momo.Error
        if !errors.As(err, &me) {
            t.Fatal("not a *momo.Error")
        }
        if d == dialect.Postgres {
            if !strings.Contains(me.Detail(), "secret@example.com") {
                t.Fatalf("Detail() lacks the raw detail: %q", me.Detail())
            }
            var pe *pgconn.PgError
            if !errors.As(err, &pe) || pe.Detail == "" {
                t.Fatal("raw PgError not reachable with errors.As")
            }
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestErrorRedaction .` → `me.Detail undefined`.

- [ ] **Step 3: Implement** the `detail` field: the classifier in momo-03 Task 3 sets `Err` to a redacted wrapper; this task adds `detail` to `Error` and sets it from `PgError.Detail` in `pgxdriver/classify.go` through an exported setter `momo.WithDetail(err, detail)` in `errors.go` (returns a copy with the detail set). `Detail()` returns it. SQLite classification sets detail to the SQLite message (column names only).

- [ ] **Step 4: Run and expect PASS.** `go test -run TestErrorRedaction -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): redacted error text with a raw Detail accessor`

## Task 27: Typed model hooks (S-85)

Files:

- Create: `/Volumes/DATA/Development/momo/hooks.go`
- Create: `/Volumes/DATA/Development/momo/hooks_test.go`
- Modify: `/Volumes/DATA/Development/momo/meta.go` (`Meta[T].Hooks HookSet[T]`, added in momo-01 per the contract; this task populates it)

Interfaces:

```go
// HookSet holds typed model hooks. Generated code (or hand-written meta) sets the fields;
// the write builders call them, in order, before SQL is built. A hook may set values
// (through the *T pointer) but cannot change the SQL.
type HookSet[T any] struct {
    BeforeInsert func(ctx context.Context, m *T) error
    BeforeUpdate func(ctx context.Context, m *T) error
    BeforeDelete func(ctx context.Context, m *T) error
    AfterLoad    func(ctx context.Context, m *T) error
}
```

Rules: a hook runs only for the builder kind it names; a returned error aborts before SQL (no statement executes); `AfterLoad` runs for every model returned by `All`, `One`, `First`, `Each`, `Iter` and `Rows`; the call path performs no reflection (asserted by an allocation test on the hook dispatch).

- [ ] **Step 1: Write the failing test** `hooks_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestModelHooks(t *testing.T) {
    var order []string
    old := DocumentMeta.Hooks
    DocumentMeta.Hooks = momo.HookSet[Document]{
        BeforeInsert: func(ctx context.Context, m *Document) error { order = append(order, "insert"); m.Secret = "set-by-hook"; return nil },
        BeforeUpdate: func(ctx context.Context, m *Document) error { order = append(order, "update"); return errors.New("veto") },
        AfterLoad:    func(ctx context.Context, m *Document) error { order = append(order, "load"); return nil },
    }
    t.Cleanup(func() { DocumentMeta.Hooks = old })
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        d := Document{TenantID: 1, Title: "hook"}
        if _, err := momo.Insert[Document](db).Model(&d).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        got, err := momo.Select[Document](db).Where(DocTitle.Eq("hook")).One(ctx)
        if err != nil || got.Secret != "set-by-hook" {
            t.Fatalf("BeforeInsert value not applied: %+v %v", got, err)
        }
        if _, err := momo.Update[Document](db).Model(&got).Exec(ctx); err == nil || errors.Is(err, momo.ErrOptimisticLock) {
            t.Fatalf("BeforeUpdate error did not abort: %v", err)
        }
    })
    want := []string{"insert", "load", "update"}
    for i := range want {
        if i >= len(order) || order[i] != want[i] {
            t.Fatalf("hook order = %v, want prefix %v", order, want)
        }
    }
}
```

Note: `DocumentMeta` is the package-level `Meta` of `internal/suite` (Task 1) and is shared; the test restores it in `t.Cleanup`. Because `testopen.Each` runs subtests, the hooks are visible to both dialects; the `order` assertion runs after both.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestModelHooks .` → `undefined: momo.HookSet` (until momo-01 adds the field) or `BeforeInsert did not run` once the field exists.

- [ ] **Step 3: Implement** `hooks.go` and the dispatch in the insert, update, delete builders (before `SQL()`) and in the result iteration (`AfterLoad` on each scanned model).

- [ ] **Step 4: Run and expect PASS.** `go test -run TestModelHooks -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): typed model hooks invoked by the write and read builders`

## Task 28: Sensitive columns redacted in events, errors and dumps (S-42)

Files:

- Create: `/Volumes/DATA/Development/momo/sensitive.go`
- Create: `/Volumes/DATA/Development/momo/sensitive_test.go`
- Modify: `/Volumes/DATA/Development/momo/db.go` (`before` redacts `QueryEvent.Args` when any argument is sensitive)

Interfaces:

```go
// Redacted is the placeholder value that replaces sensitive arguments in events, errors and SQL dumps.
const Redacted = "[redacted]"

// sensitiveArgs reports whether a compiled statement binds a FlagSensitive column (tracked by the builder).
```

Rules: with `WithArgLogging`, a statement that binds a sensitive column logs `QueryEvent.Args = []any{Redacted}`; error text never contains the sensitive value (the driver error is built from the server message, and a sensitive value is never placed into a `momo.Error` field); `SQL()` dumps used by `momotest` show `Redacted` for the bound value.

- [ ] **Step 1: Write the failing test** `sensitive_test.go`:

```go
package momo_test

import (
    "context"
    "strings"
    "testing"

    "github.com/azrtydxb/momo"
)

type argCapture struct{ events []momo.QueryEvent }

func (c *argCapture) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context { return ctx }
func (c *argCapture) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
    c.events = append(c.events, *e)
}

func TestSensitiveColumns(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        cap := &argCapture{}
        db := momo.New(db.Driver(), momo.WithHook(cap), momo.WithArgLogging())
        ctx := context.Background()
        if _, err := momo.Insert[Document](db).Values(DocTenantID.Set(1), DocTitle.Set("s"), DocSecret.Set("hunter2")).Exec(ctx); err != nil {
            t.Fatal(err)
        }
        for _, e := range cap.events {
            for _, a := range e.Args {
                if s, ok := a.(string); ok && strings.Contains(s, "hunter2") {
                    t.Fatalf("hook event carried the sensitive value: %v", e.Args)
                }
            }
        }
        _, err := momo.Insert[Document](db).Values(DocTenantID.Set(1), DocTitle.Set("s"), DocSecret.Set("hunter2")).SQL()
        if err != nil {
            t.Fatal(err)
        }
        dump, err := momo.Insert[Document](db).Values(DocTenantID.Set(1), DocTitle.Set("s"), DocSecret.Set("hunter2")).SQL()
        if err != nil {
            t.Fatal(err)
        }
        if strings.Contains(dump.String(), "hunter2") {
            t.Fatalf("SQL dump leaked the value: %s", dump.String())
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestSensitiveColumns .` → `hook event carried the sensitive value: [... hunter2]` on the first event (args logging enabled).

- [ ] **Step 3: Implement** the redaction: `Compiled` gains an unexported `sensitive bool` set by the builder when it binds a `FlagSensitive` column (contract addition, recorded in the report); `db.before` replaces the args with `[]any{Redacted}` when `sensitive` is set; `Compiled.String()` substitutes `Redacted` for bound values of sensitive statements.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestSensitiveColumns -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): redact sensitive columns in events and SQL dumps`

## Task 29: Interceptors and the per-handle privacy recipe (S-86)

Files:

- Create: `/Volumes/DATA/Development/momo/interceptor.go`
- Create: `/Volumes/DATA/Development/momo/interceptor_test.go`
- Create: `/Volumes/DATA/Development/momo/docs/PRIVACY_RULES.md`

Interfaces:

```go
// Operation is what an interceptor sees and may change before a typed statement compiles.
type Operation struct {
    Kind  string      // "select", "insert", "update", "delete"
    Table string
    Where []Predicate // interceptors may append predicates to select, update and delete
}

// Interceptor may append predicates or return an error to deny the statement. It runs
// before SQL is built. It is attached per handle with WithInterceptor and inherited by
// transactions begun from that handle only.
type Interceptor func(ctx context.Context, op *Operation) error

func WithInterceptor(i Interceptor) Option
```

- [ ] **Step 1: Write the failing test** `interceptor_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "strings"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestInterceptors(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        scoped := momo.New(db.Driver(), momo.WithInterceptor(func(ctx context.Context, op *momo.Operation) error {
            if op.Kind == "delete" {
                return errors.New("deletes are denied")
            }
            if op.Kind == "select" {
                op.Where = append(op.Where, DocTenantID.Eq(42))
            }
            return nil
        }))
        plain := db
        if _, err := momo.Exec(ctx, plain, "INSERT INTO documents (tenant_id, title) VALUES (1, 'other'), (42, 'mine')"); err != nil {
            t.Fatal(err)
        }
        rows, err := momo.Select[Document](scoped).All(ctx)
        if err != nil || len(rows) != 1 || rows[0].TenantID != 42 {
            t.Fatalf("interceptor predicate not applied: %+v %v", rows, err)
        }
        all, err := momo.Select[Document](plain).All(ctx)
        if err != nil {
            t.Fatal(err)
        }
        if len(all) != 2 {
            t.Fatalf("interceptor leaked to another handle: %d rows", len(all))
        }
        if _, err := momo.Delete[Document](scoped).Where(DocTenantID.Eq(1)).Exec(ctx); err == nil || !strings.Contains(err.Error(), "denied") {
            t.Fatalf("delete not denied: %v", err)
        }
        c, err := momo.Select[Document](scoped).SQL()
        if err != nil {
            t.Fatal(err)
        }
        if !strings.Contains(c.SQL, "tenant_id") {
            t.Fatalf("predicate not visible in compiled SQL: %s", c.SQL)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestInterceptors .` → `undefined: momo.WithInterceptor`.

- [ ] **Step 3: Implement** `interceptor.go`: the option stores the interceptor on `DB`; each select/update/delete builder calls the interceptors in order at `SQL()` time with an `Operation` populated from the table name, appends `op.Where` to the statement predicates, and returns any error wrapped as `*Error{Op: kind, Table: ..., Err: err}`. Write `docs/PRIVACY_RULES.md`: recipe for tenant predicates with interceptors; explicit statement that it is not a database security boundary and points to the RLS plan (S-38).

- [ ] **Step 4: Run and expect PASS.** `go test -run TestInterceptors -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): per-handle interceptors and privacy-rules recipe`

## Task 30: Resource-exhaustion guards: max rows and IN-list cap (S-48)

Files:

- Create: `/Volumes/DATA/Development/momo/limits.go`
- Create: `/Volumes/DATA/Development/momo/limits_test.go`

Interfaces:

```go
// WithMaxRows caps the rows a collecting read (All, Pluck, Scalars) may return; beyond the cap it returns ErrLimitExceeded.
func (sq SelectQuery[T]) MaxRows(n int) SelectQuery[T]

// WithMaxInList caps the number of values in one IN list; beyond the cap the statement is not built.
func WithMaxInList(n int) Option
```

`ErrLimitExceeded` is the sentinel from momo-01's `errors.go`. Overflow is an error, never a silent truncation.

- [ ] **Step 1: Write the failing test** `limits_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestMaxRowsGuard(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1,'a'),(1,'b'),(1,'c')"); err != nil {
            t.Fatal(err)
        }
        if _, err := momo.Select[Document](db).MaxRows(2).All(ctx); !errors.Is(err, momo.ErrLimitExceeded) {
            t.Fatalf("MaxRows(2) err = %v, want ErrLimitExceeded", err)
        }
        rows, err := momo.Select[Document](db).MaxRows(3).All(ctx)
        if err != nil || len(rows) != 3 {
            t.Fatalf("MaxRows(3) rows=%d err=%v", len(rows), err)
        }
    })
}

func TestInListCap(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        db := momo.New(db.Driver(), momo.WithMaxInList(2))
        _, err := momo.Select[Document](db).Where(DocTitle.In("a", "b", "c")).SQL()
        if !errors.Is(err, momo.ErrLimitExceeded) {
            t.Fatalf("IN over cap err = %v", err)
        }
        if _, err := momo.Select[Document](db).Where(DocTitle.In("a", "b")).SQL(); err != nil {
            t.Fatalf("IN at cap: %v", err)
        }
    })
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run 'TestMaxRowsGuard|TestInListCap' .` → `undefined: MaxRows`.

- [ ] **Step 3: Implement** `limits.go`: `maxRows` on `selectCore`, checked in `All` after the `(n+1)`th row is seen (fetch one extra row and return `ErrLimitExceeded` when it exists); `maxInList` on `DB`, checked in the typed `In`/`NotIn` rendering through `Builder.SetErr`.

- [ ] **Step 4: Run and expect PASS.** `go test -run 'TestMaxRowsGuard|TestInListCap' -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): max-rows and IN-list guards with structured errors`

## Task 31: Result helpers: Pluck, IDs, FirstID, scalars, maps, FindAndCount, Batches (S-78)

Files:

- Create: `/Volumes/DATA/Development/momo/results.go`
- Create: `/Volumes/DATA/Development/momo/results_test.go`

Interfaces:

```go
func Pluck[T any, V any](ctx context.Context, sq SelectQuery[T], c Expression[V]) ([]V, error)
func IDs[T any](ctx context.Context, sq SelectQuery[T]) ([]int64, error)        // uses the PK column
func FirstID[T any](ctx context.Context, sq SelectQuery[T]) (int64, error)     // ErrNotFound when empty
func Scalars[V any](ctx context.Context, q Querier, sql string, args ...any) ([]V, error)  // one column; raw SQL rendered via appendRaw (? binds)
func MapRows(ctx context.Context, q Querier, sql string, args ...any) ([]map[string]any, error)
func FindAndCount[T any](ctx context.Context, sq SelectQuery[T]) ([]T, int64, error)  // count and page, two statements
func Batches[T any](ctx context.Context, sq SelectQuery[T], n int, fn func([]T) error) error  // keyset on PK; never OFFSET
```

`Scalars` is first used in Task 6; this task extends it to typed scans for `V` as the single output column.

- [ ] **Step 1: Write the failing test** `results_test.go`:

```go
package momo_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
)

func TestResultHelpers(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        for i := 0; i < 7; i++ {
            if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", string(rune('a'+i))); err != nil {
                t.Fatal(err)
            }
        }
        titles, err := momo.Pluck(ctx, momo.Select[Document](db).OrderBy(DocID.Asc()), DocTitle)
        if err != nil || len(titles) != 7 || titles[0] != "a" {
            t.Fatalf("Pluck = %v %v", titles, err)
        }
        ids, err := momo.IDs(ctx, momo.Select[Document](db))
        if err != nil || len(ids) != 7 {
            t.Fatalf("IDs = %v %v", ids, err)
        }
        page, total, err := momo.FindAndCount(ctx, momo.Select[Document](db).OrderBy(DocID.Asc()).Limit(2))
        if err != nil || len(page) != 2 || total != 7 {
            t.Fatalf("FindAndCount page=%d total=%d err=%v", len(page), total, err)
        }
        seen := 0
        err = momo.Batches(ctx, momo.Select[Document](db), 3, func(b []Document) error {
            seen += len(b)
            return nil
        })
        if err != nil || seen != 7 {
            t.Fatalf("Batches visited %d rows, err=%v", seen, err)
        }
        maps, err := momo.MapRows(ctx, db, "SELECT title FROM documents ORDER BY id LIMIT 1")
        if err != nil || maps[0]["title"] != "a" {
            t.Fatalf("MapRows = %v %v", maps, err)
        }
    })
}
```

Note: `MapRows` takes a `Querier`; `db` is `*momo.DB`, which implements it. `FindAndCount` must not run the page query with `OFFSET` for `Batches`.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestResultHelpers .` → `undefined: momo.Pluck`.

- [ ] **Step 3: Implement** `results.go`; the Batches loop keeps the last PK and calls `After(lastID)` on a fresh copy of the query each batch, with `Limit(n)`.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestResultHelpers -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): result helpers with keyset batches`

## Task 32: Named scopes, typed fragments and runtime filter tree (S-79)

Files:

- Create: `/Volumes/DATA/Development/momo/compose.go`
- Create: `/Volumes/DATA/Development/momo/compose_test.go`

Interfaces:

```go
// Scope is a named, reusable query transformation.
type Scope[T any] func(SelectQuery[T]) SelectQuery[T]
func (sq SelectQuery[T]) Scope(s ...Scope[T]) SelectQuery[T]

// Filter is a runtime-built predicate tree validated against an Allowlist.
type Filter struct {
    And   []Filter
    Or    []Filter
    Not   *Filter
    Field string // external name, resolved through the Allowlist
    Op    string // eq, ne, lt, le, gt, ge, in, null, notnull, like
    Value any
}

// Predicate builds the predicate; unknown fields return ErrValidation.
func (f Filter) Predicate(al Allowlist) (Predicate, error)
```

The `Like` op uses `Like` from the spike (the value is a pattern already escaped by the caller with `EscapeLike`). `In` op uses the typed `In` with the bound values.

- [ ] **Step 1: Write the failing test** `compose_test.go`:

```go
package momo_test

import (
    "context"
    "errors"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/dialect"
)

func TestCompositionHelpers(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
        suite.CreateDocuments(t, db, suite.CapsOf(db))
        ctx := context.Background()
        if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1,'a'),(2,'b'),(2,'c')"); err != nil {
            t.Fatal(err)
        }
        byTenant := func(id int64) momo.Scope[Document] {
            return func(q momo.SelectQuery[Document]) momo.SelectQuery[Document] { return q.Where(DocTenantID.Eq(id)) }
        }
        n, err := momo.Select[Document](db).Scope(byTenant(2)).Count(ctx)
        if err != nil || n != 2 {
            t.Fatalf("scope count = %d %v", n, err)
        }
        al := momo.NewAllowlist(DocTenantID, DocTitle)
        f := momo.Filter{Or: []momo.Filter{{Field: "tenant_id", Op: "eq", Value: int64(1)}, {Field: "title", Op: "in", Value: []string{"c"}}}}
        p, err := f.Predicate(al)
        if err != nil {
            t.Fatal(err)
        }
        rows, err := momo.Select[Document](db).Where(p).OrderBy(DocTitle.Asc()).All(ctx)
        if err != nil || len(rows) != 2 {
            t.Fatalf("filter rows = %d %v", len(rows), err)
        }
        bad := momo.Filter{Field: "tenant_id; drop", Op: "eq", Value: 1}
        if _, err := bad.Predicate(al); !errors.Is(err, momo.ErrValidation) {
            t.Fatalf("unknown field err = %v", err)
        }
        locked := momo.Select[Document](db).ForUpdate("NOWAIT")
        if _, err := locked.SQL(); err != nil && d == dialect.SQLite {
            if !errors.Is(err, momo.ErrUnsupported) {
                t.Fatalf("sqlite NOWAIT err = %v", err)
            }
        }
    })
}
```

The last block asserts the SQLite `ErrUnsupported` rule for NOWAIT.

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestCompositionHelpers .` → `undefined: momo.Scope`.

- [ ] **Step 3: Implement** `compose.go`: `Scope` applies the function; `Filter.Predicate` recursively builds `And`/`Or`/`Not` with `Allowlist.Column`; unknown ops return `ErrValidation`.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestCompositionHelpers -race .` → `ok`.

- [ ] **Step 5: Commit.** `feat(core): scopes, runtime filter trees and typed fragment composition`

## Task 33: MERGE with a server-version gate, bulk update from VALUES and Truncate (S-80)

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

Files:

- Create: `/Volumes/DATA/Development/momo/merge.go`
- Create: `/Volumes/DATA/Development/momo/merge_gate_test.go` (package `momo`, unit test)
- Create: `/Volumes/DATA/Development/momo/merge_test.go` (package `momo_test` wrappers)
- Create: `/Volumes/DATA/Development/momo/internal/suite/merge.go` (bodies: `MergeAndBulkUpdate`)

Interfaces:

```go
// Merge starts a MERGE into T from a source. PostgreSQL 15+ only (dialect.Merge). RETURNING on MERGE
// requires PostgreSQL 17+; the version is read through momo.ServerVersioner (Task 13 of momo-03).
func Merge[T any, PT Modeler[T]](q Querier, source Subquery) MergeQuery[T]
func (m MergeQuery[T]) On(ps ...Predicate) MergeQuery[T]
func (m MergeQuery[T]) WhenMatchedUpdate(assigns ...Assignment) MergeQuery[T]
func (m MergeQuery[T]) WhenNotMatchedInsert(cols []ColumnRef, values ...Expr) MergeQuery[T]
func (m MergeQuery[T]) Returning(cols ...ColumnRef) MergeQuery[T]
func (m MergeQuery[T]) Exec(ctx context.Context) (Result, error)
func (m MergeQuery[T]) Rows(ctx context.Context) (*ModelRows[T], error)

// BulkUpdate updates rows of T from a VALUES list keyed by key. Only listed keys change.
// PostgreSQL only; ErrUnsupported on SQLite (caps.Merge is the capability that gates both).
func BulkUpdate[T any, PT Modeler[T]](q Querier, key ColumnRef, cols []ColumnRef, rows [][]any) UpdateQuery[T]

// Truncate removes all rows of T's table. PostgreSQL: TRUNCATE TABLE. SQLite: DELETE FROM and
// delete the table's sqlite_sequence row when present.
func Truncate[T any, PT Modeler[T]](ctx context.Context, q Querier) error

// mergeGate returns ErrUnsupported when serverVersion (server_version_num) is below 150000, or when
// returning is true and serverVersion is below 170000. It returns nil otherwise.
func mergeGate(serverVersion int, returning bool) error
```

Version source: `Merge` on a PostgreSQL DB calls `q.Driver().(momo.ServerVersioner).ServerVersion(ctx)`; on SQLite the dialect flag `Merge` is false and `Merge` returns `ErrUnsupported` at build time before any server call.

- [ ] **Step 1: Write the failing unit test** `merge_gate_test.go` (package `momo`):

```go
package momo

import (
    "errors"
    "testing"
)

func TestMergeGate(t *testing.T) {
    cases := []struct {
        version   int
        returning bool
        ok        bool
    }{
        {140013, false, false}, // below PostgreSQL 15: no MERGE
        {150000, false, true},  // MERGE without RETURNING
        {150000, true, false},  // RETURNING needs 17
        {160004, true, false},
        {170000, true, true},
        {170000, false, true},
    }
    for _, c := range cases {
        err := mergeGate(c.version, c.returning)
        if (err == nil) != c.ok {
            t.Fatalf("mergeGate(%d, %v) = %v, want ok=%v", c.version, c.returning, err, c.ok)
        }
        if err != nil && !errors.Is(err, ErrUnsupported) {
            t.Fatalf("mergeGate(%d, %v) error %v is not ErrUnsupported", c.version, c.returning, err)
        }
    }
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -run TestMergeGate .` → `undefined: mergeGate`.

- [ ] **Step 3: Implement** `mergeGate` and `Merge`, `BulkUpdate`, `Truncate` in `merge.go`. Rendering: `MERGE INTO t USING (<source>) AS src ON ... WHEN MATCHED THEN UPDATE SET ... WHEN NOT MATCHED THEN INSERT (...) VALUES (...)`, with `RETURNING` appended only after `mergeGate(v, true)` succeeds. `BulkUpdate` renders `UPDATE t SET c = v.c FROM (VALUES (...)) AS v(key, c) WHERE t.key = v.key`, values bound.

- [ ] **Step 4: Run and expect PASS.** `go test -run TestMergeGate -race .` → `ok`.

- [ ] **Step 5: Integration wrapper.** `merge_test.go`:

```go
package momo_test

import (
    "testing"

    "github.com/azrtydxb/momo/internal/suite"
)

func TestMergeAndBulkUpdate(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.MergeAndBulkUpdate(t, db, suite.CapsOf(db)) })
}
```

`suite.MergeAndBulkUpdate` (in `internal/suite/merge.go`) body: create documents; insert rows with ids 1 and 2 through `momo.Exec`; on SQLite assert `BulkUpdate(...).SQL()` and `Merge(...).Exec` return `ErrUnsupported`, then return; on PostgreSQL:

- The `BulkUpdate` call below returns `RowsAffected == 1`, and row 1 keeps its title (rows outside the VALUES list do not change).
  ```go
  BulkUpdate[Document](db, DocID, []momo.ColumnRef{DocTitle}, [][]any{{int64(2), "new"}}).Exec
  ```
- MERGE insert path, on the server version reported by the driver: the snippet below (`ServerVersioner`, `ServerVersion`). Without RETURNING: the snippet below must succeed and then `Count` of `title = 'merged'` is 1.
  ```go
  v := db.Driver().(momo.ServerVersioner)
  ver, err := v.ServerVersion(ctx)
  // Without RETURNING:
  Merge[Document](db, momo.Subquery("SELECT 3 AS id, 1 AS tenant_id, 'merged' AS title")).On(DocID.EqExpr(momo.TypedSQL[int64]("src.id"))).WhenNotMatchedInsert([]momo.ColumnRef{DocID, DocTenantID, DocTitle}, momo.TypedSQL[int64]("src.id"), momo.TypedSQL[int64]("src.tenant_id"), momo.TypedSQL[string]("src.title")).Exec(ctx)
  ```
- RETURNING path: when `ver >= 170000` the same MERGE with `.Returning(DocID).Rows(ctx)` returns one row whose ID is 4 after inserting `'merged-2'`; when `ver < 170000` it returns `ErrUnsupported`. CI runs `pgvector/pgvector:pg17`, so the RETURNING branch is the path it exercises; older majors in the CI matrix exercise the `ErrUnsupported` branch.
- Truncate the documents table, then `Count` is 0:

  ```go
  Truncate[Document](ctx, db)
  ```

- [ ] **Step 6: Mutation check.** Change `mergeGate` to return nil for `returning` at 150000; run `go test -run TestMergeGate .`. Expected FAIL `mergeGate(150000, true) = <nil>, want ok=false`. Restore.

- [ ] **Step 7: Commit.** `feat(core): MERGE with server-version gates, bulk update from VALUES and Truncate`

## Task 34: momovet analyzer for non-constant raw SQL (S-44)

Files:

- Create: `/Volumes/DATA/Development/momo/momovet/analyzer.go`
- Create: `/Volumes/DATA/Development/momo/momovet/analyzer_test.go`
- Create: `/Volumes/DATA/Development/momo/momovet/testdata/src/raw/raw.go`
- Create: `/Volumes/DATA/Development/momo/cmd/momovet/main.go`

Interfaces:

```go
package momovet

// Analyzer flags non-constant SQL strings passed to Momo's raw-SQL entry points:
// momo.Exec (2nd argument after ctx and q: arg index 2), momo.Raw (arg index 2),
// momo.SQL (arg index 0), momo.TypedSQL (arg index 0), momo.Subquery (arg index 0).
var Analyzer = &analysis.Analyzer{Name: "momovet", Doc: "...", Run: run}
```

- [ ] **Step 1: Write the failing test** `analyzer_test.go` and fixture `testdata/src/raw/raw.go`:

```go
// analyzer_test.go
package momovet

import (
    "testing"

    "golang.org/x/tools/go/analysis/analysistest"
)

func TestMomovet(t *testing.T) {
    analysistest.Run(t, analysistest.TestData(), Analyzer, "raw")
}
```

```go
// testdata/src/raw/raw.go
package raw

import (
    "context"

    "github.com/azrtydxb/momo"
)

func f(ctx context.Context, db *momo.DB, user string) {
    momo.Exec(ctx, db, "UPDATE t SET x = 1")          // ok: constant
    momo.Exec(ctx, db, "UPDATE t SET x = '"+user+"'") // want "non-constant SQL"
    momo.SQL("SELECT " + user)                        // want "non-constant SQL"
}
```

The fixture imports `github.com/azrtydxb/momo` from the module, so `testdata` must be inside the module (it is). The `momo.SQL` function is the spike's `SQL` raw fragment helper (added to the root in Task 10 if absent).

- [ ] **Step 2: Run and expect FAIL.** `go test ./momovet/ -run TestMomovet` → `undefined: Analyzer`.

- [ ] **Step 3: Implement** `analyzer.go` (walk `*ast.CallExpr` whose callee resolves to a `momo` function in the table, check the SQL argument with `types.Info` for constness: a `*ast.BasicLit` or a constant expression via `pass.TypesInfo.Types[arg].Value != nil` passes; otherwise report `non-constant SQL string passed to momo.<Name>`). `cmd/momovet/main.go` uses `singlechecker.Main(momovet.Analyzer)`.

- [ ] **Step 4: Run and expect PASS.** `go test ./momovet/ -race` → `ok`; `go vet -vettool=$(go build -o /dev/stdout ./cmd/momovet >/dev/null; echo) ./testdata/...` is not required; the analysistest run is the gate.

- [ ] **Step 5: Commit.** `feat(momovet): analyzer for non-constant raw SQL`

## Task 35: Shared suite groups, RunAll and the integration gate (S-4, S-20)

Files:

- Create: `/Volumes/DATA/Development/momo/internal/suite/run.go` (package `suite`)
- Create: `/Volumes/DATA/Development/momo/integration_queries_test.go` (package `momo_test`)

Interfaces (package `suite`, exact; these are the names consumed by plan momo-03 Task 9 and plan momo-08):

```go
// Queries runs the query features: joins, aggregates, subqueries, keyset, locking,
// hints, returning, raw arguments, JSON and array operators, window functions.
func Queries(t *testing.T, db *momo.DB, caps Caps)

// Writes runs the write features: insert, write states, bulk insert, COPY, update
// guardrails, optimistic lock, delete, soft delete, upsert, timestamps, defaults and
// validators, MERGE and bulk update.
func Writes(t *testing.T, db *momo.DB, caps Caps)

// Transactions is defined in Task 2 (commit, rollback, panic, savepoint, isolation).
// Errors is defined in Task 2 (error kinds and cause reachability).

// RunAll runs Transactions, Errors, EmptyResultNotNotFound (Task 11), ErrorRedaction (Task 26),
// Queries and Writes in that order, each member creating its own tables.
func RunAll(t *testing.T, db *momo.DB, caps Caps)
```

`Queries` calls each member as a named subtest on the same `db`, each starting with `CreateDocuments`:

```go
t.Run("joins", func(t *testing.T) { JoinsAliasesSelfJoin(t, db, caps) })
t.Run("aggregates", func(t *testing.T) { AggregatesGroupHaving(t, db, caps) })
t.Run("subqueries", func(t *testing.T) { SubqueriesCTEsRecursive(t, db, caps) })
t.Run("keyset", func(t *testing.T) { KeysetPaginationMixedDirections(t, db, caps) })
t.Run("locking", func(t *testing.T) { LockingNowaitSkipLocked(t, db, caps) })
t.Run("hints", func(t *testing.T) { HintsByDialect(t, db, caps) })
t.Run("table-override", func(t *testing.T) { WithTableNameQuoted(t, db, caps) })
t.Run("returning", func(t *testing.T) { ReturningRendersAndScans(t, db, caps) })
t.Run("raw", func(t *testing.T) { RawArgs(t, db, caps) })
t.Run("json-array", func(t *testing.T) { JSONAndArrayOperators(t, db, caps) })
t.Run("window", func(t *testing.T) { WindowFunctions(t, db, caps) })
```

`Writes` calls: `InsertModelOmitAndConflict`, `WriteStates`, `WriteStatesInsert`, `WriteStatesMultiRow`, `BulkInsertChunking`, `CopyInsert`, `MutationGuardrails`, `OptimisticLock`, `DeleteByModelAndGuard`, `SoftDelete`, `SoftDeleteVariants`, `UpsertUpdatesAllColumns`, `AutoTimestamps`, `DefaultsAndValidators`, `MergeAndBulkUpdate`, each as a subtest.

`RunAll` calls `EmptyResultNotNotFound` and `ErrorRedaction` directly after `Errors`; both take `(t, db, caps)`.

- [ ] **Step 1: Write the failing test** `integration_queries_test.go`:

```go
package momo_test

import (
    "testing"

    "github.com/azrtydxb/momo/internal/suite"
)

func TestIntegrationQueries(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.Queries(t, db, suite.CapsOf(db)) })
}

func TestSuiteRunAll(t *testing.T) {
    testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.RunAll(t, db, suite.CapsOf(db)) })
}
```

- [ ] **Step 2: Run and expect FAIL.** `MOMO_TEST_POSTGRES_URL=... go test -run 'TestIntegrationQueries|TestSuiteRunAll' .` → `undefined: suite.Queries`.

- [ ] **Step 3: Implement** `run.go` with `Queries`, `Writes` and `RunAll` as specified. Every group call uses the same `db` and `caps`.

- [ ] **Step 4: Run and expect PASS.** `MOMO_TEST_POSTGRES_URL=... go test -race -run 'TestIntegrationQueries|TestSuiteRunAll' .` → `ok` on both dialects. The features that a dialect lacks pass by asserting `ErrUnsupported`; none is skipped.

- [ ] **Step 5: Commit.** `test(core): shared suite groups and the integration gate`

## Spec coverage

| Spec id                                          | Tasks                                                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| S-4                                              | 3 (allowlist), 4 (joins), 5 (aggregates), 6 (subqueries and CTEs), 7 (keyset integration tests of momo-01's `After`), 8 (locking, hints, table override), 9 (returning), 10 (raw SQL), 11 (fuzzed raw args, empty IN), 35 (suite.Queries, TestIntegrationQueries). Unit-level tests (TestQueryReuseNoLeak, TestPlaceholdersAndQuoting, FuzzBuilder, TestCompileFail, TestValidExamplesCompile) are owned by momo-01 |
| S-5                                              | 12 (insert), 13 (bulk chunking), 14 (COPY), 15 (update, TestMutationGuardrails, TestOptimisticLock), 16 (TestWriteStates on UPDATE, TestWriteStatesInsert, TestWriteStatesMultiRow), 17 (delete), 18 (TestSoftDelete), 19 (upsert)                                                                                                                                                                                  |
| S-7                                              | 2 (TestTransactions)                                                                                                                                                                                                                                                                                                                                                                                                |
| S-8                                              | 2 (TestErrorClassification), 11 (TestEmptyResultNotNotFoundIntegration)                                                                                                                                                                                                                                                                                                                                             |
| S-31                                             | 22 (codecs), 23 (TestTypeMapping)                                                                                                                                                                                                                                                                                                                                                                                   |
| S-32                                             | 24 (TestJSONAndArrayOperators)                                                                                                                                                                                                                                                                                                                                                                                      |
| S-33                                             | 25 (TestWindowFunctions); top-N relation loading is momo-06                                                                                                                                                                                                                                                                                                                                                         |
| S-41                                             | 26 (TestErrorRedaction)                                                                                                                                                                                                                                                                                                                                                                                             |
| S-42                                             | 28 (TestSensitiveColumns)                                                                                                                                                                                                                                                                                                                                                                                           |
| S-43                                             | 3 (TestColumnByName, TestEscapeLike)                                                                                                                                                                                                                                                                                                                                                                                |
| S-44                                             | 34 (TestMomovet)                                                                                                                                                                                                                                                                                                                                                                                                    |
| S-48                                             | 30 (TestMaxRowsGuard, TestInListCap)                                                                                                                                                                                                                                                                                                                                                                                |
| S-68                                             | 20 (TestAutoTimestamps)                                                                                                                                                                                                                                                                                                                                                                                             |
| S-72                                             | 21 (TestDefaultsAndValidators)                                                                                                                                                                                                                                                                                                                                                                                      |
| S-73                                             | 22 (TestCodecsRoundTrip)                                                                                                                                                                                                                                                                                                                                                                                            |
| S-75                                             | 18 (TestSoftDeleteVariants)                                                                                                                                                                                                                                                                                                                                                                                         |
| S-77                                             | 10 (TestRawArgs), 11 (FuzzRawArgs)                                                                                                                                                                                                                                                                                                                                                                                  |
| S-78                                             | 31 (TestResultHelpers)                                                                                                                                                                                                                                                                                                                                                                                              |
| S-79                                             | 8 (locking, hints, table override), 32 (TestCompositionHelpers)                                                                                                                                                                                                                                                                                                                                                     |
| S-80                                             | 33 (TestMergeAndBulkUpdate, TestMergeGate)                                                                                                                                                                                                                                                                                                                                                                          |
| S-85                                             | 27 (TestModelHooks)                                                                                                                                                                                                                                                                                                                                                                                                 |
| S-86                                             | 29 (TestInterceptors)                                                                                                                                                                                                                                                                                                                                                                                               |
| Shared suite (plan momo-03 Task 9, plan momo-08) | 1 (package and Caps), 2, 35 (Queries, Writes, Transactions, Errors, RunAll)                                                                                                                                                                                                                                                                                                                                         |

## Dependencies on other plans

- momo-01-foundation provides: `internal/testdb` URL helpers (`PostgresURL`, `PostgresAppRoleURL`, `PgBouncerURL`, `SQLitePath`); `Meta[T]` with `CreatedAt`, `UpdatedAt`, `Hooks HookSet[T]`, `errors.go` sentinels (`ErrValidation`, `ErrLimitExceeded`, `ErrBusy`, `ErrUnfilteredMutation`, `ErrOptimisticLock`, `ErrNotFound`, `ErrUnsupported`), `dialect` capability flags, `internal/testdb/testopen` (`Each`, `Postgres` from momo-03 Task 1; `SQLite` from momo-03 Task 14), with `suite.CapsOf` and `suite.CapabilitiesOf` defined here (Task 1), and the unit-level S-4 tests listed above.
- momo-03-drivers provides `pgxdriver` and `sqlitedriver`, `ClassifyError`, `momo.Copier`, `momo.Batcher`, `momo.PoolStater`, `momo.CapabilityReporter`, `momo.ServerVersioner` (Task 13 of momo-03), and the `TestPgBouncerTransactionMode` test that calls `suite.RunAll` through the pooler.
- momo-02-generator emits `MomoMeta` for real models; this plan's fixtures are hand-written, so it does not depend on the generator.
- momo-08 (MySQL/MariaDB) consumes `suite.RunAll` with its own `Caps`.
