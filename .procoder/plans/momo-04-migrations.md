# momo-04-migrations — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

Scope ids: S-10, S-11, S-12, S-13, S-14, S-27, S-28, S-36, S-37, S-38, S-39, S-46, S-47, S-74, S-26 (CLI path separators only: TestWindowsPaths); plus the migration parts of S-22 (snapshot `format_version`, `momo.sum` header, `momo_migrations` metadata row) and the normalization part of S-1 (`schema/normalize.go`, `FuzzNormalize`).

Research cited in this plan (verified against the official pages on 2026-11-04):

- SQLite 12-step table rebuild and supported ALTER TABLE operations: https://www.sqlite.org/lang_altertable.html
- PostgreSQL CREATE INDEX CONCURRENTLY (no transaction block; failure leaves INVALID index; recovery by drop and retry): https://www.postgresql.org/docs/current/sql-createindex.html
- PostgreSQL ALTER TYPE ... ADD VALUE (may run in a transaction block; the new value is unusable until commit): https://www.postgresql.org/docs/current/sql-altertype.html
- PostgreSQL catalogs used by inspection: pg_class, pg_attribute, pg_constraint, pg_index, pg_am, pg_opclass, pg_enum, pg_extension, pg_policy, pg_trigger, pg_proc, pg_partitioned_table, pg_depend, and the functions pg_get_expr, pg_get_indexdef, pg_get_viewdef, pg_get_partkeydef, pg_get_function_arguments, pg_get_function_result: https://www.postgresql.org/docs/current/catalogs.html and https://www.postgresql.org/docs/current/functions-info.html
- PostgreSQL advisory locks (session-level pg_try_advisory_lock and pg_locks classid/objid split for bigint keys): https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS
- PostgreSQL SET lock_timeout / statement_timeout (integer values are milliseconds): https://www.postgresql.org/docs/current/runtime-config-client.html
- SQLite PRAGMA table_xinfo, index_list, index_xinfo, foreign_key_list, foreign_key_check: https://www.sqlite.org/pragma.html
- Format compatibility for importers (goose, golang-migrate, bun, Atlas): the documentation pages did not expose the table layouts reliably at planning time (two returned 404), so the importer tasks in plan momo-11 pin the layouts by reading the library source in the Go module cache of the pinned versions and cite the file path there.

## Goal

Ship a self-contained, reviewable migration engine (Go API and `momo migrate` CLI) for PostgreSQL and SQLite that diffs models against versioned snapshots, plans and applies SQL under a lock with checksummed history and documented recovery, and never guesses renames, drops or destructive changes.

## Architecture

The pure core (`migrate` diff, ordering, file/checksum/snapshot handling, lint, chain validation) has no database and is tested offline. Each dialect (`migrate/postgres`, `migrate/sqlite`) implements the `migrate.Dialect` interface: inspect a pinned connection into a `schema.Schema`, render `migrate.Change` values to statements, and take the migration lock; the engine owns history, ordering, transactions and recovery and runs everything on one pinned connection so session state (advisory locks, `lock_timeout`, `PRAGMA foreign_keys`) is reliable. Managed objects are diffed against the latest snapshot, never against the live database, so unmanaged database objects are reported by drift and never appear in a plan.

## Constraints

- Go: minimum 1.26 in go.mod; CI tests 1.26 and 1.27. Dependencies pinned in go.mod/go.sum.
- SQLite driver: mattn/go-sqlite3 v1.14.52 (cgo) with sqlite-vec v0.1.6 statically linked via github.com/asg017/sqlite-vec-go-bindings/cgo; SQLite builds require cgo and a C toolchain. The database/sql adapter itself stays driver-agnostic.
- PostgreSQL driver: jackc/pgx v5 (pinned) via pgxpool, plus pgx stdlib for *sql.DB adoption.
- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- Persisted formats carry explicit format versions independent of the module version: schema snapshot JSON (`format_version`), the checksum file header, the `momo_migrations` history table (metadata row), and generated-file headers (generator version). A newer Momo reads every older format; an older Momo refuses a newer format with an actionable error instead of misreading it.

## Build order and test harness (round 2)

Tasks run in numeric order. Tasks 1 to 9 are pure and need no database. Tasks 10 to 14 are the PostgreSQL dialect (inspection, indexes, objects, rendering, lock, session, history DDL); the engine-level tasks 15 to 32 need them, because engine tests run on PostgreSQL through `testopen.Postgres(t)`. Tasks 33 to 36 are SQLite and come last. When the SQLite driver lands (M2), the engine-level tests are wrapped in `testopen.Each(t, ...)` without changing their bodies. Every integration command assumes `MOMO_TEST_POSTGRES_URL` is exported (and `MOMO_TEST_POSTGRES_APP_URL` for tasks 23 and 24).

## Contract changes required (not made here; reported to the lead)

docs/ARCHITECTURE.md is binding and this plan does not edit it. These additions are needed, and each task below uses the exact names listed here:

1. `schema.Schema` gets the fields in ARCHITECTURE's list but the spike lacks them. The types are defined in Task 1 (`View`, `Function`, `Trigger`, `Policy`, `Partition`, `PartitionOf`, `RLS`, `SchemaNamespace`), plus `Table.Schema`. `schema.FormatVersion` is set to 1 by Task 1.
2. `migrate.Dialect` gets three methods in addition to Inspect/Render/Lock: `Dialect() *dialect.Dialect`, `HistoryDDL() []string`, `Setup(ctx, momo.PinnedConn, SessionOptions) (reset func() error, err error)`. Setup applies lock_timeout/statement_timeout (PostgreSQL) or PRAGMA foreign_keys handling (SQLite).
3. `momo.PinnedConn` is the type the migrate Dialect methods receive. ARCHITECTURE defines it and plan momo-03 Task 2 implements it, with exactly these methods: `Query(ctx context.Context, sql string, args []any) (momo.Rows, error)`, `Exec(ctx context.Context, sql string, args []any) (int64, error)`, `Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error)` and `Release() error`. A connection is obtained with `db.Driver().(momo.Pinner).Pin(ctx)`; `*momo.DB` has no `Pin` method. This plan uses these names and signatures unchanged.
4. `momo.Driver` exposes `Dialect() *dialect.Dialect`. Plan momo-01 must confirm this.
5. The history table gets two more columns: `apply_id` (groups migrations applied by one `migrate apply` invocation, used by group rollback in S-94) and `kind` (`sql` or `go`). The ARCHITECTURE column list is extended to `momo_migrations(version, name, checksum, status, statements_applied, error, applied_at, execution_ms, apply_id, kind)`.
6. CLI additions beyond the table in ARCHITECTURE/spec: `migrate diff --stage`, `migrate apply --force-unlock`, `migrate rollback [--group]`, `migrate new --custom <name>`, `migrate rebase --onto <dir-or-version>` (no git: the user runs git), and `--config <path>` on every command. Exit codes: 0 success, 1 policy findings (drift, validate, lint errors, destructive refusal, staged-change required, generate --check stale), 2 usage or configuration error, 3 database, lock or execution error.
7. `gen/config.File` (plan momo-02) has the field `Migrate MigrateConfig`; `MigrateConfig` is defined in package `gen/config` by plan momo-02, and this plan owns only its semantics. Field list is in Task 3. The CLI loads `momo.yaml` through `config.Load` (plan momo-02) and uses no other parser. `TestWindowsPaths` (Task 29) also depends on `gen.ImportPath` from plan momo-02.
8. `internal/testdb` (plan momo-01) provides URLs, paths and the per-test schema name only: `PostgresURL(t)` (memoized per `t`, search_path `t_xxx,public`), `Schema(t)` (returns `t_xxx`), `PostgresAppRoleURL(t)` (same schema; the harness grants the app role USAGE, default DML privileges and no CREATE), `SQLitePath(t)`. `*momo.DB` handles come from `internal/testdb/testopen` (plan momo-03): `testopen.Postgres(t)`, `testopen.SQLite(t)`, `testopen.Each`. Engine-level tests use `testopen.Postgres(t)` first and are wrapped in `testopen.Each` when SQLite lands (M2).
9. `migrate.WithDialect(d Dialect) Option` replaces any dialect registry. `NewEngine` without it returns an error naming the option. The CLI selects the dialect by a static switch on the configured name.
10. `func (db *DB) OnConn(pc momo.PinnedConn) *momo.DB` (owned by plan momo-03) is needed so that a non-transactional Go migration receives a `momo.Querier` on the pinned connection; it keeps db's hooks and classifier (plan momo-11 Task 13). Plan momo-03 Task 7 provides it.
11. Views and constraints (Task 32): `momo.View` marker, `WritableModeler[T]`, `Insert/Update/Delete[T, PT WritableModeler[T]]`, `RefreshMaterializedView`, and the MomoConfig methods `c.View`, `c.MaterializedView`, `c.Function`, `c.Trigger`, `c.Policy`, `c.EnableRLS` consumed from plan momo-02.
12. Statement hints `fk-check` and `dependent:<type>:<name>` (Task 35) and the history semantics of Task 15 (rolled_back is not dirty). `rebase --onto` takes a directory or a version; no git.

## Directory and file layout (owned by this plan)

- `schema/types.go` (Task 1; extends the spike `schema/schema.go`), `schema/normalize.go` (Task 2), `schema/normalize_test.go`, `schema/normalize_fuzz_test.go`.
- `migrate/types.go`, `migrate/errors.go`, `migrate/options.go` (Task 3).
- `migrate/files.go`, `migrate/split.go` (Task 4).
- `migrate/sum.go` (Task 5). `migrate/snapshot.go` (Task 6). `migrate/chain.go`, `migrate/validate.go` (Task 7).
- `migrate/diff.go` (Task 8), `migrate/order.go` (Task 9).
- `migrate/sqlite/inspect.go`, `migrate/sqlite/render.go`, `migrate/sqlite/rebuild.go`, `migrate/sqlite/lock.go`, `migrate/sqlite/dialect.go` (Tasks 33 to 36).
- `migrate/runner.go` (Task 15), `migrate/engine.go` (Tasks 15 to 20 and 27), `migrate/history.go` (Task 15), `migrate/automigrate.go` (Task 20).
- `migrate/postgres/inspect.go`, `migrate/postgres/inspect_index.go`, `migrate/postgres/inspect_objects.go`, `migrate/postgres/render.go`, `migrate/postgres/lock.go`, `migrate/postgres/dialect.go` (Tasks 10 to 22).
- `tx_scope.go` in the `momo` root package (Task 23; the root package file list is owned by plan momo-01, this file is added to it).
- `migrate/lint.go` (Task 26). `migrate/rebase.go` (Task 27).
- `cmd/momo/migrate.go`, `cmd/momo/migrate_redact.go`, `cmd/momo/migrate_test.go` (Tasks 28 and 29).
- `migrate/helpers_test.go` (Task 3), `migrate/sqlite/helpers_test.go` (Task 35), `migrate/postgres/helpers_test.go` (Task 3), `migrate/custom_test.go` (Task 21), `migrate/format_history_test.go` and `migrate/format_versions_test.go` (Task 30), `migrate/testdata/format0/` (Task 30).
- `views.go`, `views_compile_test.go` (root package, Task 32), `internal/compat/viewmodels/` (Task 32), `testdata/compilefail/view_insert/` (Task 32), `migrate/postgres/views_test.go` (Task 32).

Shared test helpers: `migrate/migratetest/migratetest.go` (Task 3) holds `WriteFiles(t, dir, map[string]string)`, `CountingDriver` (wraps momo.Driver and counts statements), and `Schema(t, dsl)` helpers used by later tasks.

## Task 1: Schema types for snapshots and managed objects

Covers: S-1 (types), S-36 (schema qualification), S-37 (views), S-38 (policies, RLS flags), S-39 (functions, triggers, partitions), S-22 (FormatVersion). Acceptance: TestSchemaJSONRoundTrip (must still pass), TestSchemaFormatVersion.

Files:

- Modify: `schema/schema.go` (spike) to add the fields listed below.
- Create: `schema/types.go` is NOT created; the new types go in `schema/schema.go` to keep one file for the struct graph.
- Create: `schema/format_test.go`.

Interfaces (exact):

```go
const FormatVersion = 1

type Schema struct {
    FormatVersion int               `json:"format_version"`
    Dialect       string            `json:"dialect"`
    Schemas       []SchemaNamespace `json:"schemas,omitempty"` // PostgreSQL namespaces other than the connection's current schema (which is implicit)
    Extensions    []Extension       `json:"extensions,omitempty"`
    Enums         []Enum            `json:"enums,omitempty"`
    Tables        []Table           `json:"tables"`
    Views         []View            `json:"views,omitempty"`
    Functions     []Function        `json:"functions,omitempty"`
    Triggers      []Trigger         `json:"triggers,omitempty"`
    Policies      []Policy          `json:"policies,omitempty"`
}
type SchemaNamespace struct{ Name string `json:"name"` }

// Table gains:
//   Schema      string       `json:"schema,omitempty"`      // empty means the connection's current_schema() (PostgreSQL) or main (SQLite)
//   RLS         *RLS         `json:"rls,omitempty"`         // nil means row-level security is off
//   Partition   *Partition   `json:"partition,omitempty"`   // parent of a partitioned table
//   PartitionOf *PartitionOf `json:"partition_of,omitempty"` // child partition
//   ForeignKeys[i].NoConstraint is NOT a schema field (see Task 8: it is a model-side option that never reaches the schema)

type RLS struct {
    Enabled bool `json:"enabled"`
    Forced  bool `json:"forced"`
}
type Partition struct {
    Strategy string   `json:"strategy"` // "range", "list", "hash"
    Key      string   `json:"key"`      // pg_get_partkeydef text, e.g. "RANGE (created_at)"
}
type PartitionOf struct {
    Parent string `json:"parent"`
    Bound  string `json:"bound"` // e.g. "FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')"
}
type View struct {
    Schema       string   `json:"schema,omitempty"`
    Name         string   `json:"name"`
    Materialized bool     `json:"materialized,omitempty"`
    Query        string   `json:"query"`            // SELECT text only, no CREATE VIEW prefix
    Depends      []string `json:"depends,omitempty"` // tables and views this view reads: bare name when in the current schema, otherwise "schema.name"
    Indexes      []Index  `json:"indexes,omitempty"` // materialized views only
}
type Function struct {
    Schema     string `json:"schema,omitempty"`
    Name       string `json:"name"`
    Args       string `json:"args"`       // pg_get_function_arguments text
    Returns    string `json:"returns"`    // pg_get_function_result text
    Language   string `json:"language"`   // "plpgsql", "sql", ...
    Volatility string `json:"volatility"` // "VOLATILE", "STABLE", "IMMUTABLE"
    Body       string `json:"body"`       // function body, rendered with dollar-quote tag $momo$
}
type Trigger struct {
    Schema     string   `json:"schema,omitempty"`
    Table      string   `json:"table"`
    Name       string   `json:"name"`
    Timing     string   `json:"timing"`     // "BEFORE", "AFTER", "INSTEAD OF" (PostgreSQL); SQLite uses Definition
    Events     []string `json:"events,omitempty"` // "INSERT","UPDATE","DELETE","TRUNCATE"
    ForEachRow bool     `json:"for_each_row,omitempty"`
    When       string   `json:"when,omitempty"`
    Function   string   `json:"function,omitempty"` // qualified function name
    Args       []string `json:"args,omitempty"`
    Definition string   `json:"definition,omitempty"` // SQLite only: the full CREATE TRIGGER text as stored in sqlite_schema
}
type Policy struct {
    Schema     string   `json:"schema,omitempty"`
    Table      string   `json:"table"`
    Name       string   `json:"name"`
    Command    string   `json:"command"` // "ALL", "SELECT", "INSERT", "UPDATE", "DELETE"
    Permissive bool     `json:"permissive"`
    Roles      []string `json:"roles,omitempty"` // empty means PUBLIC
    Using      string   `json:"using,omitempty"`
    Check      string   `json:"check,omitempty"`
}
```

Steps:

- [ ] **Step 1: Write the failing format test**

Create `schema/format_test.go`:

```go
package schema

import (
    "testing"
	"errors"
	"strings"
	"bytes"
)

func TestSchemaFormatVersion(t *testing.T) {
    s := &Schema{FormatVersion: FormatVersion, Dialect: "postgres", Tables: []Table{{Name: "t"}}}
    var buf bytes.Buffer
    if err := s.Write(&buf); err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(buf.String(), `"format_version": 1`) {
        t.Fatalf("snapshot does not carry format_version: %s", buf.String())
    }
    future := strings.Replace(buf.String(), `"format_version": 1`, `"format_version": 99`, 1)
    _, err := Load(strings.NewReader(future))
    if !errors.Is(err, ErrNewerFormat) {
        t.Fatalf("want ErrNewerFormat, got %v", err)
    }
    if !strings.Contains(err.Error(), "upgrade momo") {
        t.Fatalf("error must name the required Momo version action: %v", err)
    }
}
```

Add `"errors"` to the import list. `Write` is a new method: `func (s *Schema) Write(w io.Writer) error` writes indented JSON with a trailing newline. Run: `go test ./schema/ -run TestSchemaFormatVersion -count=1`. Expect FAIL with `s.Write undefined` or `undefined: FormatVersion`.

- [ ] **Step 2: Implement the types**

Add the fields and types above to `schema/schema.go`. Implement:

```go
var ErrNewerFormat = errors.New("schema: snapshot format is newer than this momo; upgrade momo to read it")

func Load(r io.Reader) (*Schema, error) {
    var s Schema
    dec := json.NewDecoder(r)
    if err := dec.Decode(&s); err != nil {
        return nil, fmt.Errorf("schema: decode: %w", err)
    }
    if s.FormatVersion > FormatVersion {
        return nil, fmt.Errorf("%w (snapshot format_version %d, this momo reads up to %d)", ErrNewerFormat, s.FormatVersion, FormatVersion)
    }
    return &s, nil
}

func (s *Schema) Write(w io.Writer) error {
    s.FormatVersion = FormatVersion
    b, err := json.MarshalIndent(s, "", "  ")
    if err != nil {
        return err
    }
    _, err = w.Write(append(b, '\n'))
    return err
}
```

Keep `Sort()` and extend it to sort `Schemas`, `Views`, `Functions`, `Triggers`, `Policies`, and each view's `Indexes`, each table's `Indexes`, and `Depends` (sorted, deduplicated).

- [ ] **Step 3: Run the format test, expect PASS**

Run: `go test ./schema/ -count=1`. Expect `ok`. The existing `TestSchemaJSONRoundTrip` and `FuzzSchemaSort` from plan momo-01 must still pass.

- [ ] **Step 4: Commit**

`git add schema && git commit -m "feat(schema): add managed-object types and format_version to snapshots"`

## Task 2: Schema normalization and FuzzNormalize

Covers: S-1 (normalization), S-11 (no-change round trip depends on it). Acceptance: TestNormalizeTypes, TestNormalizeExprDBRewrites, TestEqualIgnoresOrder, FuzzNormalize.

Files:

- Create: `schema/normalize.go`, `schema/normalize_test.go`, `schema/normalize_fuzz_test.go`.

Interfaces (exact):

```go
func NormalizeType(d dialect.Name, t string) string
func NormalizeExpr(d dialect.Name, e string) string
func Normalized(s *Schema) *Schema   // deep copy, types and expressions normalized, Sort() applied
func Equal(a, b *Schema) bool         // compares JSON of Normalized(a) and Normalized(b)
```

Steps:

- [ ] **Step 1: Failing table test**

`schema/normalize_test.go`:

```go
package schema

import (
    "testing"
    "github.com/azrtydxb/momo/dialect"
)

func TestNormalizeTypes(t *testing.T) {
    cases := []struct{ d dialect.Name; in, want string }{
        {dialect.Postgres, "int4", "integer"},
        {dialect.Postgres, "int8", "bigint"},
        {dialect.Postgres, "int2", "smallint"},
        {dialect.Postgres, "bool", "boolean"},
        {dialect.Postgres, "float8", "double precision"},
        {dialect.Postgres, "varchar(255)", "character varying(255)"},
        {dialect.Postgres, "timestamptz", "timestamp with time zone"},
        {dialect.Postgres, "timestamp", "timestamp without time zone"},
        {dialect.Postgres, "decimal(10,2)", "numeric(10,2)"},
        {dialect.Postgres, "int4[]", "integer[]"},
        {dialect.Postgres, "vector(1536)", "vector(1536)"},
        {dialect.SQLite, "bigint", "BIGINT"},
        {dialect.SQLite, "varchar( 255 )", "VARCHAR(255)"},
    }
    for _, c := range cases {
        if got := NormalizeType(c.d, c.in); got != c.want {
            t.Errorf("NormalizeType(%s, %q) = %q, want %q", c.d, c.in, got, c.want)
        }
    }
}

func TestNormalizeExprDBRewrites(t *testing.T) {
    cases := []struct{ in, want string }{
        {"'active'::character varying", "'active'"},
        {"('now'::text)", "now()"},
        {"CURRENT_TIMESTAMP", "now()"},
        {"  (lower( name ))  ", "lower(name)"},
        {"((a > 0))", "a > 0"},
    }
    for _, c := range cases {
        if got := NormalizeExpr(dialect.Postgres, c.in); got != c.want {
            t.Errorf("NormalizeExpr(%q) = %q, want %q", c.in, got, c.want)
        }
    }
}

func TestEqualIgnoresOrder(t *testing.T) {
    a := &Schema{Dialect: "postgres", Tables: []Table{{Name: "b"}, {Name: "a"}}}
    b := &Schema{Dialect: "postgres", Tables: []Table{{Name: "a"}, {Name: "b"}}}
    if !Equal(a, b) {
        t.Fatal("schemas differing only in table order must be equal")
    }
}
```

Run: `go test ./schema/ -run 'TestNormalize|TestEqual' -count=1`. Expect FAIL with `undefined: NormalizeType`.

- [ ] **Step 2: Implement**

`NormalizeType` for PostgreSQL: trim and lowercase the base name, strip the parameter list before mapping and re-append it, apply the alias table `{int→integer, int4→integer, int8→bigint, int2→smallint, bool→boolean, float4→real, float8→double precision, varchar→character varying, char→character, decimal→numeric, timestamptz→timestamp with time zone, timestamp→timestamp without time zone, timetz→time with time zone, time→time without time zone}`, and handle array suffix `[]` by recursing on the element. Collapse internal whitespace. For SQLite: `strings.ToUpper`, remove spaces inside parentheses. Unknown names pass through trimmed.

`NormalizeExpr` for PostgreSQL: repeat until stable: trim spaces; remove one layer of outer parentheses when they wrap the whole expression; remove `::<type>` casts that follow a string literal (regex on `'[^']*'::[a-z][a-z0-9 _\[\]]*`, keep the literal); map `CURRENT_TIMESTAMP` to `now()`; collapse runs of whitespace outside single-quoted literals to one space; lowercase identifiers that are unquoted keywords only when they are the function-name tokens listed in the table. For SQLite return the trimmed, whitespace-collapsed text. Never change text inside single-quoted literals.

`Normalized(s)` deep-copies via JSON round trip (`json.Marshal` then `json.Unmarshal`), then walks every column type, default, generated expression, check expression, index `Where`, index part `Expr`, view `Query`, function `Body`, trigger `When`, policy `Using` and `Check`, and applies the normalizers, then calls `Sort()`.

- [ ] **Step 3: Run, expect PASS**

Run: `go test ./schema/ -run 'TestNormalize|TestEqual' -count=1`. Expect `ok`.

- [ ] **Step 4: Failing fuzz target**

`schema/normalize_fuzz_test.go`:

```go
package schema

import (
    "testing"
    "github.com/azrtydxb/momo/dialect"
	"strings"
)

func FuzzNormalize(f *testing.F) {
    for _, s := range []string{"int4", "varchar(255)", "'a'::text", "((a))", "CURRENT_TIMESTAMP", "'x''y'", ""} {
        f.Add(s)
    }
    f.Fuzz(func(t *testing.T, in string) {
        for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
            once := NormalizeExpr(d, in)
            if twice := NormalizeExpr(d, once); twice != once {
                t.Fatalf("NormalizeExpr not idempotent for %q: %q then %q", in, once, twice)
            }
            if once != strings.TrimSpace(once) {
                t.Fatalf("NormalizeExpr left surrounding whitespace: %q", once)
            }
            tOnce := NormalizeType(d, in)
            if NormalizeType(d, tOnce) != tOnce {
                t.Fatalf("NormalizeType not idempotent for %q", in)
            }
        }
    })
}
```

Run: `go test ./schema/ -run FuzzNormalize -count=1`. Expect FAIL before Step 2 if stubs panic. Then after Step 2 run `go test ./schema/ -run FuzzNormalize -fuzz=FuzzNormalize -fuzztime=30s`. Expect no failing input. Any crashing input is saved under `schema/testdata/fuzz/FuzzNormalize/` and must be fixed in `NormalizeExpr`, then kept as a regression seed.

- [ ] **Step 5: Commit**

`git commit -am "feat(schema): normalize DB-rewritten types and expressions"` (paths: `schema/normalize*.go`, `schema/testdata`).

## Task 3: Migrate core types, errors, options, Dialect interface, test helpers

Covers: S-10 (Engine API surface), S-8 (migration sentinels). Acceptance: TestCLIContracts depends on these names; compile-level test only here.

Files:

- Create: `migrate/types.go`, `migrate/errors.go`, `migrate/options.go`, `migrate/dialect.go`, `migrate/migratetest/migratetest.go`, `migrate/types_test.go`, `migrate/helpers_test.go`, `migrate/postgres/helpers_test.go`.
- Consumes (does not edit): `gen/config` from plan momo-02 (`config.File`, `MigrateConfig`, `config.Load`); if momo-02 has not landed it, this task is blocked: see Interfaces.

Interfaces (exact):

```go
// migrate/errors.go
var (
    ErrChecksumMismatch = errors.New("migrate: checksum mismatch")
    ErrDirty            = errors.New("migrate: database has a dirty migration; run momo migrate resolve after manual repair")
    ErrLocked           = errors.New("migrate: migration lock is held by another process")
    ErrDestructive      = errors.New("migrate: destructive change requires --allow-destructive or a -- momo:destructive directive")
    ErrBranchConflict   = errors.New("migrate: two migrations share the same parent snapshot")
    ErrStagedRequired   = errors.New("migrate: NOT NULL column without default on a populated table requires a staged change (--stage)")
    ErrNoDown           = errors.New("migrate: migration has no authored down file")
    ErrNoSnapshot       = errors.New("migrate: no snapshot found for the requested version")
    ErrRebuildBlocked   = errors.New("migrate: table rebuild blocked by an object that references a dropped column")
)

// migrate/types.go
type Kind string // "sql" or "go"
type Statement struct {
    SQL   string
    NoTx  bool // statement must run outside a transaction (CREATE INDEX CONCURRENTLY)
    Hint  string
}
type Change struct {
    Kind        ChangeKind
    Schema      string
    Table       string
    Name        string // object name, or new name for renames
    Before      any    // *schema.Column, *schema.Index, *schema.View, ... (kind-specific)
    After       any
    Destructive bool
    Rebuild     bool   // SQLite: requires 12-step rebuild
}
type ChangeKind string // constants: see the Change kind table in Task 8
type Migration struct {
    Version     string // 14-digit UTC timestamp
    Name        string
    Kind        Kind
    Parent      string
    Up          []Statement
    Down        []Statement
    NoTx        bool
    Destructive bool
    Changes     []Change
    Snapshot    *schema.Schema
    Files       map[string][]byte // relative file name -> content, for the CLI to write
    Hints       []string
}
type Pending struct {
    Version string
    Name    string
    Kind    Kind
    Statements []Statement
    NoTx    bool
}
type State string // "pending", "applied", "dirty", "mismatch", "missing"
type VersionStatus struct {
    Version, Name string
    State         State
    Checksum      string
    Error         string
}
type Applied struct {
    Version    string
    Statements int
    Duration   time.Duration
}
type Report struct{ Applied []Applied; ApplyID string }
type ApplyOptions struct {
    To         string // empty: all pending
    ForceUnlock bool
    LockWait   time.Duration // default 30s
}
type ResolveState string // "applied" or "rolled_back"
type Severity string     // "error", "warning"
type Finding struct {
    Severity    Severity
    Rule        string
    Version     string
    Statement   int
    Message     string
    Alternative string
}
type ObjectRef struct{ Kind, Schema, Name string }
type ObjectDiff struct{ Object ObjectRef; Detail string }
type DriftReport struct {
    Changed   []ObjectDiff
    Missing   []ObjectRef
    Unmanaged []ObjectRef
}
func (d *DriftReport) Clean() bool
type Skipped struct{ Change Change; Reason string }
type AutoReport struct {
    Applied []Change
    Skipped []Skipped
}
type DiffOptions struct {
    RenameTables    map[string]string // old -> new
    RenameColumns   map[string]string // "table.old" -> "new"
    AllowDestructive bool
    Stage           bool
    Name            string // not used; name is passed to Diff
}
type Config struct {
    // Schemas lists the PostgreSQL schemas the engine manages. Default (nil or empty): the single schema
    // returned by `SELECT current_schema()` on the pinned connection, resolved when the engine starts
    // (Plan/Apply/Status/Diff/Drift/Inspect each resolve it once per call). An app with search_path=app
    // therefore migrates `app` by default. Schemas[0] hosts the history table (see Task 14) and must equal
    // current_schema(); otherwise the engine returns a configuration error naming both.
    Schemas        []string
    LockTimeout    time.Duration // default 5s, PostgreSQL only
    StatementTimeout time.Duration // default 0 (unset)
    Exclude        []string      // glob patterns for unmanaged objects (drift)
}
type Option func(*options)
func WithClock(now func() time.Time) Option
func WithConfig(c Config) Option
func WithDialect(d Dialect) Option // required by NewEngine; NewEngine without it returns an error naming migrate.WithDialect
type SessionOptions struct {
    LockTimeout, StatementTimeout time.Duration
}

// migrate/dialect.go
type InspectOptions struct{ Schemas []string; Exclude []string }
type Dialect interface {
    Dialect() *dialect.Dialect
    Inspect(ctx context.Context, c momo.PinnedConn, opts InspectOptions) (*schema.Schema, error)
    Render(plan []Change) ([]Statement, error)
    HistoryDDL() []string
    Setup(ctx context.Context, c momo.PinnedConn, s SessionOptions) (reset func() error, err error)
    Lock(ctx context.Context, c momo.PinnedConn, key string) (unlock func() error, holder func(context.Context) (string, error), err error)
}

// migrate/migratetest/migratetest.go
func WriteFiles(t *testing.T, dir string, files map[string]string)
type CountingDriver struct{ momo.Driver; Count atomic.Int64 }
func NewEngine(t *testing.T, d migrate.Dialect, drv momo.Driver, dir fs.FS, opts ...migrate.Option) *migrate.Engine // t.Fatal on error; always passes WithDialect(d)
func WithSum(t *testing.T, files fstest.MapFS) fstest.MapFS // adds momo.sum computed by ComputeSum (Task 5)
func EmptyFS() fs.FS
func FS(files map[string][]byte) fs.FS
func Merge(base fs.FS, add map[string][]byte) fs.FS
func MergeMaps(a, b map[string][]byte) map[string][]byte
func FSFromDiff(t *testing.T, db *momo.DB, desired *schema.Schema, name string) fs.FS // Engine.Diff on an empty directory; returns the written files
func Diff(a, b *schema.Schema) string // JSON difference for failure messages
```

Gen config contract (for momo-02): `gen/config.File` has the field ``Migrate MigrateConfig `yaml:"migrate"` ``, and `MigrateConfig` is DEFINED in package `gen/config` (plan momo-02 owns the type; plan 04 owns its semantics). The shape, for reference:

```go
type MigrateConfig struct {
    Dir         map[string]string `yaml:"dir"`          // dialect -> directory, e.g. postgres: migrations/postgres
    URLEnv      map[string]string `yaml:"url_env"`      // dialect -> environment variable name
    Schemas     []string          `yaml:"schemas"`      // default: current_schema()
    LockTimeout string            `yaml:"lock_timeout"` // Go duration string
    StatementTimeout string       `yaml:"statement_timeout"`
    Exclude     []string          `yaml:"exclude"`
}
```

The accepted `momo.yaml` shape is exactly `migrate: {dir: {postgres: ...}, url_env: {postgres: ...}, schemas, lock_timeout, statement_timeout, exclude}`. If momo-02 has not landed `gen/config` when this task runs, this step is blocked and the implementer must not edit momo-02's files; report back.

Steps:

Test helper files, all checking every error with t.Fatal: `migrate/helpers_test.go` (package migrate_test): `pinned(t, *momo.DB) momo.PinnedConn`, `mustExec(t, execer, sql)`, `mustQueryRow(t, querier, sql, dest ...any)`, `historyRow(t, *momo.DB, version) migrate.HistoryRow`, `tableExists(t, *momo.DB, name) bool`, `columnExists(t, *momo.DB, table, col) bool`, `countDDL(t, *momo.DB) int` (through CountingDriver), `migratetestFS(t, version) fs.FS`, `mpg.New() migrate.Dialect`. `migrate/postgres/helpers_test.go` (package postgres_test): the same names for PostgreSQL, plus `tableExistsIn(t, *momo.DB, schema, name) bool`, `openPG(t, dsn) *momo.DB`, `openAppRole(t, testdb.PostgresAppRoleURL(t)) *momo.DB`. The PostgreSQL helpers filter on the per-test schema through `current_schema()`, because tests share one server and each test has its own schema (ARCHITECTURE, Per-test isolation):

```go
// tableExists (PostgreSQL):
//   SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)
// columnExists (PostgreSQL):
//   SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2)
// tableExistsIn(t, db, schema, name): same as tableExists with `table_schema = $1 AND table_name = $2`.
// SQLite equivalents (migrate/sqlite/helpers_test.go, Task 35): SELECT count(*) FROM sqlite_master WHERE type='table' AND name = ?  and  SELECT count(*) FROM pragma_table_info(?) WHERE name = ?.
// MySQL equivalents (when that dialect lands): table_schema = database() instead of current_schema().
```

`mustQueryRow` needs `momo.Rows` with Next, Scan, Close and Err (plan momo-01 contract). `migrate/sqlite/helpers_test.go` is created in Task 35.

- [ ] **Step 1: Failing compile test**

`migrate/types_test.go`:

```go
package migrate

import (
    "testing"
	"errors"
)

func TestSentinelsAreDistinct(t *testing.T) {
    all := []error{ErrChecksumMismatch, ErrDirty, ErrLocked, ErrDestructive, ErrBranchConflict, ErrStagedRequired, ErrNoDown, ErrNoSnapshot, ErrRebuildBlocked}
    for i := range all {
        for j := range all {
            if i != j && errors.Is(all[i], all[j]) {
                t.Fatalf("sentinels %d and %d are equal", i, j)
            }
        }
    }
    var d DriftReport
    if !d.Clean() {
        t.Fatal("empty drift report must be clean")
    }
}
```

Run: `go test ./migrate/ -run TestSentinelsAreDistinct -count=1`. Expect FAIL with `undefined: ErrChecksumMismatch`.

- [ ] **Step 2: Implement the files listed in Interfaces**

Write exactly the declarations above. `DriftReport.Clean()` returns true when all three slices are empty. `WithClock` default is `time.Now().UTC`. `WithConfig` default values: Schemas empty (resolved to `[current_schema()]` at engine start, never hard-coded), LockTimeout 5s. The engine helper `resolveSchemas(ctx, pinned, cfg) ([]string, error)` runs `SELECT current_schema()` (PostgreSQL dialect only; SQLite and MySQL ignore Schemas) when `cfg.Schemas` is empty and returns an error if the result is NULL (no schema on the search_path).

- [ ] **Step 3: Run, expect PASS**

Run: `go test ./migrate/ -run TestSentinelsAreDistinct -count=1` and `go build ./migrate/...`. Expect `ok` and no output.

- [ ] **Step 4: Commit**

`git commit -m "feat(migrate): core types, errors and dialect interface"`

## Task 4: Migration file discovery, directives, statement splitter

Covers: S-11 (SQL file directives `-- momo:no-transaction`, `-- momo:destructive`, `-- momo:split`, `-- momo:sqlite-foreign-keys-off`), S-94 (statement-split directive). Acceptance: TestSplitStatements, TestDirectives, TestSplitDollarQuotes.

Files:

- Create: `migrate/files.go`, `migrate/split.go`, `migrate/files_test.go`, `migrate/split_test.go`.

Interfaces (exact):

```go
type File struct {
    Version string // from file name
    Name    string
    Kind    string // "up", "down", "snapshot"
    Path    string // relative to the dialect directory
    Body    []byte
}
type Directives struct {
    NoTransaction       bool // -- momo:no-transaction
    Destructive         bool // -- momo:destructive
    Split               bool // -- momo:split
    SQLiteForeignKeysOff bool // -- momo:sqlite-foreign-keys-off
    Generated           bool // -- momo:generated (written by diff)
}
func ParseDirectives(body []byte) (Directives, error) // only directives in the leading comment block
func Discover(fsys fs.FS) ([]File, error)              // deterministic: sorted by path
func SplitStatements(body []byte, dialectName string) ([]string, error)
```

Splitter rules: statements end at `;` outside single-quoted strings, double-quoted identifiers, `-- ` line comments, `/* */` block comments, and PostgreSQL dollar-quoted bodies (`$tag$ ... $tag$`, tag may be empty). For SQLite, `BEGIN ... END` trigger bodies are kept together: a statement that starts with `CREATE TRIGGER` ends at the `END;` that closes it. Empty statements are dropped. A `-- momo:split` directive makes the splitter split at every `;` outside strings and comments, ignoring dollar quotes and trigger bodies.

Steps:

- [ ] **Step 1: Failing splitter test**

`migrate/split_test.go`:

```go
package migrate

import (
    "reflect"
    "testing"
)

func TestSplitStatements(t *testing.T) {
    body := "CREATE TABLE a (x int); -- trailing ; comment\nINSERT INTO a VALUES ('a;b');\n/* block ; */ SELECT 1;"
    got, err := SplitStatements([]byte(body), "postgres")
    if err != nil {
        t.Fatal(err)
    }
    want := []string{"CREATE TABLE a (x int)", "-- trailing ; comment\nINSERT INTO a VALUES ('a;b')", "/* block ; */ SELECT 1"}
    if !reflect.DeepEqual(got, want) {
        t.Fatalf("got %#v\nwant %#v", got, want)
    }
}

func TestSplitDollarQuotes(t *testing.T) {
    body := "CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql;\nSELECT f();"
    got, err := SplitStatements([]byte(body), "postgres")
    if err != nil {
        t.Fatal(err)
    }
    if len(got) != 2 {
        t.Fatalf("want 2 statements, got %d: %#v", len(got), got)
    }
}

func TestSplitSQLiteTrigger(t *testing.T) {
    body := "CREATE TRIGGER tr AFTER INSERT ON t BEGIN UPDATE t SET x = 1; END;\nSELECT 1;"
    got, err := SplitStatements([]byte(body), "sqlite")
    if err != nil {
        t.Fatal(err)
    }
    if len(got) != 2 {
        t.Fatalf("trigger body must stay one statement, got %#v", got)
    }
}

func TestDirectives(t *testing.T) {
    body := "-- momo:no-transaction\n-- momo:destructive\nDROP TABLE x;"
    d, err := ParseDirectives([]byte(body))
    if err != nil {
        t.Fatal(err)
    }
    if !d.NoTransaction || !d.Destructive || d.Split {
        t.Fatalf("bad directives: %+v", d)
    }
    d2, err := ParseDirectives([]byte("DROP TABLE x;\n-- momo:destructive"))
    if err != nil {
    	t.Fatal(err)
    }
    if d2.Destructive {
        t.Fatal("a directive below the first statement must not count")
    }
}
```

Run: `go test ./migrate/ -run 'TestSplit|TestDirectives' -count=1`. Expect FAIL with `undefined: SplitStatements`.

- [ ] **Step 2: Implement the scanner**

Implement `SplitStatements` as a single pass over bytes with states `normal`, `single`, `double`, `lineComment`, `blockComment`, `dollar(tag)`. Keep the text of each statement exactly as written, trimmed of surrounding whitespace, and keep comments that precede a statement inside it (the expected value in TestSplitStatements keeps `-- trailing ; comment` at the start of the second statement). For SQLite, track whether the current statement began with `CREATE TRIGGER` (case-insensitive) and suspend splitting until a line `END` token followed by `;` is seen at depth zero. Return an error `migrate: unterminated string literal at byte N` when the input ends in a string or block comment.

`ParseDirectives` scans only lines before the first non-comment, non-blank line.

`Discover` walks the fs.FS, accepts names matching `^(\d{14})_([a-z][a-z0-9_]*)\.(up|down)\.sql$` and `^(\d{14})_([a-z][a-z0-9_]*)\.snapshot\.json$`, ignores `momo.sum`, and returns an error naming any other file that looks like a migration (starts with 14 digits and an underscore) but does not match the pattern.

- [ ] **Step 3: Run, expect PASS**

Run: `go test ./migrate/ -run 'TestSplit|TestDirectives' -count=1`. Expect `ok`.

- [ ] **Step 4: Failing discovery test**

Append to `migrate/files_test.go`:

```go
func TestDiscoverRejectsMalformedNames(t *testing.T) {
    fsys := fstest.MapFS{
        "20260101120000_init.up.sql":            {Data: []byte("CREATE TABLE a (x int);")},
        "20260101120000_init.snapshot.json":     {Data: []byte("{}")},
        "20260101120000_bad name.up.sql":        {Data: []byte("")},
    }
    if _, err := Discover(fsys); err == nil || !strings.Contains(err.Error(), "bad name") {
        t.Fatalf("want error naming the malformed file, got %v", err)
    }
}
```

Imports: `testing/fstest`, `strings`. Run: `go test ./migrate/ -run TestDiscoverRejectsMalformedNames -count=1`. Expect FAIL before Step 2's `Discover` is complete.

- [ ] **Step 5: Commit**

`git commit -m "feat(migrate): migration discovery, directives and statement splitter"`

## Task 5: momo.sum checksum file

Covers: S-11 (checksums), S-22 (checksum file header format version). Acceptance: TestChecksumMismatch (offline part), TestSumFormatVersion.

Files:

- Create: `migrate/sum.go`, `migrate/sum_test.go`.

Interfaces (exact):

```go
const SumFormat = 1
type Sum struct {
    DirHash string            // "h1:" + base64(sha256(manifest))
    Files   map[string]string // file name -> hex sha256
}
func ComputeSum(fsys fs.FS) (*Sum, error)      // every file in the directory except momo.sum
func ParseSum(r io.Reader) (*Sum, error)        // refuses SumFormat above the current constant with ErrNewerFormat
func (s *Sum) Write(w io.Writer) error
func VerifySum(fsys fs.FS, want *Sum) []string // names of files whose content differs from want, plus added and removed names
func AppendSum(existing *Sum, fsys fs.FS, added []string) (*Sum, error) // only adds entries for new files; never changes existing lines
```

File format written by `Write`:

```
# momo.sum format=1
h1:<base64 dirhash>
<hex sha256>  <file name>
```

Manifest used for DirHash: for each file in sorted order, the bytes `<name>\n<hex sha256>\n`.

Steps:

- [ ] **Step 1: Failing test**

`migrate/sum_test.go`:

```go
package migrate

import (
    "testing"
	"errors"
	"strings"
	"bytes"
	"testing/fstest"
)

func TestSumFormatVersion(t *testing.T) {
    fsys := fstest.MapFS{"20260101120000_a.up.sql": {Data: []byte("SELECT 1;")}}
    s, err := ComputeSum(fsys)
    if err != nil {
        t.Fatal(err)
    }
    var buf bytes.Buffer
    if err := s.Write(&buf); err != nil {
        t.Fatal(err)
    }
    if !strings.HasPrefix(buf.String(), "# momo.sum format=1\n") {
        t.Fatalf("missing header: %q", buf.String())
    }
    future := strings.Replace(buf.String(), "format=1", "format=9", 1)
    if _, err := ParseSum(strings.NewReader(future)); !errors.Is(err, ErrNewerFormat) {
        t.Fatalf("want ErrNewerFormat, got %v", err)
    }
}

func TestChecksumMismatch(t *testing.T) {
    orig := fstest.MapFS{"20260101120000_a.up.sql": {Data: []byte("SELECT 1;")}}
    s, err := ComputeSum(orig)
    if err != nil {
    	t.Fatal(err)
    }
    edited := fstest.MapFS{"20260101120000_a.up.sql": {Data: []byte("SELECT 2;")}}
    bad := VerifySum(edited, s)
    if len(bad) != 1 || bad[0] != "20260101120000_a.up.sql" {
        t.Fatalf("edited file not reported: %v", bad)
    }
}
```

Add `"errors"` to imports. Run: `go test ./migrate/ -run 'TestSumFormat|TestChecksumMismatch' -count=1`. Expect FAIL with `undefined: ComputeSum`.

- [ ] **Step 2: Implement**

`ComputeSum` reads each file through `fs.ReadFile`, hashes with `crypto/sha256`, sorts names with `slices.Sort`, builds the manifest and `DirHash`. `VerifySum` recomputes per-file hashes and returns sorted names where the hash differs, plus names present in only one side. `ParseSum` reads the header, requires the first line to match `# momo.sum format=(\d+)`, and returns `fmt.Errorf("%w (momo.sum format %d, this momo reads up to %d)", ErrNewerFormat, got, SumFormat)`. `AppendSum` copies `existing`, adds the new names with their hashes, and recomputes DirHash. It never modifies an existing entry. Its error message names any `added` file already present in `existing`.

- [ ] **Step 3: Run, expect PASS** with `go test ./migrate/ -run 'TestSumFormat|TestChecksumMismatch' -count=1`.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate): momo.sum with format header and append-only updates"`

## Task 6: Snapshot wrapper with parent version and format_version

Covers: S-11 (per-migration snapshots recording their parent), S-22 (snapshot format_version). Acceptance: TestSnapshotFormatVersion, TestSnapshotParentRecorded.

Files:

- Create: `migrate/snapshot.go`, `migrate/snapshot_test.go`.

Interfaces (exact):

```go
const SnapshotFormat = 1
type SnapshotFile struct {
    Format        int            `json:"format_version"` // SnapshotFormat
    Version       string         `json:"version"`
    ParentVersion string         `json:"parent"`         // empty for the first migration
    Schema        *schema.Schema `json:"schema"`
}
func ReadSnapshot(r io.Reader) (*SnapshotFile, error) // ErrNewerFormat when Format > SnapshotFormat or schema.FormatVersion > schema.FormatVersion
func WriteSnapshot(w io.Writer, s SnapshotFile) error
func LatestSnapshot(fsys fs.FS) (*SnapshotFile, error) // nil, ErrNoSnapshot when none
```

The snapshot file name is `<version>_<name>.snapshot.json`.

Steps:

- [ ] **Step 1: Failing test**

`migrate/snapshot_test.go`:

```go
package migrate

import (
    "testing"

    "github.com/azrtydxb/momo/schema"
	"errors"
	"strings"
	"bytes"
)

func TestSnapshotParentRecorded(t *testing.T) {
    var buf bytes.Buffer
    in := SnapshotFile{Version: "20260102000000", ParentVersion: "20260101000000", Schema: &schema.Schema{Dialect: "postgres"}}
    if err := WriteSnapshot(&buf, in); err != nil {
        t.Fatal(err)
    }
    out, err := ReadSnapshot(&buf)
    if err != nil {
        t.Fatal(err)
    }
    if out.ParentVersion != "20260101000000" || out.Version != "20260102000000" {
        t.Fatalf("parent or version lost: %+v", out)
    }
}

func TestSnapshotFormatVersion(t *testing.T) {
    src := `{"format_version":9,"version":"1","parent":"","schema":{"dialect":"postgres","tables":[]}}`
    _, err := ReadSnapshot(strings.NewReader(src))
    if !errors.Is(err, schema.ErrNewerFormat) {
        t.Fatalf("want ErrNewerFormat, got %v", err)
    }
}
```

Run: `go test ./migrate/ -run TestSnapshot -count=1`. Expect FAIL with `undefined: SnapshotFile`.

- [ ] **Step 2: Implement** per Interfaces. `ReadSnapshot` decodes with `json.Decoder.DisallowUnknownFields` is NOT used (forward compatibility); it checks `Format` against `SnapshotFormat`, and checks `schema.FormatVersion` in the embedded schema. `LatestSnapshot` picks the greatest version among `.snapshot.json` files and returns an error when two snapshots share a version.

- [ ] **Step 3: Run, expect PASS** with `go test ./migrate/ -run TestSnapshot -count=1`.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate): snapshot files record parent version and format"`

## Task 7: Offline chain, numbering and validate

Covers: S-27 (linear snapshot chain, branch-conflict detection), S-11 (validate offline: checksums, numbering, chain), S-10 (validate command contract, offline part). Acceptance: TestMigrationBranchConflict, TestChecksumMismatch (offline validate), TestValidateOffline.

Files:

- Create: `migrate/chain.go`, `migrate/validate.go`, `migrate/chain_test.go`.

Interfaces (exact):

```go
type ValidateReport struct{ Problems []string }
func ValidateOffline(fsys fs.FS) (*ValidateReport, error)
// Checks, in order, each producing a problem string that names the file:
//  1. every migration file parses (Discover), directives parse
//  2. momo.sum verifies (VerifySum); a missing momo.sum is a problem when migrations exist
//  3. versions are unique and strictly increasing as strings
//  4. every snapshot names a parent that exists, or is empty for the first
//  5. no two snapshots name the same non-empty parent (ErrBranchConflict text)
//  6. every migration has a snapshot file; a custom migration's snapshot equals its parent's (checked by Equal)
func Chain(fsys fs.FS) ([]SnapshotFile, error) // ordered root to head; error wraps ErrBranchConflict
```

Steps:

- [ ] **Step 1: Failing test**

`migrate/chain_test.go`:

```go
package migrate

import (
    "testing"
	"errors"
	"strings"
	"testing/fstest"
)

func snap(version, parent string) string {
    return `{"format_version":1,"version":"` + version + `","parent":"` + parent + `","schema":{"format_version":1,"dialect":"postgres","tables":[]}}`
}

func TestMigrationBranchConflict(t *testing.T) {
    fsys := fstest.MapFS{
        "20260101000000_root.snapshot.json": {Data: []byte(snap("20260101000000", ""))},
        "20260102000000_a.snapshot.json":    {Data: []byte(snap("20260102000000", "20260101000000"))},
        "20260102000001_b.snapshot.json":    {Data: []byte(snap("20260102000001", "20260101000000"))},
    }
    _, err := Chain(fsys)
    if !errors.Is(err, ErrBranchConflict) {
        t.Fatalf("want ErrBranchConflict, got %v", err)
    }
}

func TestValidateOffline(t *testing.T) {
    fsys := fstest.MapFS{
        "20260101000000_root.up.sql":        {Data: []byte("CREATE TABLE a (x int);")},
        "20260101000000_root.snapshot.json": {Data: []byte(snap("20260101000000", ""))},
    }
    rep, err := ValidateOffline(fsys)
    if err != nil {
        t.Fatal(err)
    }
    if len(rep.Problems) != 1 || !strings.Contains(rep.Problems[0], "momo.sum") {
        t.Fatalf("want one problem naming momo.sum, got %v", rep.Problems)
    }
}
```

Add `"strings"` to imports. Run: `go test ./migrate/ -run 'TestMigrationBranchConflict|TestValidateOffline' -count=1`. Expect FAIL with `undefined: Chain`.

- [ ] **Step 2: Implement** `Chain` as: build map parent -> []version; if any parent has more than one child return `fmt.Errorf("%w: %s and %s both have parent %s; run momo migrate rebase", ErrBranchConflict, a, b, parent)` with the two versions sorted. Walk from the empty parent. `ValidateOffline` implements the six checks, collects problems, and returns them; it returns an error only when the directory cannot be read.

- [ ] **Step 3: Run, expect PASS** with `go test ./migrate/ -run 'TestMigrationBranchConflict|TestValidateOffline' -count=1`.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate): offline validation and branch-conflict detection"`

## Task 8: Pure diff engine with explicit renames, destructive gating, staged NOT NULL, FK actions

Covers: S-11 (no rename guessing, explicit rename flags, destructive authorization, staged changes), S-74 (ON DELETE/ON UPDATE in diff, no-constraint option), S-10 (`migrate diff` contract). Acceptance: TestNoRenameGuessing, TestDestructiveRequiresFlag, TestStagedChangeDiff (offline part: diff output), TestFKActions (diff part), TestDiffNoChangeEmpty.

Files:

- Create: `migrate/diff.go`, `migrate/diff_test.go`.

Change kinds (exact constants in `migrate/types.go`, `ChangeKind` type):
`create_schema, create_extension, create_enum, alter_enum_add_value, drop_enum, create_table, drop_table, rename_table, add_column, drop_column, rename_column, alter_column, add_primary_key, drop_primary_key, add_unique, drop_unique, add_check, drop_check, add_fk, drop_fk, create_index, drop_index, set_rls, create_policy, drop_policy, create_view, drop_view, create_function, replace_function, drop_function, create_trigger, drop_trigger`.

Interfaces (exact):

```go
// Diff compares the snapshot schema (from) with the desired schema (to). Pure: no I/O.
func Diff(from, to *schema.Schema, opts DiffOptions) (changes []Change, hints []string, err error)
// Errors: ErrDestructive when a destructive change exists and opts.AllowDestructive is false
//         ErrStagedRequired when NOT NULL without default is added to an existing table and opts.Stage is false
// With opts.Stage true, such a column is emitted as a nullable add_column plus a hint.
```

Rules:

- Table or column present in `from` only: drop (destructive). A drop is never paired with an add into a rename unless the caller passed the explicit rename flag; the hint `"possible rename: drop+add of t.a -> t.b; pass --rename-column t.a=b to rename"` is added when a dropped column and an added column share a type on the same table.
- Rename flags turn the pair into `rename_table` or `rename_column` and produce no drop/add.
- Column type change: `alter_column`, destructive.
- NULL to NOT NULL on an existing table without default: ErrStagedRequired (or staged emit).
- NOT NULL to NULL, default change, identity change: `alter_column`, not destructive.
- Foreign keys: `add_fk` and `drop_fk` compare `OnDelete` and `OnUpdate` as well as columns and referenced table. An action change is a drop_fk followed by add_fk.
- Views, functions, triggers, policies: changes compare normalized `Query`, `Body`, `Definition` (NormalizeExpr) and structured fields; a changed view is `drop_view` + `create_view`, and every view that depends on it is also dropped and re-created (handled in order.go).
- Destructive kinds: drop_table, drop_column, alter_column with a type change, drop_enum, drop_fk? (no: dropping an FK is not destructive), drop_policy? (no), drop_function (no), replace_function (no). Table-level RLS disable is not destructive.

Steps:

- [ ] **Step 1: Failing tests**

`migrate/diff_test.go`:

```go
package migrate

import (
    "testing"

    "github.com/azrtydxb/momo/schema"
	"errors"
)

func base() *schema.Schema {
    return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
        Name: "users",
        Columns: []schema.Column{{Name: "id", Type: "bigint"}, {Name: "email", Type: "text"}},
        PrimaryKey: []string{"id"},
    }}}
}

func TestNoRenameGuessing(t *testing.T) {
    from := base()
    to := base()
    to.Tables[0].Columns[1] = schema.Column{Name: "mail", Type: "text"}
    changes, hints, err := Diff(from, to, DiffOptions{AllowDestructive: true})
    if err != nil {
        t.Fatal(err)
    }
    for _, c := range changes {
        if c.Kind == "rename_column" {
            t.Fatalf("diff guessed a rename: %+v", c)
        }
    }
    if len(hints) == 0 {
        t.Fatal("expected a rename hint for drop+add of the same type")
    }
}

func TestDestructiveRequiresFlag(t *testing.T) {
    from := base()
    to := base()
    to.Tables[0].Columns = to.Tables[0].Columns[:1]
    _, _, err := Diff(from, to, DiffOptions{})
    if !errors.Is(err, ErrDestructive) {
        t.Fatalf("want ErrDestructive, got %v", err)
    }
    if _, _, err := Diff(from, to, DiffOptions{AllowDestructive: true}); err != nil {
        t.Fatalf("with the flag the diff must succeed: %v", err)
    }
}

func TestStagedChangeDiff(t *testing.T) {
    from := base()
    to := base()
    to.Tables[0].Columns = append(to.Tables[0].Columns, schema.Column{Name: "tenant", Type: "bigint"})
    _, _, err := Diff(from, to, DiffOptions{})
    if !errors.Is(err, ErrStagedRequired) {
        t.Fatalf("NOT NULL without default must need staging, got %v", err)
    }
    changes, hints, err := Diff(from, to, DiffOptions{Stage: true})
    if err != nil {
        t.Fatal(err)
    }
    if len(changes) != 1 || changes[0].Kind != "add_column" || changes[0].After.(*schema.Column).Nullable != true {
        t.Fatalf("staged add must be nullable: %+v", changes)
    }
    if len(hints) == 0 {
        t.Fatal("staged add must hint the backfill and SET NOT NULL steps")
    }
}

func TestFKActions(t *testing.T) {
    from := base()
    to := base()
    to.Tables[0].ForeignKeys = []schema.ForeignKey{{Name: "fk", Columns: []string{"id"}, RefTable: "orgs", RefColumns: []string{"id"}, OnDelete: "CASCADE"}}
    changes, _, err := Diff(from, to, DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if len(changes) != 1 || changes[0].Kind != "add_fk" {
        t.Fatalf("want one add_fk, got %+v", changes)
    }
}

func TestDiffNoChangeEmpty(t *testing.T) {
    changes, hints, err := Diff(base(), base(), DiffOptions{})
    if err != nil || len(changes) != 0 || len(hints) != 0 {
        t.Fatalf("identical schemas must produce no changes: %v %v %v", changes, hints, err)
    }
}
```

Run: `go test ./migrate/ -run 'TestNoRename|TestDestructive|TestStaged|TestFKActions|TestDiffNoChange' -count=1`. Expect FAIL with `undefined: Diff`.

- [ ] **Step 2: Implement `Diff`** in `migrate/diff.go` following the rules above. Compare tables by name, columns by name (order changes are not diffed: column order is preserved by SQLite rebuilds only when requested), indexes by name and structure, unique and check constraints by name, FKs by name and structure. `ChangeKind` constants go in `types.go`. The `After` field of an add_column change holds the `*schema.Column`. Each change's `Destructive` is set per the rules. Sort the output by kind order defined in `order.go` (Task 9), and the hints in sorted order.

- [ ] **Step 3: Run, expect PASS** with the same command. Expect `ok`.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate): pure diff with explicit renames, destructive and staged gating"`

## Task 9: Dependency-aware ordering of managed objects

Covers: S-36 (schema-qualified creation order), S-37 (views dropped after dependents, recreated in dependency order), S-39 (functions before triggers, triggers before dropped functions). Acceptance: TestViewsAsModels (ordering part), TestManagedFunctionsTriggersPartitions (ordering part).

Files:

- Create: `migrate/order.go`, `migrate/order_test.go`.

Interfaces (exact):

```go
// Order sorts changes into an executable sequence.
// Creates: create_schema, create_extension, create_enum, create_table (parents before partitions, referenced tables before referencing via FK, FK-free order otherwise), add_column, alter_column, create_index, create_function, create_view (dependencies first), create_trigger, create_policy, set_rls.
// Drops: drop_trigger, drop_policy, drop_view (dependents first), drop_fk, drop_index, drop_table, drop_function, drop_enum.
// Renames run before everything else touching the renamed object.
func Order(changes []Change) []Change
```

Steps:

- [ ] **Step 1: Failing test**

`migrate/order_test.go`:

```go
package migrate

import "testing"

func TestDropViewBeforeDependentView(t *testing.T) {
    changes := []Change{
        {Kind: "drop_view", Name: "base_v", After: nil, Before: viewDep{Depends: nil}},
        {Kind: "drop_view", Name: "top_v", Before: viewDep{Depends: []string{"base_v"}}},
    }
    got := Order(changes)
    if got[0].Name != "top_v" || got[1].Name != "base_v" {
        t.Fatalf("dependent view must drop first, got %s then %s", got[0].Name, got[1].Name)
    }
}

func TestCreateViewAfterDependency(t *testing.T) {
    changes := []Change{
        {Kind: "create_view", Name: "top_v", After: viewDep{Depends: []string{"base_v"}}},
        {Kind: "create_view", Name: "base_v", After: viewDep{}},
    }
    got := Order(changes)
    if got[0].Name != "base_v" {
        t.Fatalf("dependency must be created first, got %s", got[0].Name)
    }
}

func TestCycleIsAnError(t *testing.T) {
    changes := []Change{
        {Kind: "create_view", Name: "a", After: viewDep{Depends: []string{"b"}}},
        {Kind: "create_view", Name: "b", After: viewDep{Depends: []string{"a"}}},
    }
    if _, err := OrderErr(changes); err == nil {
        t.Fatal("cycle must be reported, not silently ordered")
    }
}
```

`viewDep` is a test type `type viewDep struct{ Depends []string }` defined in the test file; the production code reads dependencies through an interface `interface{ dependsOn() []string }` that `viewDep` implements in the test file and that `*schema.View` implements in `order.go` via `func (v *schema.View) ...`. Because the schema package is outside migrate, implement the adapter `func depsOf(c Change) []string` with a type switch on `*schema.View`, `schema.View`, and `dependent` (an interface with method `dependsOn() []string`). Add `OrderErr(changes []Change) ([]Change, error)`; `Order` panics never; it calls OrderErr and, on error, returns changes unchanged and the caller (diff) turns the error into a diagnostic. Run: `go test ./migrate/ -run 'TestDrop|TestCreate|TestCycle' -count=1`. Expect FAIL with `undefined: Order`.

- [ ] **Step 2: Implement** a stable topological sort (Kahn's algorithm over names, ties broken by original index) per phase, phases in the order listed in Interfaces. Dependencies are matched by qualified name. Cycle returns `fmt.Errorf("migrate: dependency cycle among %s", names)`.

- [ ] **Step 3: Run, expect PASS** with the same command.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate): dependency-aware ordering of managed objects"`

## Task 10: PostgreSQL inspection: schemas, extensions, enums, tables, columns, constraints

Covers: S-12 (extensions, enums, constraints), S-36 (schemas), S-74 (FK actions round trip). Acceptance: TestPostgresObjects (partial), TestFKActions (PostgreSQL), TestPostgresSchemas (inspection part).

Files:

- Create: `migrate/postgres/dialect.go`, `migrate/postgres/inspect.go`, `migrate/postgres/inspect_test.go`.

Interfaces (exact):

```go
package postgres
type Dialect struct{}
func New() Dialect
func (Dialect) Inspect(ctx context.Context, c momo.PinnedConn, opts migrate.InspectOptions) (*schema.Schema, error)
```

Catalog queries (exact):

- Namespaces: `SELECT nspname FROM pg_namespace WHERE nspname = ANY($1) ORDER BY 1` with $1 = configured schemas (default `{current_schema()}`: when `InspectOptions.Schemas` is empty, Inspect first runs `SELECT current_schema()`). A table whose namespace equals the current schema is returned with `Table.Schema == ""`; a table in any other managed namespace carries its name. `Schemas` in the result lists only the non-current managed namespaces.
- Extensions: `SELECT extname, extversion FROM pg_extension WHERE extname <> 'plpgsql' ORDER BY 1`.
- Enums: `SELECT n.nspname, t.typname, e.enumlabel FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace JOIN pg_enum e ON e.enumtypid = t.oid WHERE t.typtype = 'e' AND n.nspname = ANY($1) ORDER BY 1,2,e.enumsortorder`.
- Tables: `SELECT n.nspname, c.relname, c.relkind, c.relrowsecurity, c.relforcerowsecurity FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('r','p') AND c.relispartition = false AND n.nspname = ANY($1) ORDER BY 1,2`. Partition children (relispartition true) are fetched in the partition query and set `PartitionOf`.
- Columns: `SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull, pg_get_expr(d.adbin, d.adrelid), a.attidentity, a.attgenerated, t.typtype, t.typname FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum JOIN pg_type t ON t.oid = a.atttypid WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`.
- Serial rule: when the default is `nextval('<seq>'::regclass)` and the sequence is owned by the column (`SELECT 1 FROM pg_depend WHERE classid='pg_class'::regclass AND objid=<seq oid> AND refobjsubid=<attnum> AND deptype='a'`), inspection reports `Identity: "by_default"` and an empty Default. Declared identity columns therefore round-trip.
- Primary key and unique and check and FK: `SELECT conname, contype, conkey, confrelid::regclass::text, confkey, confupdtype, confdeltype, pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = $1 ORDER BY conname`. Column names are resolved through `pg_attribute` by attnum. For `contype = 'f'`, map `confupdtype` and `confdeltype` with `a` NO ACTION (empty string), `r` RESTRICT, `c` CASCADE, `n` SET NULL, `d` SET DEFAULT. For `contype = 'c'`, Expr is the text inside `CHECK (...)` from pg_get_constraintdef.
- Partitioned parent: `SELECT pt.partstrat, pg_get_partkeydef(c.oid) FROM pg_partitioned_table pt JOIN pg_class c ON c.oid = pt.partrelid WHERE c.oid = $1`. Strategy map: `r` range, `l` list, `h` hash.
- Partition child: `SELECT pg_get_expr(c.relpartbound, c.oid), p.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid JOIN pg_class p ON p.oid = i.inhparent WHERE c.oid = $1`.

Steps:

- [ ] **Step 1: Failing test (requires PostgreSQL)**

`migrate/postgres/inspect_test.go`:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestPostgresInspectColumnsAndConstraints(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    mustExec(t, db.Driver(), `CREATE EXTENSION IF NOT EXISTS vector`)
    mustExec(t, db.Driver(), `CREATE TYPE status AS ENUM ('draft','live')`)
    mustExec(t, db.Driver(), `CREATE TABLE orgs (id bigint PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY)`)
    mustExec(t, db.Driver(), `CREATE TABLE docs (
        id bigint PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
        org_id bigint NOT NULL REFERENCES orgs(id) ON DELETE CASCADE ON UPDATE SET NULL,
        state status NOT NULL DEFAULT 'draft',
        emb vector(3),
        CONSTRAINT docs_state_check CHECK (id > 0)
    )`)
    c := pinned(t, db)
    got, err := mpg.New().Inspect(ctx, c, migrate.InspectOptions{Schemas: []string{testdb.Schema(t)}}) // explicit: the per-test schema; an empty list would resolve to current_schema() as well
    if err != nil {
        t.Fatal(err)
    }
    docs, ok := got.Table("docs")
    if !ok {
        t.Fatal("docs missing")
    }
    if len(docs.ForeignKeys) != 1 {
    	t.Fatalf("want one foreign key, got %+v", docs.ForeignKeys)
    }
    if docs.ForeignKeys[0].OnDelete != "CASCADE" || docs.ForeignKeys[0].OnUpdate != "SET NULL" {
        t.Fatalf("fk actions: %+v", docs.ForeignKeys[0])
    }
    col, ok := docs.Column("state")
    if !ok {
    	t.Fatalf("column %q missing", "state")
    }
    if col.Enum != "status" || col.Default != "'draft'::status" {
        t.Fatalf("enum column: %+v", col)
    }
    col, ok := docs.Column("id")
    if !ok {
    	t.Fatalf("column %q missing", "id")
    }
    if col.Identity != "always" {
        t.Fatalf("identity: %+v", col)
    }
    col, ok := docs.Column("emb")
    if !ok {
    	t.Fatalf("column %q missing", "emb")
    }
    if col.Type != "vector(3)" {
        t.Fatalf("vector type: %+v", col)
    }
    if len(got.Enums) != 1 || len(got.Extensions) != 1 {
        t.Fatalf("enums/extensions: %+v %+v", got.Enums, got.Extensions)
    }
}
```

`testopen.Postgres(t)` requires `MOMO_TEST_POSTGRES_URL` (failing with `MOMO_REQUIRE_INTEGRATION=1` when unset). Run: `MOMO_TEST_POSTGRES_URL=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable go test ./migrate/postgres/ -run TestPostgresInspect -count=1`. Expect FAIL with `undefined: mpg.New`.

Note: `col.Default` for an enum default is stored as the dialect text `'draft'::status` (not normalized here; normalization happens in schema.Equal). The assertion above is therefore against the raw inspected text.

- [ ] **Step 2: Implement** the queries in `inspect.go`. the dialect is passed explicitly with `migrate.WithDialect(mpg.New())`; there is no registry.

- [ ] **Step 3: Run, expect PASS** with the same command.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate/postgres): inspection of schemas, enums, extensions, columns and constraints"`

## Task 11: PostgreSQL inspection: indexes (expression, partial, method, opclass, invalid)

Covers: S-12 (expression and partial indexes, index methods and operator classes). Acceptance: TestPostgresObjects (index part), TestPostgresIndexInspection.

Files:

- Create: `migrate/postgres/inspect_index.go`, `migrate/postgres/inspect_index_test.go`.

Interfaces (exact):

```go
func (Dialect) inspectIndexes(ctx context.Context, c momo.PinnedConn, relOID uint32) ([]schema.Index, []string /* invalid index names */, error)
```

Query (exact):

```sql
SELECT ci.relname, am.amname, i.indisunique, i.indisvalid, i.indnkeyatts,
       pg_get_expr(i.indpred, i.indrelid), i.indoption::text, i.indclass::text, ci.reloptions
FROM pg_index i
JOIN pg_class ci ON ci.oid = i.indexrelid
JOIN pg_am am ON am.oid = ci.relam
WHERE i.indrelid = $1 AND NOT i.indisprimary
  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = i.indexrelid AND c.contype IN ('u','p','x'))
ORDER BY ci.relname
```

Per key column k (1..indnkeyatts): `pg_get_indexdef(indexrelid, k, true)` gives the column name or expression text; `indoption` array element bit 0x1 = DESC, bit 0x2 = NULLS FIRST (default for DESC is NULLS FIRST, so `Nulls` is set only when it differs from the default: DESC with NULLS LAST, or ASC with NULLS FIRST); opclass is the `indclass` element joined to `pg_opclass` and recorded only when `opcdefault = false`. `reloptions` split into KV pairs (`m=16` etc.). Invalid indexes (`indisvalid = false`) are returned as names so drift reports them as Changed with Detail "invalid index (failed CONCURRENTLY build)".

Steps:

- [ ] **Step 1: Failing test**

`migrate/postgres/inspect_index_test.go`:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestPostgresIndexInspection(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    mustExec(t, db.Driver(), `CREATE EXTENSION IF NOT EXISTS vector`)
    mustExec(t, db.Driver(), `CREATE TABLE d (id bigint PRIMARY KEY, title text, emb vector(3), deleted_at timestamptz)`)
    mustExec(t, db.Driver(), `CREATE INDEX d_lower_title ON d (lower(title))`)
    mustExec(t, db.Driver(), `CREATE INDEX d_live ON d (id) WHERE deleted_at IS NULL`)
    mustExec(t, db.Driver(), `CREATE INDEX d_emb ON d USING hnsw (emb vector_cosine_ops) WITH (m = 16, ef_construction = 64)`)
    got, err := mpg.New().Inspect(ctx, pinned(t, db), migrate.InspectOptions{})
    if err != nil {
        t.Fatal(err)
    }
    tbl, ok := got.Table("d")
    if !ok {
    	t.Fatalf("table %q missing", "d")
    }
    byName := map[string]schema.Index{}
    for _, ix := range tbl.Indexes {
        byName[ix.Name] = ix
    }
    must := func(name string) schema.Index {
        ix, ok := byName[name]
        if !ok {
            t.Fatalf("index %q missing", name)
        }
        return ix
    }
    if must("d_lower_title").Parts[0].Expr != "lower(title)" {
        t.Fatalf("expression index: %+v", must("d_lower_title"))
    }
    if must("d_live").Where != "(deleted_at IS NULL)" {
        t.Fatalf("partial index predicate: %q", must("d_live").Where)
    }
    emb := must("d_emb")
    if emb.Method != "hnsw" || emb.Parts[0].OpClass != "vector_cosine_ops" || len(emb.With) != 2 {
        t.Fatalf("hnsw index: %+v", emb)
    }
}
```

Add `"github.com/azrtydxb/momo/schema"` to imports. Run: `MOMO_TEST_POSTGRES_URL=... go test ./migrate/postgres/ -run TestPostgresIndexInspection -count=1`. Expect FAIL with `undefined: ... inspectIndexes` or wrong values.

- [ ] **Step 2: Implement** `inspectIndexes` and call it from Inspect for every table.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(migrate/postgres): index inspection with expressions, predicates, methods and opclasses"`.

## Task 12: PostgreSQL inspection: views, functions, triggers, policies, RLS, partitions

Covers: S-37 (views and materialized views), S-38 (policies, RLS flags), S-39 (functions, triggers, partitions). Acceptance: TestManagedFunctionsTriggersPartitions (inspection part), TestViewsAsModels (inspection part), TestRowLevelSecurity (inspection part).

Files:

- Create: `migrate/postgres/inspect_objects.go`, `migrate/postgres/inspect_objects_test.go`.

Interfaces (exact): `func (Dialect) inspectObjects(ctx context.Context, c momo.PinnedConn, s *schema.Schema, schemas []string) error` fills `Views`, `Functions`, `Triggers`, `Policies` and sets `RLS` and `Partition`/`PartitionOf` on tables.

Queries (exact):

- Views and materialized views: `SELECT n.nspname, c.relname, c.relkind, pg_get_viewdef(c.oid, true) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('v','m') AND n.nspname = ANY($1)`; deps with `SELECT DISTINCT CASE WHEN rn.nspname = current_schema() THEN rc.relname ELSE rn.nspname || '.' || rc.relname END FROM pg_depend d JOIN pg_rewrite r ON r.oid = d.objid JOIN pg_class v ON v.oid = r.ev_class JOIN pg_class rc ON rc.oid = d.refobjid JOIN pg_namespace rn ON rn.oid = rc.relnamespace WHERE v.oid = $1 AND rc.oid <> v.oid AND d.refclassid = 'pg_class'::regclass`. Materialized view indexes come from inspectIndexes on the matview's OID.
- Functions: `SELECT n.nspname, p.proname, pg_get_function_arguments(p.oid), pg_get_function_result(p.oid), l.lanname, p.provolatile, p.prosrc FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace JOIN pg_language l ON l.oid = p.prolang WHERE p.prokind = 'f' AND n.nspname = ANY($1)`. Volatility map: `i` IMMUTABLE, `s` STABLE, `v` VOLATILE.
- Triggers: the function is read as `p.oid::regprocedure::text`, which yields `name(argtypes)` and prefixes the schema only when the function is not visible on the search_path (so `touch()` for a function in the current schema, `other.touch()` otherwise); `Trigger.Function` stores exactly that text and the renderer emits `EXECUTE FUNCTION <Function>` followed by the argument list only for non-empty `Trigger.Args` (an empty argument list is already the `()` in the stored text). `SELECT n.nspname, c.relname, t.tgname, t.tgtype, pg_get_expr(t.tgqual, t.tgrelid), p.oid::regprocedure::text FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid JOIN pg_namespace n ON n.oid = c.relnamespace JOIN pg_proc p ON p.oid = t.tgfoid WHERE NOT t.tgisinternal AND n.nspname = ANY($1)`. Bits: ROW 1, BEFORE 2, INSERT 4, DELETE 8, UPDATE 16, TRUNCATE 32, INSTEAD 64. A trigger with an UPDATE OF column list returns ErrUnsupported naming the trigger.
- Policies: `SELECT n.nspname, c.relname, p.polname, p.polcmd, p.polpermissive, ARRAY(SELECT rolname FROM pg_roles WHERE oid = ANY(p.polroles) ORDER BY rolname), pg_get_expr(p.polqual, p.polrelid), pg_get_expr(p.polwithcheck, p.polrelid) FROM pg_policy p JOIN pg_class c ON c.oid = p.polrelid JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = ANY($1)`. Command map: `r` SELECT, `a` INSERT, `w` UPDATE, `d` DELETE, `*` ALL. Roles containing 0 (PUBLIC) are returned as an empty list.
- RLS flags: from the table query in Task 10 (relrowsecurity, relforcerowsecurity) stored as `RLS` when either is true.
- Partition info: Task 10 queries. Partition children are returned as tables carrying `PartitionOf` (Task 10 does not filter `relispartition`); constraints a child merely inherits (`conparentid <> 0`) are hidden.

Steps:

- [ ] **Step 1: Failing test**

`migrate/postgres/inspect_objects_test.go`:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestPostgresObjectInspection(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    d := db.Driver()
    mustExec(t, d, `CREATE TABLE events (id bigint, created_at timestamptz NOT NULL, tenant text NOT NULL, PRIMARY KEY (id, created_at)) PARTITION BY RANGE (created_at)`)
    mustExec(t, d, `CREATE TABLE events_2026 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')`)
    mustExec(t, d, `CREATE FUNCTION touch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
    mustExec(t, d, `CREATE TRIGGER events_touch BEFORE INSERT OR UPDATE ON events FOR EACH ROW EXECUTE FUNCTION touch()`)
    mustExec(t, d, `ALTER TABLE events ENABLE ROW LEVEL SECURITY`)
    mustExec(t, d, `CREATE POLICY tenant_only ON events USING (tenant = current_setting('app.tenant', true))`)
    mustExec(t, d, `CREATE VIEW recent AS SELECT id FROM events`)
    got, err := mpg.New().Inspect(ctx, pinned(t, db), migrate.InspectOptions{})
    if err != nil {
        t.Fatal(err)
    }
    ev, ok := got.Table("events")
    if !ok {
    	t.Fatalf("table %q missing", "events")
    }
    if ev.Partition == nil || ev.Partition.Strategy != "range" {
        t.Fatalf("partition not inspected: %+v", ev.Partition)
    }
    if ev.RLS == nil || !ev.RLS.Enabled {
        t.Fatal("RLS flag not inspected")
    }
    child, ok := got.Table("events_2026")
    if !ok || child.PartitionOf == nil || child.PartitionOf.Parent != "events" {
        t.Fatalf("partition child must appear as a table carrying PartitionOf: %+v", child)
    }
    if len(child.PrimaryKey) != 0 {
        t.Fatalf("constraints inherited from the parent must be hidden on the child: %+v", child.PrimaryKey)
    }
    if len(got.Triggers) != 1 || got.Triggers[0].Timing != "BEFORE" || len(got.Triggers[0].Events) != 2 {
        t.Fatalf("trigger: %+v", got.Triggers)
    }
    if len(got.Functions) != 1 || got.Functions[0].Language != "plpgsql" {
        t.Fatalf("function: %+v", got.Functions)
    }
    if len(got.Policies) != 1 || got.Policies[0].Command != "ALL" {
        t.Fatalf("policy: %+v", got.Policies)
    }
    if len(got.Views) != 1 || got.Views[0].Name != "recent" {
        t.Fatalf("view: %+v", got.Views)
    }
}
```

Run with the integration URL. Expect FAIL before implementation with `undefined` or assertion failures.

- [ ] **Step 2: Implement** `inspectObjects` and wire it into Inspect after tables and indexes exist.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(migrate/postgres): inspect views, functions, triggers, policies, RLS and partitions"`.

## Task 13: PostgreSQL render for all change kinds

Covers: S-12 (extensions, enums, constraints, expression and partial indexes, methods and opclasses, CONCURRENTLY outside transactions), S-37, S-38, S-39 (CREATE OR REPLACE FUNCTION, CREATE TRIGGER, partitions). Acceptance: TestPostgresObjects (full round trip with no diff), TestManagedFunctionsTriggersPartitions (CREATE OR REPLACE on body change), TestViewsAsModels (create, replace via drop and recreate in order), TestPostgresConcurrentIndexNoTx.

Files:

- Create: `migrate/postgres/render.go`, `migrate/postgres/render_test.go`, `migrate/postgres/roundtrip_test.go`, `migrate/postgres/managed_objects_test.go`.

Interfaces (exact):

```go
func (Dialect) Render(plan []migrate.Change) ([]migrate.Statement, error)
// Every identifier via dialect.For(dialect.Postgres).QuoteIdent; every table reference is "schema"."table" when Table.Schema is set and just "table" when it is empty (resolved by the connection's search_path, whose first entry is the current schema).
// create_index with Index.Concurrently=true renders CREATE INDEX CONCURRENTLY and sets Statement.NoTx = true.
// drop_index with Concurrently=true renders DROP INDEX CONCURRENTLY and sets NoTx.
// Enum value add renders ALTER TYPE ... ADD VALUE [BEFORE|AFTER] and sets Hint "enum-add" so the lint rule R7 can see it.
// Function bodies render with dollar tag $momo$ and fail with an error if the body contains $momo$.
// replace_function (same name and arguments, changed body or volatility) renders CREATE OR REPLACE FUNCTION; a changed argument list or return type renders DROP FUNCTION then CREATE FUNCTION.
// Trigger renders CREATE TRIGGER name timing events ON table FOR EACH ROW|STATEMENT WHEN(...) EXECUTE FUNCTION <Trigger.Function> where Function is the regprocedure text read by Task 12 (for example "touch()"; Args, when non-empty, replace the empty parentheses with the quoted argument list).
// Policy renders CREATE POLICY name ON table AS PERMISSIVE|RESTRICTIVE FOR command TO roles USING (...) WITH CHECK (...).
// set_rls renders ALTER TABLE ... ENABLE|DISABLE ROW LEVEL SECURITY and FORCE|NO FORCE ROW LEVEL SECURITY.
// Partitioned create_table renders PARTITION BY <Partition.Key>; partition children render CREATE TABLE ... PARTITION OF parent <Bound>.
// Materialized view: CREATE MATERIALIZED VIEW; views: CREATE VIEW.
// Unsupported on PostgreSQL in this plan: drop_enum (returns error naming the enum; enum value removal requires a manual migration)
```

Steps:

- [ ] **Step 1: Failing round-trip test**

`migrate/postgres/roundtrip_test.go`:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"strings"
)

func TestPostgresObjects(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    desired := &schema.Schema{
        Dialect: "postgres",
        Extensions: []schema.Extension{{Name: "vector"}},
        Enums:      []schema.Enum{{Name: "status", Values: []string{"draft", "live"}}},
        Tables: []schema.Table{{
            Name: "docs",
            Columns: []schema.Column{
                {Name: "id", Type: "bigint", Identity: "always"},
                {Name: "state", Type: "status", Enum: "status", Default: "'draft'"},
                {Name: "emb", Type: "vector(3)", Nullable: true},
                {Name: "deleted_at", Type: "timestamp with time zone", Nullable: true},
            },
            PrimaryKey: []string{"id"},
            Indexes: []schema.Index{
                {Name: "docs_lower", Parts: []schema.IndexPart{{Expr: "lower(state::text)"}}},
                {Name: "docs_live", Parts: []schema.IndexPart{{Column: "id"}}, Where: "deleted_at IS NULL"},
                {Name: "docs_emb", Method: "hnsw", Parts: []schema.IndexPart{{Column: "emb", OpClass: "vector_cosine_ops"}}, With: []schema.KV{{Key: "m", Value: "16"}}},
                {Name: "docs_state_c", Parts: []schema.IndexPart{{Column: "state"}}, Concurrently: true},
            },
        }},
    }
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
    mig, err := e.Diff(ctx, desired, "objects", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
    if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    got, err := e2.Inspect(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if !schema.Equal(got, desired) {
        t.Fatalf("round trip has a diff: %s", migratetest.Diff(got, desired))
    }
}

func TestPostgresConcurrentIndexNoTx(t *testing.T) {
    stmts, err := mpg.Dialect{}.Render([]migrate.Change{{Kind: "create_index", Table: "docs", Name: "i", After: &schema.Index{Name: "i", Concurrently: true, Parts: []schema.IndexPart{{Column: "x"}}}}})
    if err != nil {
        t.Fatal(err)
    }
    if !stmts[0].NoTx || !strings.HasPrefix(stmts[0].SQL, "CREATE INDEX CONCURRENTLY") {
        t.Fatalf("concurrent index must render outside a transaction: %+v", stmts[0])
    }
}
```

Add `"strings"` and schema imports to the render test. `migratetest.Diff(a, b) string` prints the JSON difference. Run: `MOMO_TEST_POSTGRES_URL=... go test ./migrate/postgres/ -run 'TestPostgresObjects|TestPostgresConcurrentIndexNoTx' -count=1`. Expect FAIL with `undefined: mpg.Dialect` or `Render` returning nothing.

- [ ] **Step 1b: Failing S-39 test (functions, triggers, partitions)**

`migrate/postgres/managed_objects_test.go` (`//go:build integration`, package `postgres_test`). It covers the acceptance for S-39: the three object kinds are created by a migration, round-trip with no diff, are not reported as unmanaged, and a changed function body renders `CREATE OR REPLACE FUNCTION`:

```go
//go:build integration

package postgres_test

import (
    "context"
    "strings"
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/migrate/migratetest"
    mpg "github.com/azrtydxb/momo/migrate/postgres"
    "github.com/azrtydxb/momo/schema"
    testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

// managedObjects returns a range-partitioned table with one child partition, a plpgsql
// trigger function and a row trigger on the parent that calls it.
func managedObjects(body string) *schema.Schema {
    cols := []schema.Column{
        {Name: "id", Type: "bigint"},
        {Name: "tenant", Type: "text"},
        {Name: "created_at", Type: "timestamp with time zone"},
    }
    return &schema.Schema{
        Dialect: "postgres",
        Tables: []schema.Table{
            {
                Name:       "events",
                Columns:    cols,
                PrimaryKey: []string{"id", "created_at"},
                Partition:  &schema.Partition{Strategy: "range", Key: "RANGE (created_at)"},
            },
            {
                Name:        "events_2026",
                Columns:     cols,
                PartitionOf: &schema.PartitionOf{Parent: "events", Bound: "FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')"},
            },
        },
        Functions: []schema.Function{{
            Name: "touch", Args: "", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: body,
        }},
        Triggers: []schema.Trigger{{
            Table: "events", Name: "events_touch", Timing: "BEFORE", Events: []string{"INSERT", "UPDATE"},
            ForEachRow: true, Function: "touch()", // exactly what Task 12 reads back: p.oid::regprocedure::text, unqualified because touch is on the test schema's search_path
        }},
    }
}

func TestManagedFunctionsTriggersPartitions(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    desired := managedObjects("BEGIN RETURN NEW; END;")

    first, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS()).Diff(ctx, desired, "managed", migrate.DiffOptions{})
    if err != nil || first == nil {
        t.Fatalf("diff: %v %v", first, err)
    }
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(first.Files))
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }

    // Round trip: the live database inspects back to the desired schema.
    got, err := e.Inspect(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if !schema.Equal(got, desired) {
        t.Fatalf("round trip has a diff: %s", migratetest.Diff(got, desired))
    }

    // None of the three kinds is reported as drift or as unmanaged.
    rep, err := e.Drift(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if !rep.Clean() {
        t.Fatalf("managed objects reported as drift: %+v", rep)
    }
    for _, u := range rep.Unmanaged {
        if u.Kind == "function" || u.Kind == "trigger" || u.Kind == "table" {
            t.Fatalf("%s %s.%s reported as unmanaged", u.Kind, u.Schema, u.Name)
        }
    }

    // No change: Diff returns nil (nothing written); a non-nil result must hold no changes.
    again, err := e.Diff(ctx, desired, "noop", migrate.DiffOptions{})
    if err != nil || (again != nil && len(again.Changes) != 0) {
        t.Fatalf("no-change diff must write nothing: %v %v", again, err)
    }

    // Body change: the function is replaced, not dropped and recreated.
    changed := managedObjects("BEGIN NEW.tenant := upper(NEW.tenant); RETURN NEW; END;")
    mod, err := e.Diff(ctx, changed, "body", migrate.DiffOptions{})
    if err != nil || mod == nil {
        t.Fatalf("body change must produce a migration: %v %v", mod, err)
    }
    var up strings.Builder
    for _, st := range mod.Up {
        up.WriteString(st.SQL)
        up.WriteString(";\n")
    }
    if !strings.Contains(up.String(), "CREATE OR REPLACE FUNCTION") {
        t.Fatalf("body change must render CREATE OR REPLACE FUNCTION, got:\n%s", up.String())
    }
}
```

Run: `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration ./migrate/postgres/ -run TestManagedFunctionsTriggersPartitions -count=1`. Expect FAIL at the first assertion the not-yet-written render, inspection or diff code does not satisfy (`diff: ...`, `round trip has a diff: ...`, or `body change must render CREATE OR REPLACE FUNCTION`).

- [ ] **Step 2: Implement `Render`** with one renderer function per change kind, each returning exactly one Statement except `create_view` with dependents, which renders in the order given by Order. Use `strings.Builder` and `dialect.AppendIdent` only (no `fmt.Sprintf` of identifiers).

- [ ] **Step 3: Run, expect PASS** for both tests.

- [ ] **Step 3b: Run the S-39 test, expect PASS.** `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration ./migrate/postgres/ -run TestManagedFunctionsTriggersPartitions -count=1` → `ok`.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate/postgres): render all managed object kinds with CONCURRENTLY marked no-transaction"`.

## Task 14: PostgreSQL migration lock and session timeouts

Covers: S-11 (concurrency lock, session-scoped), S-28 (lock_timeout and statement_timeout applied during apply). Acceptance: TestConcurrentApply (PostgreSQL), TestLockTimeoutApplied, TestLockHolderNamed, TestLockKeyPerSchema. Tests that assert lock contention use two connections or engines on the SAME schema (one `testopen.Postgres(t)` handle).

Also implements `HistoryDDL() []string` (the `momo_migrations` DDL, including `apply_id` and `kind`), which the engine core in Task 15 needs.

Files:

- Create: `migrate/postgres/lock.go`, `migrate/postgres/session.go`, `migrate/postgres/lock_test.go`.

Interfaces (exact):

```go
// Lock takes a session-level advisory lock on the pinned connection:
//   SELECT pg_try_advisory_lock($1)  -- $1 = int64 from fnv64a(key)
// The engine passes key = "momo:" + <history schema> + "." + <history table> (for example "momo:t_ab12.momo_migrations"),
// because PostgreSQL advisory locks are database-wide and two schemas in one database must not block each other.
// The history table lives in the first managed schema (Config.Schemas[0], which is current_schema() by default).
// retried every 200ms by the engine until LockWait; on timeout the holder query runs:
//   SELECT a.pid, a.application_name FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid
//   WHERE l.locktype = 'advisory' AND l.classid = ($1 >> 32)::oid AND l.objid = ($1 & 4294967295)::oid
// and migrate.ErrLocked is returned with "held by pid=N application_name=X".
// unlock: SELECT pg_advisory_unlock($1).
// Setup: SET lock_timeout = <ms>; SET statement_timeout = <ms> (session scope on the pinned connection); reset runs RESET lock_timeout; RESET statement_timeout.
```

Note: `SET` (not `SET LOCAL`) is used because the pinned connection stays session-scoped for the whole apply; reset restores the pool connection before release.

Steps:

- [ ] **Step 1: Failing tests**

`migrate/postgres/lock_test.go`:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"errors"
	"strings"
	"time"
)

func TestLockHolderNamed(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    key := "momo:" + testdb.Schema(t) + ".momo_migrations" // same schema => same key => contention
    unlockA, _, err := mpg.New().Lock(ctx, pinned(t, db), key)
    if err != nil {
        t.Fatal(err)
    }
    defer unlockA()
    _, _, err = mpg.New().Lock(ctx, pinned(t, db), key) // second connection, SAME schema
    if !errors.Is(err, migrate.ErrLocked) || !strings.Contains(err.Error(), "pid=") {
        t.Fatalf("want ErrLocked naming the holder pid, got %v", err)
    }
}

// Locks for different history schemas must not contend even though the advisory lock space is per database.
func TestLockKeyPerSchema(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    unlockA, _, err := mpg.New().Lock(ctx, pinned(t, db), "momo:"+testdb.Schema(t)+".momo_migrations")
    if err != nil {
        t.Fatal(err)
    }
    defer unlockA()
    unlockB, _, err := mpg.New().Lock(ctx, pinned(t, db), "momo:"+testdb.Schema(t)+"_other.momo_migrations")
    if err != nil {
        t.Fatalf("a lock keyed by another schema must not be blocked: %v", err)
    }
    defer unlockB()
}

func TestLockTimeoutApplied(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    c := pinned(t, db)
    reset, err := mpg.New().Setup(ctx, c, migrate.SessionOptions{LockTimeout: 1500 * time.Millisecond, StatementTimeout: 3 * time.Second})
    if err != nil {
        t.Fatal(err)
    }
    var lt string
    mustQueryRow(t, c, `SHOW lock_timeout`, &lt)
    if lt != "1500ms" {
        t.Fatalf("lock_timeout = %q", lt)
    }
    if err := reset(); err != nil {
        t.Fatal(err)
    }
}
```

Run: `MOMO_TEST_POSTGRES_URL=... go test ./migrate/postgres/ -run 'TestLockHolderNamed|TestLockTimeoutApplied' -count=1`. Expect FAIL with `undefined` symbols.

- [ ] **Step 2: Implement** per Interfaces. The key hash: `h := fnv.New64a(); h.Write([]byte(key)); k := int64(h.Sum64())`. The Lock function is non-blocking (`pg_try_advisory_lock`); the engine handles the wait loop.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(migrate/postgres): advisory migration lock with named holder and session timeouts"`.

## Task 15: Runner, history table, failure recovery, resolve

Covers: S-11 (transactional and non-transactional execution, partial-failure recovery, resolve, history with checksums and status), S-8 (ErrDirty), S-22 (history metadata row format_version, apply_id, kind). Acceptance: TestNonTransactionalFailureRecovery, TestTransactionalFailureRolledBack, TestHistoryFormatVersion.

Dialect selection: no global registry. `migrate.NewEngine` requires the explicit option `migrate.WithDialect(d migrate.Dialect)`; without it, NewEngine returns an error whose text names `migrate.WithDialect`. The CLI selects the dialect with a static switch (Task 29).

History semantics (normative):

- Transactional migration (default). One transaction contains: delete any `rolled_back` row for the version; insert a `running` row; run the statements (hints `fk-check` and `dependent:` handled as in Task 35); update the row to `applied` with statements_applied and execution_ms; commit. On any error the whole transaction is rolled back, so no `running` row survives. Then, in autocommit, insert a `rolled_back` row carrying the error text. A `rolled_back` row is NOT dirty.
- Non-transactional migration (NoTx, or any statement with NoTx). Autocommit: delete any `rolled_back` row; insert `running`; execute each statement autocommitted and update statements_applied after each success; on error update the row to `failed` with the error text and the count so far.
- Dirty means status `running` or `failed`. Apply refuses with ErrDirty while any dirty row exists.

Files:

- Create: `migrate/history.go`, `migrate/runner.go`, `migrate/engine.go` (Engine type, NewEngine with WithDialect, Resolve), `migrate/runner_test.go`, `migrate/history_test.go`, `migrate/helpers_test.go` (shared helpers, see Task 3).
- Modify: `migrate/options.go` (add `WithDialect`).

Interfaces (exact):

```go
// history.go
const (
    StatusRunning    = "running"
    StatusApplied    = "applied"
    StatusFailed     = "failed"
    StatusRolledBack = "rolled_back"
    StatusMeta       = "meta"
)
const MetaVersion = "__momo_meta__"
type HistoryRow struct {
    Version, Name, Checksum, Status, Error, ApplyID, Kind string
    StatementsApplied int
    AppliedAt         time.Time
    ExecutionMS       int64
}
func EnsureHistory(ctx context.Context, c momo.PinnedConn, d Dialect) error // creates the table from d.HistoryDDL(), inserts the metadata row (checksum = strconv.Itoa(FormatVersion)) when missing; refuses a higher format with an error naming the required momo version
func ReadHistory(ctx context.Context, c momo.PinnedConn) ([]HistoryRow, error)

// options.go
func WithDialect(d Dialect) Option // required by NewEngine

// engine.go
func NewEngine(drv momo.Driver, dir fs.FS, opts ...Option) (*Engine, error) // error "migrate: NewEngine requires migrate.WithDialect(...)" when no dialect is given
func (e *Engine) Resolve(ctx context.Context, version string, state ResolveState) error
// applied: failed or running row becomes applied (statements_applied kept). rolled_back: the failed or running row is deleted, so the version is pending again.
// Errors when the version has no dirty row.

// runner.go
type RunEnv struct {
    Conn    momo.PinnedConn
    Dialect Dialect
    Clock   func() time.Time
}
func RunMigration(ctx context.Context, env RunEnv, p Pending, applyID string) (Applied, error)
func RunStatements(ctx context.Context, c momo.PinnedConn, stmts []Statement) error // one transaction; honours hints fk-check and dependent:<type>:<name>
```

Steps:

- [ ] **Step 1: Failing non-transactional recovery test**

`migrate/runner_test.go` (package `migrate_test`):

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"errors"
	"testing/fstest"
)

func TestNonTransactionalFailureRecovery(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    fsys := migratetest.WithSum(t, fstest.MapFS{
        "20260101000000_bad.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE TABLE ok1 (x int);\nCREATE TABLE ok2 (x int);\nINSERT INTO missing VALUES (1);")},
    })
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
        t.Fatal("apply must fail")
    }
    row := historyRow(t, db, "20260101000000")
    if row.Status != migrate.StatusFailed || row.StatementsApplied != 2 || row.Error == "" {
        t.Fatalf("want failed with statements_applied=2, got %+v", row)
    }
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrDirty) {
        t.Fatalf("apply must refuse while dirty, got %v", err)
    }
    if err := e.Resolve(ctx, "20260101000000", migrate.ResolveRolledBack); err != nil {
        t.Fatal(err)
    }
    plan, err := e.Plan(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if len(plan) != 1 {
        t.Fatalf("after resolve the version is pending again, plan = %+v", plan)
    }
}
```

Run: `go test ./migrate/ -run TestNonTransactionalFailureRecovery -count=1`. Expect FAIL with `migrate: NewEngine requires migrate.WithDialect` (or `undefined` symbols) before Step 2.

- [ ] **Step 2: Implement history, runner (non-transactional path), engine and WithDialect**

Implement `EnsureHistory` (DDL from `d.HistoryDDL()`: table `momo_migrations` with columns `version TEXT PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, status TEXT NOT NULL, statements_applied INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '', applied_at TEXT NOT NULL, execution_ms INTEGER NOT NULL DEFAULT 0, apply_id TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL DEFAULT 'sql'`). `NewEngine` checks the dialect option first. `Apply` (minimal here: no lock, no checksum gate; Task 16 completes it) and `Plan` (pending list) use `RunMigration`. Run the test: expect PASS.

- [ ] **Step 3: Failing transactional test**

Append to `migrate/runner_test.go`:

```go
func TestTransactionalFailureRolledBack(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    bad := migratetest.WithSum(t, fstest.MapFS{
        "20260101000000_t.up.sql": {Data: []byte("CREATE TABLE a (x int);\nINSERT INTO missing VALUES (1);")},
    })
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), bad)
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
        t.Fatal("apply must fail")
    }
    if tableExists(t, db, "a") {
        t.Fatal("transactional failure must roll back the DDL")
    }
    row := historyRow(t, db, "20260101000000")
    if row.Status != migrate.StatusRolledBack || row.Error == "" {
        t.Fatalf("want rolled_back with the error text, got %+v", row)
    }
    fixed := migratetest.WithSum(t, fstest.MapFS{
        "20260101000000_t.up.sql": {Data: []byte("CREATE TABLE a (x int);")},
    })
    e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), fixed)
    if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatalf("a rolled-back transactional migration must apply again without resolve: %v", err)
    }
    if !tableExists(t, db, "a") {
        t.Fatal("the re-applied migration did not create a")
    }
}
```

Run: `go test ./migrate/ -run TestTransactionalFailureRolledBack -count=1`. Expect FAIL with the old dirty behaviour (apply refuses with ErrDirty) until Step 4.

- [ ] **Step 4: Implement the transactional path**

In `RunMigration`, for a transactional migration: begin the transaction on `env.Conn`; delete `rolled_back` rows for the version; insert the `running` row; run statements (`RunStatements` semantics for hints); update the row to `applied`; commit. On error roll back and insert the `rolled_back` row with the error text in autocommit. Run both runner tests: expect PASS.

- [ ] **Step 5: History format test**

`migrate/history_test.go`:

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"strings"
)

func TestHistoryFormatVersion(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    c := pinned(t, db)
    if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
        t.Fatal(err)
    }
    mustExec(t, c, `UPDATE momo_migrations SET checksum='99' WHERE version='__momo_meta__'`)
    err := migrate.EnsureHistory(ctx, c, mpg.New())
    if err == nil || !strings.Contains(err.Error(), "upgrade momo") {
        t.Fatalf("newer history format must be refused with an actionable error, got %v", err)
    }
}
```

Run: `go test ./migrate/ -run TestHistoryFormatVersion -count=1`. Expect PASS.

- [ ] **Step 6: Commit**

`git commit -m "feat(migrate): history inside the migration transaction, rolled_back is not dirty, explicit dialect option"`

## Task 16: Engine Apply, Plan, Status with lock, checksum and dirty gating

Covers: S-11 (concurrency lock, checksum refusal, apply to version), S-10 (`plan`, `apply [--to N]`, `status` contracts). Acceptance: TestConcurrentApply (SQLite, goroutines), TestChecksumMismatch (apply refuses), TestApplyTo, TestStatusStates.

Files:

- Modify: `migrate/engine.go`.
- Create: `migrate/engine_apply_test.go`.

Interfaces (exact):

```go
func (e *Engine) Plan(ctx context.Context) ([]Pending, error)
func (e *Engine) Apply(ctx context.Context, opts ApplyOptions) (*Report, error)
func (e *Engine) Status(ctx context.Context) ([]VersionStatus, error)
```

Apply algorithm (normative): (1) open a pinned connection and resolve the managed schemas with `resolveSchemas` (default `[current_schema()]`; `Schemas[0]` must equal `current_schema()` because the history table is referenced unqualified and so lives there); (2) `EnsureHistory`; (3) acquire the lock via `Dialect.Lock` with key `"momo:" + historySchema + "." + historyTable` (`historySchema` = resolved `Schemas[0]`, `historyTable` = `momo_migrations`; the key is hashed with fnv64a in Task 14, and SQLite ignores it) and `ApplyOptions.LockWait` (default 30s, polling every 200ms; on timeout return ErrLocked with the holder text); (4) `Dialect.Setup` with lock timeouts; (5) read history; if any row is `running` or `failed`, return ErrDirty naming the version; (6) verify `momo.sum` with VerifySum and every applied row's checksum against the file (ErrChecksumMismatch naming the file); (7) for each pending migration up to `opts.To` in version order, call RunMigration with a shared ApplyID (`time.Now().UTC().Format(time.RFC3339Nano)` from the clock); (8) unlock and reset in defer, joining errors.

Status states: `applied` (history applied, checksum matches), `pending`, `dirty` (failed or running), `mismatch` (history checksum differs from file), `missing` (history row without a file).

Steps:

- [ ] **Step 1: Failing tests**

`migrate/engine_apply_test.go`:

```go
package migrate_test

import (
    "context"
    "sync"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"errors"
	"testing/fstest"
)

func TestConcurrentApply(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    fsys := fstest.MapFS{
        "20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")},
        "20260101000001_b.up.sql": {Data: []byte("CREATE TABLE b (x int);")},
    }
    var wg sync.WaitGroup
    errs := make(chan error, 2)
    for i := 0; i < 2; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            e, err := migrate.NewEngine(db.Driver(), fsys, migrate.WithDialect(mpg.New()), migrate.WithLockWait(0))
            if err != nil {
                errs <- err
                return
            }
            _, err = e.Apply(ctx, migrate.ApplyOptions{})
            errs <- err
        }()
    }
    wg.Wait()
    close(errs)
    var locked int
    for err := range errs {
        if errors.Is(err, migrate.ErrLocked) {
            locked++
        } else if err != nil {
            t.Fatal(err)
        }
    }
    rows, err := migrate.ReadHistory(ctx, pinned(t, db))
    if err != nil {
    	t.Fatal(err)
    }
    applied := 0
    for _, r := range rows {
        if r.Status == migrate.StatusApplied {
            applied++
        }
    }
    if applied != 2 {
        t.Fatalf("each version must be applied exactly once, applied rows = %d", applied)
    }
}

func TestApplyTo(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    fsys := fstest.MapFS{
        "20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")},
        "20260101000001_b.up.sql": {Data: []byte("CREATE TABLE b (x int);")},
    }
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
    if _, err := e.Apply(ctx, migrate.ApplyOptions{To: "20260101000000"}); err != nil {
        t.Fatal(err)
    }
    plan, err := e.Plan(ctx)
    if err != nil {
    	t.Fatal(err)
    }
    if len(plan) != 1 || plan[0].Version != "20260101000001" {
        t.Fatalf("after --to the second version must remain pending: %+v", plan)
    }
}
```

`migrate.WithLockWait(d time.Duration) Option` is added in Task 16 to `options.go`. TestConcurrentApply must run and must not be skipped; both goroutines contend for the same SQLite file, so one of them is refused with ErrLocked and the other applies both versions. Run: `go test ./migrate/ -run 'TestConcurrentApply|TestApplyTo' -race -count=1`. Expect FAIL with `undefined: migrate.WithLockWait`.

- [ ] **Step 2: Implement** per the Apply algorithm. The lock key for SQLite is ignored (single table), for PostgreSQL it is derived from the history table name and hashed (Task 14). `Status` builds the list from Discover, history and VerifySum.

- [ ] **Step 3: Failing checksum test, then implement**

Add to `migrate/engine_apply_test.go`:

```go
func TestChecksumMismatchRefusesApply(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    orig := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}}
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), orig)
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    edited := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int, y int);")}}
    e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), edited)
    _, err := e2.Apply(ctx, migrate.ApplyOptions{})
    if !errors.Is(err, migrate.ErrChecksumMismatch) || !strings.Contains(err.Error(), "20260101000000_a.up.sql") {
        t.Fatalf("want ErrChecksumMismatch naming the file, got %v", err)
    }
}
```

Add `"strings"` import. Run: `go test ./migrate/ -run TestChecksumMismatchRefusesApply -count=1`. Expect PASS after implementation (note: `momo.sum` must exist for the orig fixture; `NewEngine` does not write it; the test must call `migrate.WriteSumForTest(t, orig)`—a helper in `migratetest` that writes momo.sum into the fixture map). Use `migratetest.WithSum(t, orig)` which returns the map plus `momo.sum` entry; apply `orig = migratetest.WithSum(t, orig)` before both engines so that the edited map keeps the old momo.sum (edited map is built from the original sum with the edited body substituted).

- [ ] **Step 4: Commit** `git commit -m "feat(migrate): apply and plan under lock with checksum and dirty gating"`

## Task 17: Diff writes files; no-change round trip (PostgreSQL)

Covers: S-10 (`migrate diff` contract), S-11 (no-change round trip), S-27 (snapshot parent). Acceptance: TestMigrationRoundTrip (SQLite), TestNoChangeWritesNothing.

Files:

- Modify: `migrate/engine.go` (Engine.Diff), `migrate/diff.go` (file rendering helpers).
- Create: `migrate/diff_engine_test.go`.

Interfaces (exact):

```go
func (e *Engine) Diff(ctx context.Context, desired *schema.Schema, name string, opts DiffOptions) (*Migration, error)
// Returns nil, nil when there are no changes (nothing to write).
// Otherwise Migration.Files contains:
//   <version>_<name>.up.sql     (header: -- momo:generated, then -- momo:destructive if applicable, then "-- momo:no-transaction" if NoTx, then statements separated by ";\n\n")
//   <version>_<name>.snapshot.json (WriteSnapshot with ParentVersion = latest version or "")
//   momo.sum                    (AppendSum over the new files)
// The version is the engine clock formatted as 20060102150405 in UTC.
// name must match ^[a-z][a-z0-9_]*$ (else error).
```

The Engine's `Diff` is also the only place that calls `migrate.Diff` for the engine: it loads the latest snapshot (`LatestSnapshot`) as `from` (empty schema for the first migration, Dialect set), and uses the dialect's `Render` for the statements. Rebuild markers are expanded by the sqlite dialect in `RunStatements`, so Diff stores them as `Statement{Hint:"rebuild"}`, and the up file for a rebuild writes a comment line `-- momo:rebuild table=<name>` followed by the statements the sqlite dialect generates with `Rebuild` at render time; because Rebuild needs a live connection, Diff for SQLite requires the Engine to have a connection (Diff is online for SQLite rebuilds, offline otherwise) and this is documented in the function comment.

Steps:

- [ ] **Step 1: Failing round-trip test**

`migrate/diff_engine_test.go`:

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestMigrationRoundTrip(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    desired := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
        Name:       "docs",
        Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "title", Type: "TEXT", Default: "''"}},
        PrimaryKey: []string{"id"},
        Indexes:    []schema.Index{{Name: "docs_title", Parts: []schema.IndexPart{{Column: "title"}}}},
    }}}
    m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
    mig, err := m.Diff(ctx, desired, "init", migrate.DiffOptions{})
    if err != nil || mig == nil {
        t.Fatalf("diff: %v %v", mig, err)
    }
    fsys := migratetest.FS(mig.Files)
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    inspected, err := e.Inspect(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if !schema.Equal(inspected, desired) {
        t.Fatalf("round trip not empty")
    }
    again, err := e.Diff(ctx, desired, "noop", migrate.DiffOptions{})
    if err != nil || again != nil {
        t.Fatalf("no-change diff must write nothing: %v %v", again, err)
    }
}
```

Helpers `emptyFS(t)` (empty `fstest.MapFS`), `filesFS(map[string][]byte) fs.FS`, and `Engine.Inspect(ctx) (*schema.Schema, error)` (opens a pinned connection and calls the dialect's Inspect with the configured Schemas) are added in this task. Test name `TestNoChangeWritesNothing` is the second half of the same test (kept as a separate function that calls `Diff` twice). Run: `go test ./migrate/ -run TestMigrationRoundTrip -count=1`. Expect FAIL with `m.Diff undefined`.

- [ ] **Step 2: Implement** `Engine.Diff`, `Engine.Inspect`, and the file-rendering helpers, following the Interfaces. Diff is pure with respect to the database except for the snapshot read: the up file is the rendering of the change list through the dialect's `Render`. PostgreSQL needs nothing more. SQLite table rebuilds (Task 35) are rendered by the pure `RebuildStatements`, which takes the old column list and the captured dependents; the SQLite Diff path is enabled in Task 35 and is not needed here.

- [ ] **Step 3: Run, expect PASS** with `go test ./migrate/ -run 'TestMigrationRoundTrip|TestNoChangeWritesNothing' -count=1`.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate): diff writes versioned files with snapshot parent; sqlite round trip"`

## Task 18: Baseline and drift

Covers: S-11 (baselining, drift detection, unmanaged-object ownership), S-10 (`baseline --version N`, `drift`). Acceptance: TestBaselineAndDrift.

Files:

- Create: `migrate/baseline.go`, `migrate/drift.go`, `migrate/baseline_test.go`.

Interfaces (exact):

```go
func (e *Engine) Baseline(ctx context.Context, version string, force bool) error
// Verifies that Inspect(live) Equals the snapshot of version (ErrNoSnapshot if missing; mismatch error lists the differing object kinds unless force).
// Inserts a history row (status applied, statements_applied 0, checksum from momo.sum, apply_id "baseline") for every migration with version <= version that has no row. Executes no DDL.
func (e *Engine) Drift(ctx context.Context) (*DriftReport, error)
// from = snapshot of the latest applied version (the empty schema when no migration is applied, so every live object is unmanaged).
// Changed: objects in both whose normalized JSON differs (Detail names the field).
// Missing: in snapshot, not in live DB.
// Unmanaged: in live DB, not in snapshot, not matching Config.Exclude globs. Never proposed for drop.
```

Object identity for drift: `Kind` (table, column, index, fk, unique, check, view, function, trigger, policy, enum, extension, schema) plus qualified name `schema.table.name`.

Steps:

- [ ] **Step 1: Failing test**

`migrate/baseline_test.go`:

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestBaselineAndDrift(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    fsys := migratetestFS(t, "20260101000000_init") // helper: writes a create table migration + snapshot + momo.sum via migratetest
    mustExec(t, db.Driver(), `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
    before := countDDL(t, db)
    if err := e.Baseline(ctx, "20260101000000", false); err != nil {
        t.Fatalf("baseline: %v", err)
    }
    if countDDL(t, db)-before != 0 {
        t.Fatal("baseline must execute no DDL")
    }
    mustExec(t, db.Driver(), `ALTER TABLE users ADD COLUMN extra TEXT`)
    mustExec(t, db.Driver(), `CREATE TABLE unmanaged_audit (id INTEGER PRIMARY KEY)`)
    rep, err := e.Drift(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if len(rep.Changed) == 0 {
        t.Fatal("altered column not reported as changed")
    }
    if len(rep.Unmanaged) != 1 || rep.Unmanaged[0].Name != "unmanaged_audit" {
        t.Fatalf("unmanaged table not listed: %+v", rep.Unmanaged)
    }
    plan, err := e.Plan(ctx)
    if err != nil {
    	t.Fatal(err)
    }
    if len(plan) != 0 {
        t.Fatal("baselined version must not be pending")
    }
}
```

`migratetestFS(t, version)` and `countDDL(t, db)` (counts statements executed through the CountingDriver from migratetest) are helpers in migratetest / the test package. Run: `go test ./migrate/ -run TestBaselineAndDrift -count=1`. Expect FAIL with `e.Baseline undefined`.

- [ ] **Step 2: Implement** Baseline and Drift per the Interfaces. Drift compares by building a map of object identity to normalized JSON for both sides.

- [ ] **Step 3: Run, expect PASS** with the same command.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate): baseline without DDL and drift with unmanaged-object reporting"`

## Task 19: Optional authored down migrations and group rollback

Covers: S-11 (optional authored down migrations), S-94 (group rollback of the last applied group, reverse order; this task OWNS group rollback by apply_id, plan momo-11 Task 13 only consumes it). Acceptance: TestRollbackGroup, TestRollbackNoDown. Go-function downs: for a history row of kind `go`, Rollback looks up the registered `GoMigration` by version in `e.goMigrations` (type and option defined in plan momo-11 Task 13) and calls its Down with the transaction, or with the pinned DB when NoTx.

Files:

- Create: `migrate/rollback.go`, `migrate/rollback_test.go`.

Interfaces (exact):

```go
type RollbackOptions struct{ Group bool } // default rolls back the last applied migration only
func (e *Engine) Rollback(ctx context.Context, opts RollbackOptions) (*Report, error)
// Selects the applied rows with the greatest apply_id (Group) or the greatest applied_at (single).
// Runs each selected version's .down.sql in reverse version order. Missing down file: ErrNoDown, nothing runs.
// On success the history row status becomes rolled_back. Refuses when dirty.
```

Steps:

- [ ] **Step 1: Failing test**

`migrate/rollback_test.go`:

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"errors"
	"testing/fstest"
)

func TestRollbackGroup(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    fsys := migratetest.WithSum(t, fstest.MapFS{
        "20260101000000_a.up.sql":   {Data: []byte("CREATE TABLE a (x int);")},
        "20260101000000_a.down.sql": {Data: []byte("DROP TABLE a;")},
        "20260101000001_b.up.sql":   {Data: []byte("CREATE TABLE b (x int);")},
        "20260101000001_b.down.sql": {Data: []byte("DROP TABLE b;")},
    })
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    if _, err := e.Rollback(ctx, migrate.RollbackOptions{Group: true}); err != nil {
        t.Fatal(err)
    }
    if tableExists(t, db, "a") || tableExists(t, db, "b") {
        t.Fatal("group rollback must drop both tables")
    }
}

func TestRollbackNoDown(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    fsys := migratetest.WithSum(t, fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}})
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
    	t.Fatal(err)
    }
    if _, err := e.Rollback(ctx, migrate.RollbackOptions{}); !errors.Is(err, migrate.ErrNoDown) {
        t.Fatalf("want ErrNoDown, got %v", err)
    }
    if !tableExists(t, db, "a") {
        t.Fatal("nothing may run when a down file is missing")
    }
}
```

`tableExists` helper in test file. Run: `go test ./migrate/ -run 'TestRollback' -count=1`. Expect FAIL with `e.Rollback undefined`.

- [ ] **Step 2: Implement** Rollback: validate all selected down files exist first; then run each down through RunMigration-like execution with kind "down" recorded in the status update (the row becomes rolled_back in the same transaction as the DDL). Reverse order means descending version.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(migrate): authored down migrations and group rollback"`.

## Task 20: AutoMigrate additive

Covers: S-14 (additive only, reports skipped changes), S-11 (no DROP or ALTER TYPE from AutoMigrate). Acceptance: TestAutoMigrateAdditive.

Files:

- Create: `migrate/automigrate.go`, `migrate/automigrate_test.go`.

Interfaces (exact):

```go
func (e *Engine) AutoMigrate(ctx context.Context, desired *schema.Schema) (*AutoReport, error)
// Inspects live DB; Diff(live, desired, AllowDestructive=false, Stage=false) with a filter:
//  kept: create_table, add_column (nullable, or NOT NULL with non-NULL default, or with a default), create_index, add_unique? (no: unique is an index on existing data; it is skipped with a reason), create_enum? (SQLite: unsupported -> skipped)
//  skipped (with reason): drop_*, rename_*, alter_column, add_fk, add_check, add_primary_key, anything with Destructive true, NOT NULL without default.
// Applies kept changes in one transaction when the dialect has TransactionalDDL, else statement by statement. Writes no history rows.
```

Steps:

- [ ] **Step 1: Failing test**

`migrate/automigrate_test.go`:

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestAutoMigrateAdditive(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    mustExec(t, db.Driver(), `CREATE TABLE things (id INTEGER PRIMARY KEY, kind TEXT, old_col TEXT)`)
    desired := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{
        {Name: "things", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "kind", Type: "INTEGER"}, {Name: "added", Type: "TEXT", Nullable: true}}, PrimaryKey: []string{"id"}},
        {Name: "fresh", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}},
    }}
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
    rep, err := e.AutoMigrate(ctx, desired)
    if err != nil {
        t.Fatal(err)
    }
    if !tableExists(t, db, "fresh") || !columnExists(t, db, "things", "added") {
        t.Fatal("additive changes not applied")
    }
    if !columnExists(t, db, "things", "old_col") {
        t.Fatal("AutoMigrate must never drop a column")
    }
    if len(rep.Skipped) != 2 {
        t.Fatalf("type change and removed column must be skipped, got %+v", rep.Skipped)
    }
}
```

`columnExists` helper uses information_schema.columns. Run: `go test ./migrate/ -run TestAutoMigrateAdditive -count=1`. Expect FAIL with `e.AutoMigrate undefined`.

- [ ] **Step 2: Implement** per Interfaces. Skipped entries: the type change on `things.kind` (alter_column) and the removed `old_col` (drop_column is not generated because AutoMigrate's diff is run with the removed column treated as `drop_column` and then filtered out) — the reason strings are `"type change requires a migration"` and `"column removed in models; AutoMigrate never drops"`.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(migrate): additive AutoMigrate with skipped-change report"`.

## Task 21: Staged changes end to end (PostgreSQL)

Covers: S-11 (staged changes on a populated DB: add nullable, backfill with a data migration, SET NOT NULL). Acceptance: TestStagedChange (full, populated database).

Files:

- Create: `migrate/staged_test.go`.
- Modify: `migrate/diff.go` (SET NOT NULL emission after backfill).

Interfaces: no new names. Behavior: after `diff --stage` writes the nullable add, a hand-written data migration (`momo migrate new --custom backfill`, which creates `<version>_backfill.up.sql` with snapshot = parent snapshot and header `-- momo:custom`) updates the rows; then the next `Diff` against the desired schema emits `alter_column` (NOT NULL) because the snapshot still shows nullable: the backfill migration's snapshot is the parent's, so this is correct only if the backfill migration advances the snapshot. Decision recorded here: a custom migration's snapshot is copied from the parent; the staged column add's snapshot (written by `--stage`) already contains the nullable column, so the NOT NULL alter is emitted and the rows are verified by the statement `SELECT count(*) FROM t WHERE col IS NULL` executed by the SET NOT NULL migration's guard `-- momo:guard` (new directive: the first statement must return zero rows, otherwise the migration fails before changing anything).

Add directive `-- momo:guard` to `files.go` `Directives{Guard bool}`: the runner executes the guard query `SELECT count(*) FROM (...)`? Simplify: the first statement of a guarded migration is a SELECT; if it returns a non-zero count the migration fails with error "guard failed: N rows" and status failed. Implementation: runner evaluates the first statement via Query and requires an integer 0 in the first column.

Steps:

- [ ] **Step 1: Failing test**

`migrate/staged_test.go`:

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestStagedChange(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    v1 := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}}}}
    v2 := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "tenant", Type: "INTEGER", Nullable: false}}, PrimaryKey: []string{"id"}}}}
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
    m1, err := e.Diff(ctx, v1, "init", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    e1 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(m1.Files))
    if _, err := e1.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    mustExec(t, db.Driver(), `INSERT INTO t (id) VALUES (1), (2)`)
    if _, err := e1.Diff(ctx, v2, "tenant", migrate.DiffOptions{}); err == nil {
        t.Fatal("populated table: NOT NULL without default must need staging")
    }
    m2, err := e1.Diff(ctx, v2, "tenant", migrate.DiffOptions{Stage: true})
    if err != nil {
        t.Fatal(err)
    }
    e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(migratetest.MergeMaps(m1.Files, m2.Files)))
    if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    mustExec(t, db.Driver(), `UPDATE t SET tenant = id`)
    backfill, err := e2.NewCustom(ctx, "backfill")
    if err != nil {
        t.Fatal(err)
    }
    mustExec(t, db.Driver(), `UPDATE t SET tenant = 1 WHERE tenant IS NULL`)
    m3, err := e2.Diff(ctx, v2, "not_null", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if len(m3.Changes) != 1 || m3.Changes[0].Kind != "alter_column" {
        t.Fatalf("NOT NULL after backfill must be an alter_column, got %+v", m3.Changes)
    }
}
```

`Engine.NewCustom(ctx, name) (*Migration, error)` and `merge(a, b map[string][]byte) map[string][]byte` are helpers introduced in this task (NewCustom writes a `.up.sql` with header `-- momo:custom` and a snapshot copied from the parent). Run: `go test ./migrate/ -run TestStagedChange -count=1`. Expect FAIL with `e1.NewCustom undefined` or `Diff` returning no error.

Note: Task 8's `Diff` returns `ErrStagedRequired` for NULL to NOT NULL without default and `TestStagedNotNullOnExistingColumn` (`migrate/diff_test.go`) asserts it. This task must change that rule in `migrate/diff.go` and that test together.

- [ ] **Step 2: Implement** `NewCustom`, the `-- momo:guard` directive and runner check, and make Diff's staged logic emit `alter_column` NOT NULL when the snapshot shows a nullable column that the desired schema declares NOT NULL without default (no hint needed beyond the documented guard).

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(migrate): staged NOT NULL with guarded set-not-null migration"`.

Task 21 addition (custom SQL decision):

Second test (custom SQL that changes a modeled table; decision: the snapshot written by `migrate diff` is authoritative, a custom migration copies the parent snapshot, and drift reports what the models do not describe). File: `migrate/custom_test.go` (package `migrate_test`):

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"strings"
	"bytes"
	"testing/fstest"
)

func TestCustomMigrationDriftMismatch(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    desired := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}}}}
    e1 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
    mig, err := e1.Diff(ctx, desired, "init", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if mig == nil {
        t.Fatal("expected an init migration")
    }
    var snapBody []byte
    for name, body := range mig.Files {
        if strings.HasSuffix(name, ".snapshot.json") {
            snapBody = body
        }
    }
    parent, err := migrate.ReadSnapshot(bytes.NewReader(snapBody))
    if err != nil {
        t.Fatal(err)
    }
    custom := migrate.SnapshotFile{Format: parent.Format, Version: "20260102000000", ParentVersion: parent.Version, Schema: parent.Schema}
    var buf bytes.Buffer
    if err := migrate.WriteSnapshot(&buf, custom); err != nil {
        t.Fatal(err)
    }
    files := fstest.MapFS{}
    for name, body := range mig.Files {
        if name != "momo.sum" {
            files[name] = &fstest.MapFile{Data: body}
        }
    }
    files["20260102000000_addcol.up.sql"] = &fstest.MapFile{Data: []byte("ALTER TABLE t ADD COLUMN extra TEXT;")}
    files["20260102000000_addcol.snapshot.json"] = &fstest.MapFile{Data: buf.Bytes()}
    e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.WithSum(t, files))
    if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    again, err := e2.Diff(ctx, desired, "noop", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if again != nil {
        t.Fatalf("the diff trusts the snapshot and must see no change, got %+v", again.Changes)
    }
    rep, err := e2.Drift(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if rep.Clean() {
        t.Fatal("a column added by custom SQL that the models do not describe must be reported by drift")
    }
}
```

Run: `go test ./migrate/ -run TestCustomMigrationDriftMismatch -count=1`. Expect FAIL with `undefined: migratetest.WithSum` until Task 3 and Task 5 exist, then PASS.

## Task 22: PostgreSQL schemas and drift across configured schemas

Covers: S-36 (schema-qualified tables, creation of extra schemas, drift across configured schemas). Acceptance: TestPostgresSchemas.

Files:

- Create: `migrate/postgres/schemas_test.go`.
- Modify: `migrate/postgres/render.go` (create_schema renders `CREATE SCHEMA IF NOT EXISTS "name"`).

Interfaces: none new. Behavior: the extra namespace is named `testdb.Schema(t) + "_billing"` (the harness drops only `t_xxx`, so the test drops its own extra schema in `t.Cleanup`: `DROP SCHEMA ... CASCADE`, error reported with `t.Errorf`). A table in that namespace gets `"<extra>"."invoices"`; a table with empty `Schema` lives in the current (per-test) schema and is rendered unqualified. `Inspect` with `Schemas: [current, extra]` returns the extra namespace's tables with `Schema` set and the current schema's tables with `Schema == ""`; `Drift` labels unmanaged objects with their schema.

Steps:

- [ ] **Step 1: Failing test**

`migrate/postgres/schemas_test.go`:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestPostgresSchemas(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    cur := testdb.Schema(t)
    extra := cur + "_billing" // derived from the per-test schema; the harness drops only t_xxx
    t.Cleanup(func() {
        if _, err := db.Driver().Exec(context.Background(), `DROP SCHEMA IF EXISTS "`+extra+`" CASCADE`, nil); err != nil {
            t.Errorf("drop extra schema %s: %v", extra, err)
        }
    })
    cfg := migrate.WithConfig(migrate.Config{Schemas: []string{cur, extra}})
    desired := &schema.Schema{
        Dialect: "postgres",
        Schemas: []schema.SchemaNamespace{{Name: extra}},
        Tables: []schema.Table{
            {Schema: extra, Name: "invoices", Columns: []schema.Column{{Name: "id", Type: "bigint"}}, PrimaryKey: []string{"id"}},
            {Name: "users", Columns: []schema.Column{{Name: "id", Type: "bigint"}}, PrimaryKey: []string{"id"}},
        },
    }
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS(), cfg)
    mig, err := e.Diff(ctx, desired, "schemas", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files), cfg)
    if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    if !tableExistsIn(t, db, extra, "invoices") {
        t.Fatal("schema-qualified table not created")
    }
    mustExec(t, db.Driver(), `CREATE TABLE "`+extra+`".stray (id bigint)`)
    rep, err := e2.Drift(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if len(rep.Unmanaged) != 1 || rep.Unmanaged[0].Schema != extra || rep.Unmanaged[0].Name != "stray" {
        t.Fatalf("drift must report the stray table with its schema: %+v", rep.Unmanaged)
    }
}
```

Run: `MOMO_TEST_POSTGRES_URL=... go test ./migrate/postgres/ -run TestPostgresSchemas -count=1`. Expect FAIL with `tableExistsIn undefined` or missing schema qualification.

- [ ] **Step 2: Implement** the schema-qualified identifiers in render and the `create_schema` renderer; ensure `Drift` sets `ObjectRef.Schema`.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(migrate/postgres): schema-qualified objects and drift across schemas"`.

## Task 23: Per-transaction row-level-security scope helper

Covers: S-38 (per-transaction set_config helper on momo.Tx; the database-enforced boundary). Acceptance: TestRowLevelSecurity.

Files:

- Create: `tx_scope.go` (root package `momo`), `tx_scope_test.go` in package `momo_test` under `internal/rlstest`? Put the integration test in `migrate/postgres/rls_test.go`, which can import both momo and migrate.

Interfaces (exact):

```go
// tx_scope.go, package momo
// SetLocal sets a transaction-scoped session variable (set_config(name, value, true)).
// It must be called inside a transaction; outside one it returns ErrUnsupported with a message naming the requirement.
func (tx *Tx) SetLocal(ctx context.Context, name, value string) error
```

Implementation: `SELECT set_config($1, $2, true)` through the transaction's connection; the dialect check uses the Dialect capability (`Savepoints`-style capability flag is not needed; the method is PostgreSQL-only and returns ErrUnsupported on SQLite).

Steps:

- [ ] **Step 1: Failing test**

`migrate/postgres/rls_test.go`:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	testdb "github.com/azrtydxb/momo/internal/testdb"
)

func TestRowLevelSecurity(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    desired := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
        Name:       "rows",
        Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "tenant", Type: "text"}},
        PrimaryKey: []string{"id"},
        RLS:        &schema.RLS{Enabled: true, Forced: true},
    }}}
    desired.Policies = []schema.Policy{{Table: "rows", Name: "tenant_only", Command: "ALL", Permissive: true, Using: "tenant = current_setting('app.tenant', true)", Check: "tenant = current_setting('app.tenant', true)"}}
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
    mig, err := e.Diff(ctx, desired, "rls", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
    if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    mustExec(t, db.Driver(), `INSERT INTO rows (id, tenant) VALUES (1, 'a'), (2, 'b')`)
    appDB := openAppRole(t, testdb.PostgresAppRoleURL(t)) // migratetest helper: a *momo.DB connected as the non-superuser app role from testdb.PostgresAppRole
    var n int
    err = appDB.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
        if err := tx.SetLocal(ctx, "app.tenant", "a"); err != nil {
            return err
        }
        mustQueryRow(t, tx, `SELECT count(*) FROM rows`, &n)
        return nil
    })
    if err != nil || n != 1 {
        t.Fatalf("tenant a must see one row, got %d (%v)", n, err)
    }
    // The setting is transaction-scoped: after commit the pooled connection must not carry it.
    var setting string
    mustQueryRow(t, appDB.Driver(), `SELECT coalesce(current_setting('app.tenant', true), '')`, &setting)
    if setting != "" {
        t.Fatalf("set_config leaked past the transaction: %q", setting)
    }
}
```

Run with integration URL: `MOMO_TEST_POSTGRES_URL=... go test ./migrate/postgres/ -run TestRowLevelSecurity -count=1`. Expect FAIL with `tx.SetLocal undefined` before Step 2.

- [ ] **Step 2: Implement** `tx_scope.go` per Interfaces. The file is added to the root package owned by plan momo-01 (report the addition to the lead).

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat: transaction-scoped set_config helper for row-level security"`.

## Task 24: Least-privilege roles: DDL refused for the application role

Covers: S-47 (migration role versus application role, integration tests run as non-superuser). Acceptance: TestAppRoleCannotDDL.

Files:

- Create: `migrate/postgres/approle_test.go`.

Interfaces: uses `testdb.PostgresAppRoleURL(t) (dsn string)` (plan momo-01 contract, see Contract changes item 8).

Steps:

- [ ] **Step 1: Failing test**

`migrate/postgres/approle_test.go`:

```go
package postgres_test

import (
    "context"
    "strings"
    "testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	testdb "github.com/azrtydxb/momo/internal/testdb"
)

func TestAppRoleCannotDDL(t *testing.T) {
    testopen.Postgres(t)
    // The harness (internal/testdb, plan momo-01) gives the app role USAGE on the per-test schema plus default
    // privileges (SELECT, INSERT, UPDATE, DELETE on tables; USAGE, SELECT on sequences) for objects the admin
    // creates there, and deliberately NO CREATE. This test grants nothing itself.
    appDSN := testdb.PostgresAppRoleURL(t)
    app := openPG(t, appDSN)
    ctx := context.Background()
    var super bool
    mustQueryRow(t, app.Driver(), `SELECT rolsuper FROM pg_roles WHERE rolname = current_user`, &super)
    if super {
        t.Fatal("application role must not be a superuser")
    }
    // search_path is t_xxx,public: an unqualified CREATE TABLE targets t_xxx, where the role has no CREATE.
    _, err := app.Driver().Exec(ctx, `CREATE TABLE should_fail (x int)`, nil)
    if err == nil || !strings.Contains(err.Error(), "permission denied") {
        t.Fatalf("application role must not run DDL (want permission denied), got %v", err)
    }
}
```

`openPG(t, dsn)` opens a `*momo.DB` through pgxdriver. Run with integration URL and `MOMO_TEST_APP_ROLE` ... (no extra env; the harness provisions the role). Expect FAIL with `testdb.PostgresAppRoleURL undefined` until plan momo-01 adds it.

- [ ] **Step 2: Implement** only the test; the production behaviour is already the PostgreSQL permission model. Add a CI assertion note in the test file comment that CI must run integration tests with this role (the CI workflow itself belongs to plan momo-01).

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "test(migrate/postgres): application role cannot run DDL"`.

## Task 25: PostgreSQL end-to-end apply with the history contract

Covers: S-11 (PostgreSQL transactional apply, failure rolled back, non-transactional CONCURRENTLY migration), S-12. Acceptance: TestPostgresApplyRecovery, TestCreateIndexConcurrentlyApply.

Files:

- Create: `migrate/postgres/apply_test.go`.

Steps:

- [ ] **Step 1: Failing test**

`migrate/postgres/apply_test.go`:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"testing/fstest"
)

func TestCreateIndexConcurrentlyApply(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    fsys := migratetest.WithSum(t, fstest.MapFS{
        "20260101000000_a.up.sql": {Data: []byte("CREATE TABLE d (id bigint PRIMARY KEY, title text);")},
        "20260101000001_idx.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE INDEX CONCURRENTLY d_title ON d (title);")},
    })
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    var valid bool
    mustQueryRow(t, pinned(t, db), `SELECT indisvalid FROM pg_index WHERE indexrelid = 'd_title'::regclass`, &valid)
    if !valid {
        t.Fatal("concurrent index must be valid")
    }
}

func TestPostgresApplyRecovery(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    fsys := migratetest.WithSum(t, fstest.MapFS{
        "20260101000000_bad.up.sql": {Data: []byte("CREATE TABLE ok (x int);\nINSERT INTO missing VALUES (1);")},
    })
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
    if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
        t.Fatal("apply must fail")
    }
    if tableExistsIn(t, db, testdb.Schema(t), "ok") {
        t.Fatal("transactional PostgreSQL migration must roll back DDL")
    }
}
```

Run: `MOMO_TEST_POSTGRES_URL=... go test ./migrate/postgres/ -run 'TestCreateIndexConcurrentlyApply|TestPostgresApplyRecovery' -count=1`. Expect FAIL with `undefined: ... postgres dialect not registered` until the engine is constructed with `migrate.WithDialect`.

- [ ] **Step 2: Implement** the TestMain registration and any missing wiring (the runner already runs `Setup` and `Lock` through the dialect). Verify that a CONCURRENTLY migration runs its statement outside a transaction (Statement.NoTx) and that the history row is written as applied.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "test(migrate/postgres): end-to-end apply, rollback and concurrent index"`.

## Task 26: Migration lint with severities and safer alternatives

Covers: S-28 (lint rules, severity, alternative, exit 1 on errors). Acceptance: TestMigrateLint.

Files:

- Create: `migrate/lint.go`, `migrate/lint_test.go`.

Interfaces (exact):

```go
func (e *Engine) Lint(ctx context.Context) ([]Finding, error) // pending migrations only
func LintSQL(version string, stmts []Statement, parentTables map[string]bool, dialectName string) []Finding
```

Rules (SQL-text heuristics on whitespace-collapsed, lowercased statements; each rule documented with its limit):

- R1 `not-null-without-default` (error) when `ALTER TABLE <t> ADD COLUMN <c> ... NOT NULL` without `DEFAULT`, or `ALTER TABLE <t> ALTER COLUMN <c> SET NOT NULL`, and `<t>` is in parentTables. Alternative: "add the column nullable, backfill in a separate migration, then SET NOT NULL with a -- momo:guard"
- R2 `column-type-change` (error) for `ALTER COLUMN <c> TYPE`, on a table in parentTables. Alternative: "add a new column, dual-write, backfill, switch reads, drop the old column in a later migration"
- R3 `blocking-index` (error) for `CREATE INDEX` without `CONCURRENTLY` on a table in parentTables (PostgreSQL only). Alternative: "CREATE INDEX CONCURRENTLY in a -- momo:no-transaction migration"
- R4 `foreign-key-without-not-valid` (error) for `ADD CONSTRAINT ... FOREIGN KEY` or `ADD CONSTRAINT ... CHECK` without `NOT VALID` on a table in parentTables. Alternative: "add NOT VALID, then VALIDATE CONSTRAINT in a separate migration"
- R5 `sqlite-rebuild` (warning) for a statement marked `Hint == "rebuild"`; with a row count supplied by the caller (`LintSQL` takes no count; the engine adds the count from `--db` as an error when > 100000 in Task 29) the default severity is warning. Alternative: "batch the data copy or split the table"
- R6 `enum-add-and-use` (error) when a migration contains `ALTER TYPE ... ADD VALUE` and a later statement in the same migration references the new value literal. Alternative: "move the use to a later migration; a new enum value is unusable until its transaction commits"
- R7 `destructive-without-directive` (error) for `DROP TABLE`, `DROP COLUMN`, `TRUNCATE`, `DROP SCHEMA`, `DROP TYPE` when the migration lacks `-- momo:destructive`. Alternative: "add -- momo:destructive after a review"

Steps:

- [ ] **Step 1: Failing test**

`migrate/lint_test.go`:

```go
package migrate

import "testing"

func TestMigrateLint(t *testing.T) {
    parent := map[string]bool{"docs": true}
    cases := []struct {
        name string
        sql  string
        rule string
    }{
        {"not null add", "ALTER TABLE docs ADD COLUMN tenant bigint NOT NULL;", "not-null-without-default"},
        {"type change", "ALTER TABLE docs ALTER COLUMN title TYPE varchar(10);", "column-type-change"},
        {"blocking index", "CREATE INDEX docs_title ON docs (title);", "blocking-index"},
        {"fk without not valid", "ALTER TABLE docs ADD CONSTRAINT fk FOREIGN KEY (org) REFERENCES orgs(id);", "foreign-key-without-not-valid"},
        {"drop table", "DROP TABLE docs;", "destructive-without-directive"},
    }
    for _, c := range cases {
        got := LintSQL("20260101000000", []Statement{{SQL: c.sql}}, parent, "postgres")
        found := false
        for _, f := range got {
            if f.Rule == c.rule && f.Severity == "error" && f.Alternative != "" {
                found = true
            }
        }
        if !found {
            t.Errorf("%s: rule %s not reported with severity and alternative: %+v", c.name, c.rule, got)
        }
    }
}
```

Run: `go test ./migrate/ -run TestMigrateLint -count=1`. Expect FAIL with `undefined: LintSQL`.

- [ ] **Step 2: Implement** `LintSQL` with the regular expressions for each rule (compiled once as package variables; regexp.MustCompile on literal patterns is acceptable). Implement `Engine.Lint` by loading pending migrations, building parentTables from the parent snapshot's table names, and calling LintSQL. The CLI exit code logic is in Task 29.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(migrate): lint rules for blocking, rewriting and destructive operations"`.

## Task 27: Rebase onto a target directory or version

Covers: S-27 (`migrate rebase`, no-change round trip after rebase). Acceptance: TestMigrateRebase.

Files:

- Create: `migrate/rebase.go`, `migrate/rebase_test.go`.

Interfaces (exact):

```go
// Rebase regenerates this branch's migrations that are absent from the target on top of the target's latest snapshot.
// target is a version string or a directory of the target branch's migrations (fs.FS).
// Refuses with an error naming the file when a branch migration has no momo:generated header (hand-written SQL cannot be regenerated).
// Returns the removed file names and the new Migration (nil when nothing changes).
func (e *Engine) Rebase(ctx context.Context, target fs.FS, desired *schema.Schema, name string) (removed []string, mig *Migration, err error)
```

The CLI maps `--onto <dir>` to `os.DirFS(dir)` and `--onto <version>` to the snapshot of that version in the current directory (Task 29). There is no git dependency: the user runs git to produce the target directory.

Steps:

- [ ] **Step 1: Failing test**

`migrate/rebase_test.go`:

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestMigrateRebase(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    base := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}}}}
    mine := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "a", Type: "TEXT", Nullable: true}}, PrimaryKey: []string{"id"}}}}
    target := migratetest.FSFromDiff(t, db, base, "root")               // target branch: root migration only
    branchFS := migratetest.FSFromDiff(t, db, base, "root")            // this branch starts from the same root...
    e := migratetest.NewEngine(t, mpg.New(), db.Driver(), branchFS)
    m2, err := e.Diff(ctx, mine, "add_a", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    branchFS = migratetest.Merge(branchFS, m2.Files)
    e = migratetest.NewEngine(t, mpg.New(), db.Driver(), branchFS)
    removed, mig, err := e.Rebase(ctx, target, mine, "add_a")
    if err != nil {
        t.Fatal(err)
    }
    if len(removed) != 2 || mig == nil || mig.Parent == "" {
        t.Fatalf("rebase must drop the branch's generated files and regenerate on the target head: removed=%v mig=%+v", removed, mig)
    }
    if _, err := migrate.ValidateOffline(migratetest.Merge(target, mig.Files)); err != nil {
        t.Fatal(err)
    }
}
```

Run: `go test ./migrate/ -run TestMigrateRebase -count=1`. Expect FAIL with `e.Rebase undefined`.

- [ ] **Step 2: Implement** Rebase: compute the set of versions in the branch absent from target; for each, check the first lines for `-- momo:generated` (refuse otherwise); remove them from the in-memory file set; take the target's latest snapshot as `from`; call `Diff(from, desired)`; write the new Migration with a fresh version from the clock; `removed` lists the file names. The no-change round trip then holds because the regenerated migration's snapshot equals the desired schema.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(migrate): rebase branch migrations onto the target snapshot"`.

## Task 28: CLI URL redaction and TLS warning

Covers: S-46 (database URLs only from environment or config env indirection; redacted in all CLI output and errors; TLS-disabled warning for remote hosts). Acceptance: TestCLIRedactsURL, TestCLITLSWarning.

Files:

- Create: `cmd/momo/migrate_redact.go`, `cmd/momo/migrate_redact_test.go`.

Interfaces (exact):

```go
// RedactURL returns the URL with the password replaced by "xxxxx"; it never returns the input when parsing fails (it returns "<unparseable URL>").
func RedactURL(raw string) string
// TLSWarning returns a non-empty warning when sslmode is disable (PostgreSQL) or the host is not localhost, 127.0.0.1, ::1, or a unix socket path.
func TLSWarning(raw string) string
// DatabaseURL reads the environment variable named by config (never a flag value); error text uses RedactURL.
func DatabaseURL(env func(string) string, varName string) (string, error)
```

Steps:

- [ ] **Step 1: Failing test**

`cmd/momo/migrate_redact_test.go`:

```go
package main

import (
    "testing"
	"strings"
)

func TestCLIRedactsURL(t *testing.T) {
    raw := "postgres://app:s3cret-pass@db.example.com:5432/app?sslmode=require"
    if got := RedactURL(raw); strings.Contains(got, "s3cret-pass") || !strings.Contains(got, "xxxxx") {
        t.Fatalf("password not redacted: %s", got)
    }
    _, err := DatabaseURL(func(string) string { return "" }, "MOMO_DATABASE_URL")
    if err == nil || !strings.Contains(err.Error(), "MOMO_DATABASE_URL") {
        t.Fatalf("missing variable must name the variable, got %v", err)
    }
}

func TestCLITLSWarning(t *testing.T) {
    w := TLSWarning("postgres://app:x@db.example.com/app?sslmode=disable")
    if !strings.Contains(w, "TLS") {
        t.Fatalf("want TLS warning for remote disabled TLS, got %q", w)
    }
    if TLSWarning("postgres://app:x@localhost/app?sslmode=disable") != "" {
        t.Fatal("local disabled TLS must not warn")
    }
}
```

Run: `go test ./cmd/momo/ -run 'TestCLIRedactsURL|TestCLITLSWarning' -count=1`. Expect FAIL with `undefined: RedactURL`.

- [ ] **Step 2: Implement** with `net/url`. `RedactURL` uses `u.User = url.UserPassword(u.User.Username(), "xxxxx")` and `u.String()`. Every error path in the CLI (Task 29) passes through RedactURL before printing; the test in Task 29 checks this.

- [ ] **Step 3: Run, expect PASS**, then commit `git commit -m "feat(cli): redact database URLs and warn on disabled TLS for remote hosts"`.

## Task 29: CLI wiring for every migrate command and exit codes

Covers: S-10 (every command in the Interfaces table with its contract), S-27 (`rebase --onto`), S-11 (`resolve`, `baseline`, `drift`, `rollback`, `new --custom`), S-28 (`lint`, lock_timeout flags), S-46, S-26 (generated imports and migration paths use forward slashes; plan 04 owns TestWindowsPaths for the CLI package that drives both gen and migrate). Acceptance: TestCLIContracts, TestOfflineCommandsNoDatabase, TestMigrateLint (exit code part), TestWindowsPaths.

Files:

- Create: `cmd/momo/migrate.go`, `cmd/momo/migrate_test.go`, `cmd/momo/paths_test.go`.
- Modify: `cmd/momo/main.go` (plan momo-02 owns `cmd/momo/main.go` for `generate`; this task adds one `case "migrate":` dispatch line and reports the change to the lead).

Interfaces (exact):

```go
// Subcommands and flags (every command accepts --config <path>, default momo.yaml, and --dir <path> overriding the config directory):
//   migrate diff <name> [--rename-table a=b]... [--rename-column t.a=b]... [--allow-destructive] [--stage]   offline for pure diffs; SQLite rebuilds require --url
//   migrate plan                    (needs DB)
//   migrate apply [--to N] [--force-unlock] [--lock-wait 30s]   (needs DB)
//   migrate status                  (needs DB)
//   migrate validate [--db]         (offline; --db needs DB)
//   migrate baseline --version N [--force]  (needs DB)
//   migrate drift                   (needs DB)
//   migrate rebase [--onto <dir-or-version>]  (offline; no git)
//   migrate lint [--db]             (offline; --db adds row counts and the >100000 rule)
//   migrate resolve --version N --applied|--rolled-back  (needs DB)
//   migrate rollback [--group]      (needs DB)
//   migrate new --custom <name>     (offline)
// Exit codes: 0 success; 1 findings (drift, validate or lint errors, destructive or staged refusal, status dirty or mismatch);
//             2 usage or configuration error; 3 database, lock or execution error.
// Output: plain text lines; every error passes through RedactURL.
// Offline commands never call DatabaseURL and never open a connection (TestOfflineCommandsNoDatabase enforces it by setting the environment variable to a value that would fail if used).
```

Steps:

- [ ] **Step 1: Failing contract tests**

`cmd/momo/migrate_test.go`:

```go
package main

import (
    "testing"
	"bytes"
	"path/filepath"
)

func TestCLIContracts(t *testing.T) {
    dir := t.TempDir()
    writeFile(t, filepath.Join(dir, "momo.yaml"), "migrate:\n  dir:\n    sqlite: "+dir+"/migrations/sqlite\n")
    var out, errOut bytes.Buffer
    code := runMigrate(&out, &errOut, []string{"--config", filepath.Join(dir, "momo.yaml"), "validate"}, func(string) string { return "" })
    if code != 0 {
        t.Fatalf("validate on an empty directory must exit 0, got %d: %s", code, errOut.String())
    }
    code = runMigrate(&out, &errOut, []string{"plan"}, func(string) string { return "" })
    if code != 2 {
        t.Fatalf("plan without a database URL is a configuration error (2), got %d", code)
    }
    code = runMigrate(&out, &errOut, []string{"nosuch"}, func(string) string { return "" })
    if code != 2 {
        t.Fatalf("unknown subcommand must exit 2, got %d", code)
    }
}

func TestOfflineCommandsNoDatabase(t *testing.T) {
    dir := t.TempDir()
    writeFile(t, filepath.Join(dir, "momo.yaml"), "migrate:\n  dir:\n    sqlite: "+dir+"/migrations/sqlite\n")
    poison := func(k string) string {
        if k == "MOMO_DATABASE_URL" {
            return "postgres://nobody:secret@127.0.0.1:1/none?sslmode=disable"
        }
        return ""
    }
    var out, errOut bytes.Buffer
    for _, args := range [][]string{{"validate"}, {"lint"}, {"rebase"}} {
        code := runMigrate(&out, &errOut, append([]string{"--config", filepath.Join(dir, "momo.yaml")}, args...), poison)
        if code == 3 {
            t.Fatalf("%v opened a connection (exit 3): %s", args, errOut.String())
        }
    }
}

func TestCLIExitOneOnDrift(t *testing.T) {
    // Full body is given in Step 3; this test is added to the same file.
}
```

Run: `go test ./cmd/momo/ -run 'TestCLIContracts|TestOfflineCommandsNoDatabase' -count=1`. Expect FAIL with `undefined: runMigrate`.

- [ ] **Step 2: Implement** `runMigrate(out, errOut io.Writer, args []string, env func(string) string) int` in `cmd/momo/migrate.go` with a `flag.FlagSet` per subcommand; the configuration comes from `config.Load(--config)` (plan momo-02) and `File.Migrate` maps to `migrate.Config` (empty `Schemas` stays empty so the engine resolves `current_schema()`); `DatabaseURL` only in commands marked "needs DB"; the dialect is chosen by a static switch on the configured dialect name (`postgres` gives `mpg.New()`, `sqlite` gives `mpg.New()`, anything else exits 2) and passed with `migrate.WithDialect`. The `--lock-wait` flag maps to `migrate.WithLockWait`. The engine's `Apply` maps `ErrLocked` to exit 3, `ErrDirty`/`ErrChecksumMismatch` to exit 3 with the file name, `ErrDestructive`/`ErrStagedRequired` on diff to exit 1.

- [ ] **Step 3: Add the drift contract test (replaces the empty function from Step 1)**

Replace the empty `TestCLIExitOneOnDrift` function from Step 1 with:

```go
func TestCLIExitOneOnDrift(t *testing.T) {
    // testdb.PostgresURL(t) is memoized per t: the stray table and the CLI use the same per-test schema
    // (search_path=t_xxx,public), so the CLI's default managed schema current_schema() is that schema.
    dsn := testdb.PostgresURL(t)
    sqlDB, err := sql.Open("pgx", dsn)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { sqlDB.Close() })
    if _, err := sqlDB.ExecContext(context.Background(), `CREATE TABLE stray (id INTEGER PRIMARY KEY)`); err != nil {
        t.Fatal(err)
    }
    dir := t.TempDir()
    writeFile(t, filepath.Join(dir, "momo.yaml"), "migrate:\n  dir:\n    postgres: "+dir+"/migrations/postgres\n  url_env:\n    postgres: MOMO_TEST_URL\n")
    mkdirAll(t, dir+"/migrations/postgres")
    var out, errOut bytes.Buffer
    code := runMigrate(&out, &errOut, []string{"--config", filepath.Join(dir, "momo.yaml"), "drift"}, func(k string) string {
        if k == "MOMO_TEST_URL" {
            return dsn
        }
        return ""
    })
    if code != 1 {
        t.Fatalf("drift with an unmanaged table must exit 1, got %d: %s", code, errOut.String())
    }
}
```

Test helpers `writeFile` and `mkdirAll` live in `cmd/momo/testhelpers_test.go`. `migrate_test.go` gains these imports for the drift test: `context`, `database/sql`, `testdb "github.com/azrtydxb/momo/internal/testdb"` and the blank import `_ "github.com/jackc/pgx/v5/stdlib"` (the `pgx` database/sql driver). The drift test needs PostgreSQL and is not build-tagged: `testdb` skips it when `MOMO_TEST_POSTGRES_URL` is unset and fails it under `MOMO_REQUIRE_INTEGRATION=1`. Run: `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test ./cmd/momo/ -run 'TestCLI|TestOffline' -count=1`; expect PASS. The config keys (`migrate.dir.<dialect>`, `migrate.url_env.<dialect>`) are the `MigrateConfig` fields defined in the Task 3 contract block; the CLI loads the file through `config.Load(path)` from plan momo-02 (package `gen/config`, returning `*config.File`, with the settings under `File.Migrate`) and has no YAML parser of its own.

- [ ] **Step 3b: Forward-slash path test (S-26, TestWindowsPaths)**

`cmd/momo/paths_test.go` (no build tag: a unit test that runs on the windows-latest CI job). The first subtest calls the plan-02 function `gen.ImportPath(modulePath, moduleDir, pkgDir string) (string, error)` (it returns `modulePath + "/" + filepath.ToSlash(rel)` where `rel` comes from `filepath.Rel(moduleDir, pkgDir)`, and an error when `pkgDir` is outside `moduleDir`). The second calls the plan-04 `migrate.ComputeSum`, which records `momo.sum` entries, and `(*migrate.Sum).Write`. Paths are built with `filepath.Join`, so on Windows they contain `\`; the assertions check what the plan-02 `gen.ImportPath` returns and what the plan-04 code records and writes (a migration file in the nested directory `archive/2026` must be recorded with forward slashes).

```go
package main

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/azrtydxb/momo/gen"
    "github.com/azrtydxb/momo/migrate"
)

// TestWindowsPaths asserts generated imports and migration paths use forward slashes.
func TestWindowsPaths(t *testing.T) {
    t.Run("generated import path", func(t *testing.T) {
        root := t.TempDir()
        // filepath.Join uses backslashes on Windows, so this genuinely tests the conversion to slashes.
        got, err := gen.ImportPath("example.com/app", root, filepath.Join(root, "internal", "models", "user"))
        if err != nil {
            t.Fatal(err)
        }
        if want := "example.com/app/internal/models/user"; got != want {
            t.Fatalf("ImportPath = %q, want %q", got, want)
        }
    })

    t.Run("migration and momo.sum paths", func(t *testing.T) {
        dir := t.TempDir()
        nested := filepath.Join(dir, "archive", "2026") // built with filepath.Join: backslashes on Windows
        if err := os.MkdirAll(nested, 0o755); err != nil {
            t.Fatal(err)
        }
        if err := os.WriteFile(filepath.Join(nested, "20260101000000_init.up.sql"), []byte("CREATE TABLE t (id int);\n"), 0o644); err != nil {
            t.Fatal(err)
        }
        sum, err := migrate.ComputeSum(os.DirFS(dir))
        if err != nil {
            t.Fatal(err)
        }
        if _, ok := sum.Files["archive/2026/20260101000000_init.up.sql"]; !ok {
            t.Fatalf("momo.sum entry must use a forward slash, got %v", sum.Files)
        }
        for name := range sum.Files {
            if strings.Contains(name, `\`) {
                t.Fatalf("momo.sum entry %q contains a backslash", name)
            }
        }
        var b strings.Builder
        if err := sum.Write(&b); err != nil {
            t.Fatal(err)
        }
        if strings.Contains(b.String(), `\`) {
            t.Fatalf("momo.sum text contains a backslash:\n%s", b.String())
        }
    })
}
```

Run on any host: `go test ./cmd/momo/ -run TestWindowsPaths -count=1 -v`; expect `ok` once plans 02 and 04 have landed (before then, `undefined: gen.ImportPath` or `undefined: migrate.ComputeSum`). Compile check for Windows from macOS or Linux: `GOOS=windows go vet ./cmd/momo/`; expect no output. The real check runs on the windows-latest job (`go test ./cmd/momo/ -run TestWindowsPaths -count=1`).

- [ ] **Step 4: Commit** `git commit -m "feat(cli): migrate subcommands with documented exit codes"`.

## Task 30: Format-version tests across persisted artifacts

Covers: S-22 (snapshot format_version, momo.sum header, history metadata row; a legacy format 0 is read; a newer format is refused with an actionable error). Acceptance: TestFormatVersions (format 0 read path and future-version refusal), TestHistoryFormat0Upgrade.

Format 0 is the pre-release layout: a snapshot with no `format_version` field, a `momo.sum` with no header line, and a history table with no metadata row. Format 0 is the only legacy format that exists, so it is committed as a hand-written fixture. Tasks 5 and 6 gain this requirement: `ParseSum` treats a missing header as format 0, and `ReadSnapshot` treats a missing `format_version` as format 0; both read it and both refuse any value above the current constant.

Files:

- Create: `migrate/testdata/format0/20260101000000_init.up.sql`, `migrate/testdata/format0/20260101000000_init.snapshot.json`, `migrate/testdata/format0/momo.sum`, `migrate/format_versions_test.go` (package `migrate`), `migrate/format_history_test.go` (package `migrate_test`).

Fixture contents (hand-written, committed verbatim):

`migrate/testdata/format0/20260101000000_init.up.sql`:

```sql
CREATE TABLE t (id INTEGER PRIMARY KEY);
```

`migrate/testdata/format0/20260101000000_init.snapshot.json`:

```json
{
  "version": "20260101000000",
  "parent": "",
  "schema": {
    "dialect": "sqlite",
    "tables": [
      {
        "name": "t",
        "columns": [{ "name": "id", "type": "INTEGER" }],
        "primary_key": ["id"]
      }
    ]
  }
}
```

`migrate/testdata/format0/momo.sum`: the output of `ComputeSum` over the two files above, written once by a throwaway test that calls `ComputeSum(os.DirFS("testdata/format0"))` and `Write`s the result, then pasted verbatim with the header line removed. The committed file must satisfy `VerifySum`.

- [ ] **Step 1: Failing format test**

`migrate/format_versions_test.go`:

```go
package migrate

import (
    "testing"

    "github.com/azrtydxb/momo/schema"
	"os"
	"errors"
	"strings"
	"bytes"
	"io/fs"
)

func TestFormatVersions(t *testing.T) {
    fsys := os.DirFS("testdata/format0")
    sumBytes, err := fs.ReadFile(fsys, "momo.sum")
    if err != nil {
        t.Fatal(err)
    }
    sum, err := ParseSum(bytes.NewReader(sumBytes))
    if err != nil {
        t.Fatalf("format 0 momo.sum must be read: %v", err)
    }
    if bad := VerifySum(fsys, sum); len(bad) != 0 {
        t.Fatalf("format 0 momo.sum does not verify: %v", bad)
    }
    snapBytes, err := fs.ReadFile(fsys, "20260101000000_init.snapshot.json")
    if err != nil {
        t.Fatal(err)
    }
    snap, err := ReadSnapshot(bytes.NewReader(snapBytes))
    if err != nil {
        t.Fatalf("format 0 snapshot must be read: %v", err)
    }
    if snap.Schema == nil || len(snap.Schema.Tables) != 1 {
        t.Fatalf("legacy schema not read: %+v", snap)
    }
    _, err = ReadSnapshot(strings.NewReader(`{"format_version":9,"version":"1","parent":"","schema":{}}`))
    if !errors.Is(err, schema.ErrNewerFormat) || !strings.Contains(err.Error(), "upgrade momo") {
        t.Fatalf("future snapshot must be refused, got %v", err)
    }
    _, err = ParseSum(strings.NewReader("# momo.sum format=9\n"))
    if !errors.Is(err, schema.ErrNewerFormat) {
        t.Fatalf("future momo.sum must be refused, got %v", err)
    }
}
```

Run: `go test ./migrate/ -run TestFormatVersions -count=1`. Expect FAIL with `format 0 momo.sum must be read` until Tasks 5 and 6 accept format 0.

- [ ] **Step 2: Accept format 0 in ParseSum and ReadSnapshot**

Implement the format 0 branches described above, then run the test: expect PASS.

- [ ] **Step 3: Failing history upgrade test**

`migrate/format_history_test.go`:

```go
package migrate_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHistoryFormat0Upgrade(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    c := pinned(t, db)
    // Format 0 history: the table exists with one applied row and no metadata row.
    mustExec(t, c, `CREATE TABLE momo_migrations (version TEXT PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, status TEXT NOT NULL, statements_applied INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '', applied_at TEXT NOT NULL, execution_ms INTEGER NOT NULL DEFAULT 0)`)
    mustExec(t, c, `INSERT INTO momo_migrations (version, name, checksum, status, applied_at) VALUES ('20260101000000', 'init', 'abc', 'applied', '2026-01-01T00:00:00Z')`)
    if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
        t.Fatalf("format 0 history must be read and upgraded: %v", err)
    }
    rows, err := migrate.ReadHistory(ctx, c)
    if err != nil {
        t.Fatal(err)
    }
    var applied, meta bool
    for _, r := range rows {
        if r.Version == "20260101000000" && r.Status == migrate.StatusApplied {
            applied = true
        }
        if r.Version == migrate.MetaVersion && r.Checksum == "1" {
            meta = true
        }
    }
    if !applied || !meta {
        t.Fatalf("upgrade must keep the applied row and add the metadata row: %+v", rows)
    }
}
```

Run: `MOMO_TEST_...` not needed; `go test ./migrate/ -run TestHistoryFormat0Upgrade -count=1`. Expect FAIL until `EnsureHistory` adds missing columns for format 0 (the `apply_id` and `kind` columns are added with `ALTER TABLE ... ADD COLUMN` defaults) and the metadata row. Implement and expect PASS.

- [ ] **Step 4: Commit**

`git commit -m "test(migrate): format 0 fixtures are read and newer formats are refused"`

## Task 31: Docs for migration safety and recovery

Covers: S-21 (migration safety and recovery guide, indexed in docs/README.md). Acceptance: docs file exists and documents each failure mode from the spec.

Files:

- Create: `docs/migrations.md`.
- Modify: `docs/README.md` (add one index line; the file is owned by plan momo-01 for the initial index, this task appends one line).

Steps:

- [ ] **Step 1: Failing check**

Run: `test -f docs/migrations.md && grep -q 'momo migrate resolve' docs/migrations.md`. Expect FAIL (exit 1) because the file does not exist.

- [ ] **Step 2: Write the guide** with sections: Files and formats (`momo.sum`, snapshots, history table, the format versions), Commands and exit codes (copy the Task 29 table), Transactional and non-transactional migrations, Recovering a dirty migration (`momo migrate status`, repair manually, `momo migrate resolve --version N --applied|--rolled-back`), Lock behavior (advisory lock on PostgreSQL, `momo_migration_lock` on SQLite, `--force-unlock` only after confirming no process holds it), Destructive changes and staged NOT NULL, Renames (explicit flags only), Baselining an existing database, Drift and unmanaged objects, Branch conflicts and rebase, Application roles (migration DDL role versus application DML role, S-47), SQLite table rebuilds, CONCURRENTLY indexes. Custom SQL migrations copy the parent snapshot: model-affecting custom SQL must be followed by `momo migrate diff`, and drift reports any object the snapshot does not describe. Views are compared by identity in drift; PostgreSQL rewrites view text. A changed view body made outside migrations is not detected as drift.

- [ ] **Step 3: Run the check, expect PASS**, then commit `git commit -m "docs: migration safety and recovery guide"`.

## Task 32: Views and materialized views as read-only models

Covers: S-37 (views and materialized views created, replaced and dropped in dependency order by migrations; queried as read-only models; REFRESH helper; mutation builders on them do not compile). Acceptance: TestViewsAsModels, TestViewInsertDoesNotCompile.

Contract (plan momo-02 generates the models; this task consumes them):

- Model marker `momo.View` with tag `momo:"view:<name>[,materialized]"`. The generator emits no `MomoWritable()` method for a view, so the view type does not satisfy `WritableModeler`.
- `type WritableModeler[T any] interface { Modeler[T]; MomoWritable() }`, generated only for `momo.Model` tables.
- `func Insert[T any, PT WritableModeler[T]](q Querier) InsertQuery[T]`; `Update` and `Delete` take the same constraint. `Select` keeps `PT Modeler[T]`.
- `func RefreshMaterializedView(ctx context.Context, q Querier, name string, concurrently bool) error` (root package, file `views.go`): renders `REFRESH MATERIALIZED VIEW [CONCURRENTLY] "schema"."name"` (an unqualified name resolves through the search_path; a `schema.name` argument is split and quoted) through `q`; PostgreSQL only, other dialects return ErrUnsupported.
- Consumed from plan momo-02 (MomoConfig companion, read statically; plan momo-02 maps them into `schema.Schema`): `c.View(name, query string)`, `c.MaterializedView(name, query string)`, `c.Function(name string, def FunctionDef)`, `c.Trigger(name string, def TriggerDef)`, `c.Policy(name string, def PolicyDef)`, `c.EnableRLS(force bool)`, with `FunctionDef{Args, Returns, Language, Volatility, Body string}`, `TriggerDef{Table, Timing string; Events []string; ForEachRow bool; When, Function string; Args []string}`, `PolicyDef{Table, Command string; Permissive bool; Roles []string; Using, Check string}`.
- Drift rule for views (decision, documented in docs/migrations.md): drift compares views by identity, materialized flag and dependencies. PostgreSQL rewrites view text, so body changes made outside migrations are not detected by drift; migration diffs compare the source text.

Files:

- Create: `internal/compat/viewmodels/viewmodels.go` (models plus the generated `momo_gen.go`), `migrate/postgres/views_test.go`, `testdata/compilefail/view_insert/main.go`, `views_compile_test.go` (root package, package `momo_test`).
- Create: `views.go` (root package; plan momo-01 owns the root file list, report the addition to the lead).

Interfaces (exact):

```go
// views.go, package momo
func RefreshMaterializedView(ctx context.Context, q Querier, name string, concurrently bool) error
```

Steps:

- [ ] **Step 1: The view models and the compile-fail fixture**

`internal/compat/viewmodels/viewmodels.go`:

```go
package viewmodels

import "github.com/azrtydxb/momo"

// OpenOrder reads the open_orders view. It has no write methods.
type OpenOrder struct {
    momo.View `momo:"view:open_orders"`
    ID        int64   `momo:"column:id"`
    Total     float64 `momo:"column:total"`
}

func (*OpenOrder) MomoConfig(c *momo.Config) {
    c.View("open_orders", "SELECT id, total FROM orders WHERE status = 'open'")
}

// OrderTotal reads the order_totals materialized view.
type OrderTotal struct {
    momo.View `momo:"view:order_totals,materialized"`
    Status    string  `momo:"column:status"`
    Total     float64 `momo:"column:total"`
}

func (*OrderTotal) MomoConfig(c *momo.Config) {
    c.MaterializedView("order_totals", "SELECT status, sum(total) AS total FROM orders GROUP BY status")
}
```

Run `momo generate ./internal/compat/viewmodels` and commit the generated `momo_gen.go`; the generated code must contain `MomoMeta()` for both types and no `MomoWritable()`.

`testdata/compilefail/view_insert/main.go`:

```go
//go:build compilefail

package main

import (
    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/compat/viewmodels"
)

func main() {
    var q momo.Querier
    _ = momo.Insert[viewmodels.OpenOrder, *viewmodels.OpenOrder](q)
}
```

- [ ] **Step 2: Failing compile-fail test**

`views_compile_test.go`:

```go
package momo_test

import (
    "testing"
	"os"
	"strings"
	"os/exec"
)

func TestViewInsertDoesNotCompile(t *testing.T) {
    cmd := exec.Command("go", "build", "-tags", "compilefail", "-o", os.DevNull, "./testdata/compilefail/view_insert")
    out, err := cmd.CombinedOutput()
    if err == nil {
        t.Fatal("insert on a view model must not compile")
    }
    if !strings.Contains(string(out), "does not implement WritableModeler") {
        t.Fatalf("wrong compile error: %s", out)
    }
}
```

Run: `go test . -run TestViewInsertDoesNotCompile -count=1`. Expect FAIL with `insert on a view model must not compile` before `Insert` has the `WritableModeler` constraint.

- [ ] **Step 3: Implement the constraint and RefreshMaterializedView**

Change `Insert`, `Update` and `Delete` to take `PT WritableModeler[T]`; generated code for `momo.Model` tables gains `MomoWritable()`. Implement `RefreshMaterializedView` as described in Interfaces. Run the compile-fail test: expect PASS.

- [ ] **Step 4: Failing end-to-end test**

`migrate/postgres/views_test.go`:

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
    "github.com/azrtydxb/momo/internal/compat/viewmodels"
    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"strings"
	"time"
)

func at(ts string) migrate.Option {
    tm, err := time.Parse("20060102150405", ts)
    if err != nil {
        panic(err) // test-only constant input
    }
    return migrate.WithClock(func() time.Time { return tm })
}

func upFile(files map[string][]byte) string {
    for name := range files {
        if strings.HasSuffix(name, ".up.sql") {
            return name
        }
    }
    return ""
}

func TestViewsAsModels(t *testing.T) {
    db := testopen.Postgres(t)
    ctx := context.Background()
    // Depends entries are matched as the inspector reports them (Task 12): a bare name means the current schema,
    // which is the per-test schema, so no schema prefix is needed (any other namespace would be written "schema.name").
    base := &schema.Schema{Dialect: "postgres",
        Tables: []schema.Table{{Name: "orders", Columns: []schema.Column{
            {Name: "id", Type: "bigint"}, {Name: "status", Type: "text"}, {Name: "total", Type: "numeric"},
        }, PrimaryKey: []string{"id"}}},
        Views: []schema.View{
            {Name: "open_orders", Query: "SELECT id, total FROM orders WHERE status = 'open'", Depends: []string{"orders"}},
            {Name: "order_totals", Materialized: true, Query: "SELECT status, sum(total) AS total FROM orders GROUP BY status", Depends: []string{"orders"}},
        },
    }
    e1 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS(), at("20260101000000"))
    mig, err := e1.Diff(ctx, base, "views", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if mig == nil {
        t.Fatal("expected an initial migration")
    }
    e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files), at("20260102000000"))
    if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }
    mustExec(t, db.Driver(), `INSERT INTO orders (id, status, total) VALUES (1, 'open', 10), (2, 'closed', 5)`)

    // Queried as read-only models.
    open, err := momo.Select[viewmodels.OpenOrder, *viewmodels.OpenOrder](db).All(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if len(open) != 1 || open[0].ID != 1 {
        t.Fatalf("open_orders must return order 1, got %+v", open)
    }

    // Drift is clean for the applied views.
    rep, err := e2.Drift(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if !rep.Clean() {
        t.Fatalf("applied views must not drift: %+v", rep)
    }

    // Replace a view: the migration drops the view before recreating it.
    changed := *base
    changed.Views = []schema.View{
        {Name: "open_orders", Query: "SELECT id, total FROM orders WHERE status = 'open' AND total > 0", Depends: []string{"orders"}},
        base.Views[1],
    }
    mig2, err := e2.Diff(ctx, &changed, "replace_open", migrate.DiffOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if mig2 == nil {
        t.Fatal("changed view must produce a migration")
    }
    up := string(mig2.Files[upFile(mig2.Files)])
    if strings.Index(up, "DROP VIEW") < 0 || strings.Index(up, "DROP VIEW") > strings.Index(up, "CREATE VIEW") {
        t.Fatalf("view must be dropped before it is created:\n%s", up)
    }
    e3 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(migratetest.MergeMaps(mig.Files, mig2.Files)), at("20260103000000"))
    if _, err := e3.Apply(ctx, migrate.ApplyOptions{}); err != nil {
        t.Fatal(err)
    }

    // RefreshMaterializedView: the matview is stale until refreshed.
    var count int
    mustQueryRow(t, pinned(t, db), `SELECT count(*) FROM order_totals`, &count)
    if count != 2 {
        t.Fatalf("order_totals must have two groups, got %d", count)
    }
    mustExec(t, db.Driver(), `INSERT INTO orders (id, status, total) VALUES (3, 'open', 7)`)
    if err := momo.RefreshMaterializedView(ctx, db, "order_totals", false); err != nil {
        t.Fatal(err)
    }
    if err := momo.RefreshMaterializedView(ctx, db, "order_totals", true); err == nil {
        t.Fatal("CONCURRENTLY without a unique index must fail")
    }
    mustExec(t, db.Driver(), `CREATE UNIQUE INDEX order_totals_status ON order_totals (status)`) // test-only index, outside migrations
    if err := momo.RefreshMaterializedView(ctx, db, "order_totals", true); err != nil {
        t.Fatalf("CONCURRENTLY with a unique index must succeed: %v", err)
    }
}
```

Helpers `mustExec` and `mustQueryRow` and `pinned` are in `migrate/postgres/helpers_test.go` (Task 3). Run: `MOMO_TEST_POSTGRES_URL=... go test ./migrate/postgres/ -run TestViewsAsModels -count=1`. Expect FAIL with `undefined: migratetest.MergeMaps` or `momo.RefreshMaterializedView` until Step 3 and the earlier tasks are in place.

- [ ] **Step 5: Implement the remaining pieces and run**

Run `MOMO_TEST_POSTGRES_URL=... go test ./migrate/postgres/ -run TestViewsAsModels -count=1`. Expect PASS.

- [ ] **Step 6: Commit**

`git commit -m "feat: read-only view models, writable constraint for mutations, RefreshMaterializedView"`

## Task 33: SQLite inspection

Covers: S-13 (inspection feeding drift and round-trip), S-39 (triggers on SQLite). Acceptance: TestSQLiteInspectRoundTrip.

Files:

- Create: `migrate/sqlite/inspect.go`, `migrate/sqlite/inspect_test.go`, `migrate/sqlite/dialect.go`.

Interfaces (exact):

```go
package sqlite
type Dialect struct{}
func New() Dialect                   // implements migrate.Dialect
func (Dialect) Inspect(ctx context.Context, c momo.PinnedConn, opts migrate.InspectOptions) (*schema.Schema, error)
```

Catalog queries (exact SQL):

- Tables: `SELECT name, sql FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name <> 'momo_migrations' AND name <> 'momo_migration_lock' ORDER BY name`. `WITHOUT ROWID` detected by `sql LIKE '%WITHOUT ROWID%'` after the closing parenthesis.
- Columns: `PRAGMA table_xinfo(?)` (cid, name, type, notnull, dflt_value, pk, hidden). `hidden` 2 or 3 means generated; the generated expression is parsed from the `sql` text between `GENERATED ALWAYS AS (` and the matching `)`. Identity: an `INTEGER` column that is the sole primary key of a rowid table is reported as `Identity: "by_default"`.
- Enum-constrained TEXT columns: parse `CHECK ("col" IN ('a','b'))` from the table `sql`, set `Column.Enum` to a synthetic name `<table>_<column>` and fill `schema.Enums` with it.
- Primary key: `pk` column ordinal from table_xinfo.
- Uniques and indexes: `PRAGMA index_list(?)` (seq, name, unique, origin, partial). origin `pk` is skipped, origin `u` becomes a Unique with columns from `index_xinfo`, origin `c` becomes an Index. Parts: `PRAGMA index_xinfo(?)` rows where key=1: cid >= 0 is a column, cid = -2 is an expression whose text is taken from the index `sql` in `sqlite_schema`. desc and coll come from the row. Partial predicate: text after `WHERE` in the index `sql`.
- FKs: `PRAGMA foreign_key_list(?)` (id, seq, table, from, to, on_update, on_delete). Group by id, order by seq. `NO ACTION` is stored as empty string.
- Checks: `CHECK (...)` clauses at table level parsed from the table `sql` by the same scanner as Task 4 (reuse `migrate.SplitStatements`-style scanning; implement a helper `topLevelChecks(sql string) []string`).
- Views: `SELECT name, sql FROM sqlite_schema WHERE type='view'`; Query is the text after the `AS` keyword that follows the view name.
- Triggers: `SELECT tbl_name, name, sql FROM sqlite_schema WHERE type='trigger'`; `Definition` is the stored sql, `Table` is tbl_name.
- Unsupported on SQLite and reported as an error when present in the desired schema, never inspected: extensions, enums as native types, functions, policies, partitions, RLS, schemas. Inspection returns none of these.

Steps:

- [ ] **Step 1: Failing round-trip test**

`migrate/sqlite/inspect_test.go`:

```go
package sqlite_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/dialect"
    "github.com/azrtydxb/momo/driver/sqlitedriver"
    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestSQLiteInspectRoundTrip(t *testing.T) {
    db := testopen.SQLite(t)
    ctx := context.Background()
    if _, err := db.Driver().Exec(ctx, `CREATE TABLE docs (id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT '', status TEXT CHECK ("status" IN ('a','b')), tenant INTEGER REFERENCES tenants(id) ON DELETE CASCADE)`, nil); err != nil {
        t.Fatal(err)
    }
    if _, err := db.Driver().Exec(ctx, `CREATE INDEX docs_title ON docs (lower(title)) WHERE status = 'a'`, nil); err != nil {
        t.Fatal(err)
    }
    c := pinned(t, db)
    got, err := msqlite.New().Inspect(ctx, c, migrate.InspectOptions{})
    if err != nil {
        t.Fatal(err)
    }
    tbl, ok := got.Table("docs")
    if !ok {
        t.Fatal("docs not inspected")
    }
    col, ok := tbl.Column("title")
    if !ok {
    	t.Fatalf("column %q missing", "title")
    }
    if col.Nullable || col.Default != "''" {
        t.Fatalf("title: %+v", col)
    }
    if len(tbl.ForeignKeys) != 1 || tbl.ForeignKeys[0].OnDelete != "CASCADE" {
        t.Fatalf("fk actions lost: %+v", tbl.ForeignKeys)
    }
    if len(tbl.Indexes) != 1 || tbl.Indexes[0].Where == "" || tbl.Indexes[0].Parts[0].Expr == "" {
        t.Fatalf("expression or partial index lost: %+v", tbl.Indexes)
    }
    if !schema.Equal(got, schema.Normalized(got)) {
        t.Fatal("inspection output must be stable under normalization")
    }
}
```

`pinned(t, db)` is a helper in the test that calls `db.Driver().(momo.Pinner).Pin(ctx)` (momo-03 Task 2 Pinner) and registers Release on cleanup. Run: `go test ./migrate/sqlite/ -run TestSQLiteInspectRoundTrip -count=1`. Expect FAIL with `undefined: msqlite.New`.

- [ ] **Step 2: Implement** the queries above in `inspect.go`, the `Dialect` value in `dialect.go` (method set: Inspect, Render, HistoryDDL, Setup, Lock; Render and Lock are implemented in Tasks 34 and 36, returning `migrate.ErrUnsupported` stubs until then so the package compiles).

- [ ] **Step 3: Run, expect PASS** with the same command.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate/sqlite): inspection of tables, indexes, FKs, checks and objects"`

## Task 34: SQLite rendering: native ALTER versus rebuild decision

Covers: S-13 (version-aware ALTER TABLE, rebuild when required). Acceptance: TestSQLiteAlterDecision.

Files:

- Create: `migrate/sqlite/render.go`, `migrate/sqlite/render_test.go`.

Interfaces (exact):

```go
func (Dialect) Render(plan []migrate.Change) ([]migrate.Statement, error)
// Native (no rebuild): rename_table, rename_column, create_index, drop_index, create_trigger, drop_trigger, create_view, drop_view,
//   add_column when ALL hold: not a PRIMARY KEY or UNIQUE; not NOT NULL unless the default is non-NULL and not CURRENT_TIME/CURRENT_DATE/CURRENT_TIMESTAMP and not parenthesized; no generated STORED; if a REFERENCES clause exists, default is NULL.
//   drop_column when the column is not PK, UNIQUE, indexed, named in a partial index WHERE, in a CHECK, in an FK, in a generated expression, or in a trigger or view.
// Rebuild (Change.Rebuild = true, handled by rebuild.go): alter_column, add_primary_key, drop_primary_key, add_unique, drop_unique, add_check, drop_check, add_fk, drop_fk, drop_column when a native drop is not allowed, add_column when a native add is not allowed.
// ErrUnsupported (returned as a wrapped error naming the kind): create_schema, create_extension, create_enum, alter_enum_add_value, drop_enum, create_policy, drop_policy, set_rls, create_function, replace_function, drop_function.
```

Steps:

- [ ] **Step 1: Failing test**

`migrate/sqlite/render_test.go`:

```go
package sqlite

import (
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
)

func TestSQLiteAlterDecision(t *testing.T) {
    nullDefault := &schema.Column{Name: "n", Type: "TEXT", Nullable: true}
    notNullNoDefault := &schema.Column{Name: "n", Type: "TEXT"}
    notNullDefault := &schema.Column{Name: "n", Type: "TEXT", Default: "''"}
    cases := []struct {
        name string
        ch   migrate.Change
        want bool // true means Rebuild
    }{
        {"nullable add is native", migrate.Change{Kind: "add_column", Table: "t", After: nullDefault}, false},
        {"not null without default must rebuild", migrate.Change{Kind: "add_column", Table: "t", After: notNullNoDefault}, true},
        {"not null with default is native", migrate.Change{Kind: "add_column", Table: "t", After: notNullDefault}, false},
        {"alter column rebuilds", migrate.Change{Kind: "alter_column", Table: "t", Before: nullDefault, After: notNullDefault}, true},
        {"add fk rebuilds", migrate.Change{Kind: "add_fk", Table: "t"}, true},
        {"rename column is native", migrate.Change{Kind: "rename_column", Table: "t", Name: "b"}, false},
    }
    for _, c := range cases {
        stmts, err := Dialect{}.Render([]migrate.Change{c.ch})
        if err != nil {
            t.Fatalf("%s: %v", c.name, err)
        }
        if got := len(stmts) > 0 && stmts[0].Hint == "rebuild"; got != c.want {
            t.Errorf("%s: rebuild=%v want %v", c.name, got, c.want)
        }
    }
}

func TestSQLiteUnsupportedKindsError(t *testing.T) {
    _, err := Dialect{}.Render([]migrate.Change{{Kind: "create_enum", Name: "e"}})
    if err == nil {
        t.Fatal("create_enum must not be silently dropped on SQLite")
    }
}
```

Run: `go test ./migrate/sqlite/ -run 'TestSQLiteAlterDecision|TestSQLiteUnsupported' -count=1`. Expect FAIL with `Dialect{}.Render undefined` or its stub returning no statements.

- [ ] **Step 2: Implement** `Render`. For a native change, emit exactly one `Statement{SQL: ...}` where SQL uses quoted identifiers via `dialect.For(dialect.SQLite).QuoteIdent`. For rebuild-class changes, emit one `Statement{Hint: "rebuild", SQL: ""}` marker per table so the runner (Task 35) executes the rebuild procedure for that table with the full desired table definition from `Change.After`. Group multiple rebuild changes on the same table into one marker. Drop-column native eligibility is decided with the full set of changes for the table, so `Render` takes the whole plan.

- [ ] **Step 3: Run, expect PASS** with `go test ./migrate/sqlite/ -run 'TestSQLiteAlterDecision|TestSQLiteUnsupported' -count=1`.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate/sqlite): render native ALTER where allowed, mark rebuilds"`

## Task 35: SQLite 12-step rebuild

Covers: S-13 (12-step rebuild preserving indexes, constraints, triggers, views and data; foreign-key enforcement kept; foreign_key_check). Acceptance: TestSQLiteRebuild, TestSQLiteRebuildFKStillEnforced, TestSQLiteRebuildOrphanRollsBack, TestSQLiteRebuildBlocked.

Basis (verified 2026-11-04 against https://www.sqlite.org/lang_altertable.html): DROP TABLE does not drop views, so a view that names the table must be dropped before the DROP or the recreate fails with "view already exists"; triggers on the table are removed by DROP TABLE; since SQLite 3.26, ALTER TABLE RENAME rewrites references in triggers and views and fails when one of them names a missing table, so every view and every trigger that mentions the table must be dropped first and recreated afterwards. Triggers on other tables whose body names the table are included.

Files:

- Create: `migrate/sqlite/rebuild.go`, `migrate/sqlite/rebuild_test.go`, `migrate/sqlite/helpers_test.go`.

Interfaces (exact):

```go
type Dependent struct{ Type, Name, Table, SQL string } // Type "view" or "trigger"; Table is the trigger's table (tbl_name)
// Capture reads, in rowid order, every view and every trigger whose SQL mentions table (case-insensitive whole-word match),
// plus every trigger on table. Over-inclusion is safe: recreating an unaffected object verbatim does not change behaviour.
func Capture(ctx context.Context, c momo.PinnedConn, table string) ([]Dependent, error)
// Rebuild = Capture + RebuildStatements, with the old column list read from PRAGMA table_xinfo.
// Precondition: the caller ran Setup(ctx, c, SessionOptions{ForeignKeysOff: true}) outside any transaction.
func Rebuild(ctx context.Context, c momo.PinnedConn, table string, desired schema.Table, desiredIndexes []schema.Index) ([]migrate.Statement, error)
// RebuildStatements is pure (no I/O).
func RebuildStatements(table string, oldColumns []string, desired schema.Table, desiredIndexes []schema.Index, deps []Dependent) []migrate.Statement
```

Statement hints consumed by the runner (plan momo-04 Task 15):

- `Hint: "fk-check"` on `PRAGMA foreign_key_check`: the runner executes it inside the transaction and fails when it returns any row; the error names the first violation and the transaction is rolled back.
- `Hint: "dependent:<type>:<name>"` on each recreated view or trigger: a failure wraps `ErrRebuildBlocked` and names the object.

Statement order (normative):

1. For each dependent in reverse capture order: `DROP VIEW IF EXISTS "<name>"` or `DROP TRIGGER IF EXISTS "<name>"`.
2. `CREATE TABLE "new_<table>" (...)` from the desired table, rendered by the Task 34 column and constraint renderer.
3. `INSERT INTO "new_<table>" (<common>) SELECT <common> FROM "<table>"`, where common is the desired columns that exist in oldColumns, in desired order.
4. `DROP TABLE "<table>"`.
5. `ALTER TABLE "new_<table>" RENAME TO "<table>"`.
6. `CREATE [UNIQUE] INDEX` for each desired index.
7. Each dependent's saved SQL verbatim, in capture order (triggers before views is not required; the capture order is rowid order).
8. `PRAGMA foreign_key_check` with Hint `fk-check`.

Steps:

- [ ] **Step 1: Write the shared helpers and the failing rebuild test**

`migrate/sqlite/helpers_test.go`:

```go
package sqlite_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

type execer interface {
    Exec(ctx context.Context, sql string, args []any) (int64, error)
}

func mustExec(t *testing.T, x execer, sql string) {
    t.Helper()
    if _, err := x.Exec(context.Background(), sql, nil); err != nil {
        t.Fatalf("exec %q: %v", sql, err)
    }
}

func pinned(t *testing.T, db *momo.DB) momo.PinnedConn {
    t.Helper()
    pn, ok := db.Driver().(momo.Pinner)
    if !ok {
        t.Fatal("driver does not implement momo.Pinner")
    }
    c, err := pn.Pin(context.Background())
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = c.Release() }) // Cleanup has no error channel; Release failures do not affect the assertions
    return c
}

// scanOne runs a query on c, requires one row and scans it into dest.
func scanOne(t *testing.T, c momo.PinnedConn, sql string, dest ...any) {
    t.Helper()
    rows, err := c.Query(context.Background(), sql, nil)
    if err != nil {
        t.Fatalf("query %q: %v", sql, err)
    }
    defer rows.Close()
    if !rows.Next() {
        if err := rows.Err(); err != nil {
            t.Fatal(err)
        }
        t.Fatalf("no rows for %q", sql)
    }
    if err := rows.Scan(dest...); err != nil {
        t.Fatal(err)
    }
}

// newRebuildFixture returns a pinned connection with foreign_keys ON, a parent table items with an index,
// a view, a trigger on items, a child table links with an FK to items and a trigger on links that updates items,
// and three parent rows and two child rows.
func newRebuildFixture(t *testing.T) momo.PinnedConn {
    t.Helper()
    db := testopen.SQLite(t)
    c := pinned(t, db)
    mustExec(t, c, `PRAGMA foreign_keys = ON`)
    mustExec(t, c, `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, qty INTEGER)`)
    mustExec(t, c, `CREATE INDEX items_name ON items(name)`)
    mustExec(t, c, `CREATE TRIGGER items_audit AFTER UPDATE ON items BEGIN SELECT 1; END`)
    mustExec(t, c, `CREATE VIEW items_v AS SELECT id, name FROM items`)
    mustExec(t, c, `CREATE TABLE links (id INTEGER PRIMARY KEY, item_id INTEGER NOT NULL REFERENCES items(id))`)
    mustExec(t, c, `CREATE TRIGGER links_touch AFTER INSERT ON links BEGIN UPDATE items SET qty = qty WHERE id = NEW.item_id; END`)
    mustExec(t, c, `INSERT INTO items (name, qty) VALUES ('a', 1), ('b', 2), ('c', 3)`)
    mustExec(t, c, `INSERT INTO links (item_id) VALUES (1), (2)`)
    return c
}
```

Remove the `testdb` import if `newRebuildFixture` is moved; the helper file keeps it because the fixture opens the database.

`migrate/sqlite/rebuild_test.go` (first test; the other three follow in Step 3):

```go
package sqlite_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
    "github.com/azrtydxb/momo/schema"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
)

var itemsDesired = schema.Table{Name: "items", Columns: []schema.Column{
    {Name: "id", Type: "INTEGER"}, {Name: "name", Type: "TEXT"},
    {Name: "qty", Type: "INTEGER", Default: "0"},
}, PrimaryKey: []string{"id"}}

func TestSQLiteRebuild(t *testing.T) {
    ctx := context.Background()
    c := newRebuildFixture(t)
    reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
    if err != nil {
        t.Fatal(err)
    }
    stmts, err := msqlite.Rebuild(ctx, c, "items", itemsDesired, []schema.Index{{Name: "items_name", Parts: []schema.IndexPart{{Column: "name"}}}})
    if err != nil {
        t.Fatal(err)
    }
    if err := migrate.RunStatements(ctx, c, stmts); err != nil {
        t.Fatal(err)
    }
    if err := reset(); err != nil {
        t.Fatal(err)
    }
    var n int
    scanOne(t, c, `SELECT count(*) FROM items`, &n)
    if n != 3 {
        t.Fatalf("rows lost: %d", n)
    }
    for _, want := range [][2]string{{"index", "items_name"}, {"trigger", "items_audit"}, {"trigger", "links_touch"}, {"view", "items_v"}} {
        var count int
        scanOne(t, c, `SELECT count(*) FROM sqlite_schema WHERE type='`+want[0]+`' AND name='`+want[1]+`'`, &count)
        if count != 1 {
            t.Fatalf("%s %s missing after rebuild", want[0], want[1])
        }
    }
    var fk int
    scanOne(t, c, `PRAGMA foreign_keys`, &fk)
    if fk != 1 {
        t.Fatalf("foreign_keys must be restored to ON, got %d", fk)
    }
}
```

Run: `go test ./migrate/sqlite/ -run TestSQLiteRebuild$ -count=1`. Expect FAIL with `undefined: msqlite.Rebuild`.

- [ ] **Step 2: Implement Capture, Rebuild and RebuildStatements**

`Capture` runs `SELECT type, name, tbl_name, sql FROM sqlite_schema WHERE type IN ('view','trigger') AND sql IS NOT NULL ORDER BY rowid`, then keeps a row when its type is view and its SQL matches the table name as a whole word (case-insensitive regexp `(?i)\b<quoted table>\b`), or when its type is trigger and tbl_name equals the table, or its SQL matches. `Rebuild` reads the old column names with `SELECT name FROM pragma_table_xinfo(?)` through `Capture` and calls `RebuildStatements`. `RebuildStatements` emits the eight statements of the normative order. Run the test again: expect PASS.

- [ ] **Step 3: Add the three remaining tests**

Append to `migrate/sqlite/rebuild_test.go`:

```go
func TestSQLiteRebuildFKStillEnforced(t *testing.T) {
    ctx := context.Background()
    c := newRebuildFixture(t)
    reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
    if err != nil {
        t.Fatal(err)
    }
    stmts, err := msqlite.Rebuild(ctx, c, "items", itemsDesired, nil)
    if err != nil {
        t.Fatal(err)
    }
    if err := migrate.RunStatements(ctx, c, stmts); err != nil {
        t.Fatal(err)
    }
    if err := reset(); err != nil {
        t.Fatal(err)
    }
    _, err = c.Exec(ctx, `INSERT INTO links (id, item_id) VALUES (99, 404)`, nil)
    if err == nil {
        t.Fatal("orphan insert must fail: the foreign key is not enforced after the rebuild")
    }
}

func TestSQLiteRebuildOrphanRollsBack(t *testing.T) {
    ctx := context.Background()
    c := newRebuildFixture(t)
    reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
    if err != nil {
        t.Fatal(err)
    }
    defer func() {
        if err := reset(); err != nil {
            t.Fatal(err)
        }
    }()
    // An orphan that the rebuild would carry forward (FKs are off while the orphan is inserted).
    mustExec(t, c, `INSERT INTO links (id, item_id) VALUES (99, 404)`)
    stmts, err := msqlite.Rebuild(ctx, c, "items", itemsDesired, []schema.Index{{Name: "items_name", Parts: []schema.IndexPart{{Column: "name"}}}})
    if err != nil {
        t.Fatal(err)
    }
    err = migrate.RunStatements(ctx, c, stmts)
    if err == nil || !strings.Contains(err.Error(), "foreign_key_check") || !strings.Contains(err.Error(), "rolled back") {
        t.Fatalf("want a foreign_key_check violation with rollback, got %v", err)
    }
    var sqlText string
    scanOne(t, c, `SELECT sql FROM sqlite_schema WHERE type='table' AND name='items'`, &sqlText)
    if sqlText != `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, qty INTEGER)` {
        t.Fatalf("rollback must restore the original table definition, got %q", sqlText)
    }
    var n int
    scanOne(t, c, `SELECT count(*) FROM items`, &n)
    if n != 3 {
        t.Fatalf("rollback must keep all rows, got %d", n)
    }
}

func TestSQLiteRebuildBlocked(t *testing.T) {
    ctx := context.Background()
    c := newRebuildFixture(t)
    mustExec(t, c, `CREATE TABLE a (id INTEGER PRIMARY KEY, x TEXT)`)
    mustExec(t, c, `CREATE VIEW a_v AS SELECT x FROM a`)
    desired := schema.Table{Name: "a", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}}
    stmts, err := msqlite.Rebuild(ctx, c, "a", desired, nil)
    if err != nil {
        t.Fatal(err)
    }
    err = migrate.RunStatements(ctx, c, stmts)
    if !errors.Is(err, migrate.ErrRebuildBlocked) || !strings.Contains(err.Error(), "a_v") {
        t.Fatalf("want ErrRebuildBlocked naming a_v, got %v", err)
    }
}
```

Run: `go test ./migrate/sqlite/ -run TestSQLiteRebuild -count=1`. Expect PASS for all four after Step 2 and the runner from plan momo-04 Task 15 (which implements the `fk-check` and `dependent:` hints).

- [ ] **Step 4: Commit**

`git commit -m "feat(migrate/sqlite): 12-step rebuild that drops and recreates dependent views and triggers, keeps FK enforcement, rolls back on violation"`

## Task 36: SQLite session setup and migration lock

Covers: S-13 (foreign-key handling), S-11 (concurrency lock), S-57 dependency (busy handling is in the driver). Acceptance: TestSQLiteForeignKeysOffDuringRebuild, TestSQLiteLockTable.

Files:

- Create: `migrate/sqlite/lock.go`, `migrate/sqlite/session.go`, `migrate/sqlite/lock_test.go`.

Interfaces (exact):

```go
// Setup: if opts asks for FK off (rebuild needs it) it runs PRAGMA foreign_keys=OFF on the pinned connection and returns a reset that restores the previous value. Must be called outside any transaction.
func (Dialect) Setup(ctx context.Context, c momo.PinnedConn, s migrate.SessionOptions) (reset func() error, err error)
// Lock: creates table momo_migration_lock(id INTEGER PRIMARY KEY CHECK (id = 1), holder TEXT NOT NULL, acquired_at TEXT NOT NULL) if missing;
// acquires with INSERT (one row, id=1); on conflict returns migrate.ErrLocked wrapped with the holder text.
// unlock deletes the row only when holder matches. holder() returns the current holder text or "".
// ForceUnlock deletes any row (only reachable through the --force-unlock flag, see Task 29).
func (Dialect) Lock(ctx context.Context, c momo.PinnedConn, key string) (unlock func() error, holder func(context.Context) (string, error), err error)
func ForceUnlock(ctx context.Context, c momo.PinnedConn) (previousHolder string, err error)
```

Holder text: `fmt.Sprintf("pid=%d host=%s at=%s", os.Getpid(), host, now)`.

Steps:

- [ ] **Step 1: Failing test**

`migrate/sqlite/lock_test.go`:

```go
package sqlite_test

import (
    "context"
    "testing"

    "github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"errors"
)

func TestSQLiteLockTable(t *testing.T) {
    db := testopen.SQLite(t)
    ctx := context.Background()
    unlockA, _, err := msqlite.New().Lock(ctx, pinned(t, db), "dir")
    if err != nil {
        t.Fatal(err)
    }
    _, holderB, err := msqlite.New().Lock(ctx, pinned(t, db), "dir")
    if !errors.Is(err, migrate.ErrLocked) {
        t.Fatalf("second lock must fail with ErrLocked, got %v", err)
    }
    h, herr := holderB(ctx)
    if herr != nil {
    	t.Fatal(herr)
    }
    if h == "" {
        t.Fatal("holder must name the first owner")
    }
    if err := unlockA(); err != nil {
        t.Fatal(err)
    }
    if _, _, err := msqlite.New().Lock(ctx, pinned(t, db), "dir"); err != nil {
        t.Fatalf("lock must be reacquirable after unlock: %v", err)
    }
}

func TestSQLiteForeignKeysOffDuringRebuild(t *testing.T) {
    db := testopen.SQLite(t)
    ctx := context.Background()
    c := pinned(t, db)
    reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
    if err != nil {
        t.Fatal(err)
    }
    var fk int
    mustQueryRow(t, c, `PRAGMA foreign_keys`, &fk)
    if fk != 0 {
        t.Fatal("foreign_keys must be off during rebuild")
    }
    if err := reset(); err != nil {
        t.Fatal(err)
    }
}
```

`SessionOptions` gains `ForeignKeysOff bool` (update Task 3 Interfaces accordingly; the implementer adds the field in Task 3's file). Run: `go test ./migrate/sqlite/ -run 'TestSQLiteLockTable|TestSQLiteForeignKeysOff' -count=1`. Expect FAIL with `undefined: ...Lock` or `ForeignKeysOff`.

- [ ] **Step 2: Implement** per Interfaces. Lock acquisition: `INSERT INTO momo_migration_lock (id, holder, acquired_at) VALUES (1, ?, ?)`; a `UNIQUE`/PRIMARY KEY violation (sqlite3 constraint code 1555 or 2067) maps to `migrate.ErrLocked`. Unlock: `DELETE FROM momo_migration_lock WHERE id = 1 AND holder = ?`. Setup: read `PRAGMA foreign_keys`, set OFF, reset restores the read value.

- [ ] **Step 3: Run, expect PASS** with the same command.

- [ ] **Step 4: Commit** `git commit -m "feat(migrate/sqlite): migration lock table and foreign-key session setup"`

## Coverage check (planning report)

| Scope id                                     | Tasks                                         |
| -------------------------------------------- | --------------------------------------------- |
| S-10                                         | 3, 8, 15, 16, 17, 29                          |
| S-11                                         | 4, 5, 6, 7, 8, 15, 16, 17, 18, 19, 21, 14, 25 |
| S-12                                         | 10, 11, 13, 25                                |
| S-13                                         | 33, 34, 35, 36                                |
| S-14                                         | 20                                            |
| S-27                                         | 7, 27                                         |
| S-28                                         | 14, 26, 29                                    |
| S-36                                         | 10, 22                                        |
| S-37                                         | 9, 12, 13, 32                                 |
| S-38                                         | 12, 13, 23                                    |
| S-39                                         | 9, 12, 13                                     |
| S-46                                         | 28, 29                                        |
| S-47                                         | 24                                            |
| S-74                                         | 8, 10                                         |
| S-1 (normalization)                          | 1, 2                                          |
| S-22 (migration formats)                     | 1, 5, 6, 15, 30                               |
| S-26 (forward-slash paths, TestWindowsPaths) | 29                                            |
