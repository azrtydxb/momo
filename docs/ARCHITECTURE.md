# Momo architecture and contracts

This is the binding contract for every implementation plan under
`.procoder/plans/`. Plans and code use the names below exactly; changing a
name means changing this file first. Requirements live in
`.procoder/specs/momo.md` (cited as S-n).

## Layering

```
              cmd/momo (CLI)      cmd/momovet
                 │                    │
   ┌─────────────┼──────────────┐     momovet (go/analysis)
   gen ── gen/{graphql,grpc,openapi,viz} (plugins)
   │     importer/{gorm,bun,ent}
   │     migrate ── migrate/{postgres,sqlite,mysql} ── migrate/importers/{atlas,bun,goose,golangmigrate}
   │        │
   └──── schema ◄──────────────┐
            ▲                  │
   momo (root runtime) ── dialect, vector, types
            ▲
   driver/{pgxdriver,sqlitedriver,sqlitepure,mysqldriver}
   momotest, momolog
   (separate modules) otelmomo/, bench/
```

Dependency rules (enforced by `TestImportGraph`, which runs `go list -deps`):

1. `momo`, `dialect`, `schema`, `vector`, `types` import only the standard
   library and each other. No driver, no x/tools, no YAML.
2. `driver/*` import `momo` plus their database driver only.
3. `gen`, `importer/*`, `migrate/*`, `cmd/*` may import x/tools, YAML and
   drivers. Application binaries that import only `momo` + one driver never
   link them.
4. `otelmomo` and `bench` are separate Go modules (own `go.mod`).
5. Generated code imports only `momo`, `vector`, `types`, `schema` and the
   model package.

## Packages and core names

### `dialect`

- `type Name string`; constants `Postgres = "postgres"`, `SQLite = "sqlite"`, `MySQL = "mysql"`.
- `type Flavor string`; `FlavorMySQL = "mysql"`, `FlavorMariaDB = "mariadb"` (set on MySQL dialects only).
- `type Dialect struct` with fields `Name`, `Flavor`, `MaxParams`, and capability booleans
  `Returning, OnConflict, OnDuplicateKey, Locking, NullsOrdering, DefaultInUpdate, DefaultInValues, RecursiveCTE, RowValues, Savepoints, IsolationLevels, TransactionalDDL, ILike, Merge, WindowFunctions`.
- `func For(n Name) *Dialect`, `func MySQLFlavor(f Flavor) *Dialect`.
- Methods: `AppendPlaceholder(b []byte, n int) []byte`, `AppendIdent(b []byte, ident string) []byte` (MySQL uses backticks), `QuoteIdent(string) string`.

### `schema`

- Types as in `schema/schema.go`: `Schema{FormatVersion, Dialect, Schemas, Extensions, Enums, Tables, Views, Functions, Triggers, Policies}`, `Table`, `Column`, `Unique`, `Index`, `IndexPart`, `KV`, `ForeignKey`, `Check`.
- `const FormatVersion = 1`. `func Load(r io.Reader) (*Schema, error)` refuses `format_version` > `FormatVersion` with `ErrNewerFormat`.
- `normalize.go`: `func NormalizeType(d dialect.Name, t string) string`, `func NormalizeExpr(d dialect.Name, e string) string`, `func Equal(a, b *Schema) bool`.

### `momo` (root runtime)

- Handles: `type DB`, `type Tx`, sealed `type Querier interface{ Dialect() *dialect.Dialect; conn() *execer }`. `func New(drv Driver, opts ...Option) *DB`.
- Driver contract: `Driver`, `DriverTx`, `Rows`, `TxOptions{Isolation sql.IsolationLevel; ReadOnly bool}`; optional `ErrorClassifier{ClassifyError(error) error}`, `Pinner{Pin(ctx) (PinnedConn, error)}`, `Copier{CopyFrom(ctx, table string, cols []string, rows [][]any) (int64, error)}`, `Batcher{SendBatch(ctx, []Compiled) ([]Rows, error)}`, `PoolStater{PoolStats() PoolStats}`.
- SQL building: `Builder`, `Compiled{SQL string; Args []any}`, `Expr`, `Expression[T]`, `Predicate = Expression[bool]`, `ColumnRef`, `Orderable`, `Order`, `Nulls`, `Subquery`, `TableSource`, `TableRef{Name, Alias, Schema}`.
- Aliases and joins: `Column.As(alias)` re-qualifies a column; `AsExpr()` returns it as an `Expression[T]`. Each SELECT records the qualifiers its FROM and JOIN sources expose, and a column qualified by any other table is an `ErrValidation` at build time (a reference to an enclosing SELECT, as in a correlated subquery, is allowed).
- Columns: `Column[T]`, `NullColumn[T]`, `VectorColumn[V]`, constructors `NewColumn[T] (table, name string, flags uint16)`, `NewNullColumn[T]`, `NewVectorColumn[V]`. Flags `FlagPK, FlagUnique, FlagNullable, FlagIdentity, FlagReadOnly, FlagVersion, FlagSoftDelete, FlagHasDefault, FlagCreateOnly, FlagImmutable, FlagSensitive, FlagCreatedAt, FlagUpdatedAt`.
- Writes: `Assignment` (from `Column.Set/SetExpr/SetDefault/SetExcluded`, `NullColumn.SetNull`).
- Metadata: `Model` and `Projection` markers, `ColumnMeta{Name; Flags uint16}`, `Meta[T]{Table, Schema string; Columns; Ptr func(*T,int) any; Value func(*T,int) any; TableSchema *schema.Table; Identity, Version, SoftDelete, CreatedAt, UpdatedAt int; PK []int; Hooks HookSet[T]}`, `Modeler[T]` (`*T` with `MomoMeta() *Meta[T]`).
- Queries: `Select[T, PT Modeler[T]] (q Querier) SelectQuery[T]`, `Insert[T, PT] (q) InsertQuery[T]`, `Update[T, PT] (q) UpdateQuery[T]`, `Delete[T, PT] (q) DeleteQuery[T]`, `SoftDelete[T, PT] (q) UpdateQuery[T]`, `Raw[T, PT] (q, sql string, args ...any) RawQuery[T]`, `Exec(ctx, q, sql, args...) (int64, error)`.
  All query values are immutable; methods return modified copies.
- Terminals on `SelectQuery[T]`: `All, One, First, Each, Iter, Count, Exists, SQL`; on mutations: `Exec(ctx) (Result, error)`, `SQL()`. `type Result struct{ RowsAffected int64 }`.
- Relations: `Relation[P, C]` descriptors (generated), `Load[P, C] (ctx, q, parents []P, rel Relation[P,C], opts ...LoadOption[C]) error` and `SelectQuery.Preload(...)`. `With` stays the CTE method. The exact `Relation[P,C]` shape (fields `Name, Kind, Parent, Child, ParentCols, ChildCols, ChildPK, ParentKey, ChildKey, SetParentKey, SetChildKey, GetOne, SetOne, GetMany, SetMany, Through, Discriminator, ParentMeta, ChildMeta`, as implemented in `relation.go`) is owned by momo-06 Task 2, and the generator (momo-02) emits exactly that shape.
- Transactions: `DB.Tx(ctx, TxOptions, func(*Tx) error) error`, `DB.Begin`, `Tx.Commit/Rollback/Tx/Savepoint/RollbackTo/Release`.
- Observability: `Hook{BeforeQuery(ctx,*QueryEvent) context.Context; AfterQuery(ctx,*QueryEvent)}`, `QueryEvent`, options `WithHook`, `WithArgLogging`, `WithClock(func() time.Time)`, `WithInterceptor(Interceptor)`.
- Errors (`errors.go`): `Error{Kind, Op, Table, Column, Constraint, Err}` plus `Detail() string`; sentinels `ErrNotFound, ErrTooManyRows, ErrUniqueViolation, ErrForeignKeyViolation, ErrCheckViolation, ErrNotNullViolation, ErrSerialization, ErrOptimisticLock, ErrUnsupported, ErrUnfilteredMutation, ErrTxDone, ErrInvalidVector (= vector.ErrInvalidVector), ErrValidation, ErrBusy, ErrLimitExceeded`. Migration sentinels live in `migrate`: `ErrChecksumMismatch, ErrDirty, ErrLocked, ErrDestructive, ErrBranchConflict`.
- Placeholders in raw SQL: `?` positional, `??` literal `?`, `@name` named (via `momo.Named`), `momo.In(slice)` expands, `momo.Ident(name)` quotes.

### `vector`

`Vector []float32`, `HalfVector []float32`, `SparseVector{Dim int; Indices []int32; Values []float32}`, `Bits{Len int; Data []byte}`; each implements `sql.Scanner`, `driver.Valuer`, `momo.DialectValuer`, `Validate(dim int) error`. `var ErrInvalidVector`.

Vector fields declare their dimension with the field tag `dim:<n>` (e.g. `momo:"embedding,dim:1536"`). The Go type selects the SQL type: `vector`, `halfvec`, `sparsevec` or `bit`.

### `types`

`JSON[T]`, `Date`, `TimeOfDay`, `UUID`, `Hstore`; each a codec (`sql.Scanner` + `driver.Valuer` + `SQLType(dialect.Name) string`).

### Drivers

- `driver/pgxdriver`: `New(pool *pgxpool.Pool, opts ...Option) momo.Driver`; `FromSQLDB(db *sql.DB) momo.Driver` (pgx stdlib); option `WithExecMode(pgx.QueryExecMode)`.
- `driver/sqlitedriver` (mattn + sqlite-vec, cgo): `Open(dsn string, cfg Config) (*sql.DB, momo.Driver, error)`; `Config` holds WAL/busy_timeout/synchronous defaults (S-57).
- `driver/sqlitepure` (modernc, no cgo, no vectors): same `Open` signature.
- `driver/mysqldriver`: `New(db *sql.DB, flavor dialect.Flavor) momo.Driver`.

### `migrate`

- Directory format: `migrations/<dialect>/<YYYYMMDDHHMMSS>_<name>.up.sql`, optional `.down.sql`, `<version>_<name>.snapshot.json`, `momo.sum`. Directives: `-- momo:no-transaction`, `-- momo:destructive`, `-- momo:split`, `-- momo:sqlite-foreign-keys-off`.
- Go API: `type Engine struct`; `NewEngine(drv momo.Driver, dir fs.FS, opts ...Option) (*Engine, error)`; methods `Diff(ctx, desired *schema.Schema, name string, opts DiffOptions) (*Migration, error)`, `Plan(ctx) ([]Pending, error)`, `Apply(ctx, ApplyOptions) (*Report, error)`, `Status(ctx)`, `Validate(ctx, online bool)`, `Baseline(ctx, version string, force bool)`, `Drift(ctx) (*DriftReport, error)`, `Lint(ctx) ([]Finding, error)`, `Rebase(ctx, onto string)`, `Resolve(ctx, version string, state ResolveState)`, `AutoMigrate(ctx, desired *schema.Schema) (*AutoReport, error)`.
- Per-dialect implementations satisfy `migrate.Dialect{Inspect(ctx, PinnedConn, InspectOptions) (*schema.Schema, error); Render(plan []Change) ([]Statement, error); Lock(ctx, PinnedConn) (unlock func() error, err error)}` in `migrate/postgres`, `migrate/sqlite`, `migrate/mysql`.
- History table `momo_migrations(version, name, checksum, status, statements_applied, error, applied_at, execution_ms)` plus metadata row `format_version`.

### `gen`

- `gen.Load(patterns []string, cfg Config) (*Graph, []Diagnostic, error)` → `Graph{Models []*Model; Schema *schema.Schema}`; `gen.Emit(g *Graph, w *Writer) error`; `Writer` is the only file writer (refuses non-generated targets, gofmt, deterministic).
- Plugin interface: `type Plugin interface{ Name() string; Generate(g *Graph, w *Writer) error }`.
- Generated files start with `// Code generated by momo vX.Y.Z (format 1). DO NOT EDIT.`; per-package `momo_gen.go` in the model package, per-model subpackage `<lowercase model name>/momo_gen.go`.
- Config file `momo.yaml` (parsed in `gen/config`, shared with the CLI).
- `migrate:` section: type `config.MigrateConfig` owned by momo-02, semantics by momo-04.

### Other packages

`momotest` (test helpers), `momolog` (slog hook), `momovet` (analyzer) + `cmd/momovet`, `importer/{gorm,bun,ent}`, `gen/{graphql,grpc,openapi,viz}`, `cmd/momo`.

## Contract additions (round 1, from plan reviews)

Each name below is owned by the plan in brackets; that plan's `Interfaces:`
block is the source of truth for its exact signature. Other plans consume the
name as written.

### Root `momo`

- `PinnedConn` interface: `Query`, `Exec` (same signatures as `Driver`), `Begin(ctx, TxOptions) (DriverTx, error)`, `Release() error`; obtained through `Pinner.Pin(ctx)`. `Release` closes Rows still open, then returns the connection (a repeat after success is a nil no-op); while a Query, Exec or Begin is executing in another goroutine it returns an error matching `ErrPinnedBusy` and releases nothing. `PoolStats` struct (acquired, idle, total, wait count, wait duration). `Capabilities` / `CapabilityReporter`. Exact `Copier` and `Batcher` signatures. [momo-03 Task 1]
- `func (db *DB) OnConn(pc PinnedConn) *DB`: a DB bound to one pinned connection that keeps db's hooks, classifier and options. Used for non-transactional Go migrations and session-scoped work. [momo-03]
- `testopen.Each(t, fn, opts ...momo.Option)` passes options to every dialect's DB. [momo-03]
- `WithMaxParams(n)` lowers the per-statement bind limit for chunking tests and for databases configured with lower limits. [momo-06]
- `Batcher` is implemented by the pgx driver and by pgx transactions. The pgx pool tracer calls the observability recorder for pool wait (`RecordPoolWait`, owned by momo-09). [momo-03 implements, momo-09 defines] Batch results are buffered eagerly and without bound: `SendBatch` reads every row of every statement into memory before it returns (so the connection is free and rows can be read in any order), and a failed batch returns one error naming the statement index, never its arguments. With the default `QueryExecMode` Exec the batch is sent as unnamed extended-protocol statements, so no prepared statement is created (PgBouncer-safe); pgx has no per-batch mode, so any other `WithExecMode` is accepted only when it equals the pool connection default, else `ErrUnsupported`. Pinned connections and `OnConn` handles run COPY and batch on the pinned session.
- `QueryEvent` adds `PoolWait`, `InTx`, `NumArgs`, `Dialect`. [momo-09]
- Read replicas: `WithReplicas(...)`, `DB.Reader()`; unit of work: `NewUnitOfWork(ctx) context.Context` (writes mark it; a marked context routes reads to the primary) and `AllowStaleReads(ctx) context.Context`. [momo-03 Task 10]
- `WithClock(func() time.Time)`, `WithMaxInList(n)`, `MaxRows(n)` query option, `ErrBusy`, `ErrValidation`, `ErrLimitExceeded`. Sentinels are declared in `errors.go` by momo-01. [momo-05]
- `Interceptor`, `Operation`, `WithInterceptor`. [momo-05 Task on interceptors]
- `Meta.Hooks` filled by generated code with method values; `FieldHook[T]` for Go-side defaults and validators. [momo-05]
- `Error.Detail() string`; driver-level redaction keeps driver detail off `Error()`. `Compiled` carries a sensitive-args mask used by hooks. [momo-03, momo-05]
- Read-only models: marker `View` (tag `momo:"view:<name>[,materialized]"`). `WritableModeler[T]` = `*T` with `MomoMeta()` and `MomoWritable()`. `MomoWritable` is generated only for `Model` tables, and `Insert/Update/Delete/SoftDelete` require `PT WritableModeler[T]`. `RefreshMaterializedView(ctx, q, name string, concurrently bool) error`. [momo-02 emits, momo-04 tests]
- `Tx.SetLocal(ctx, name, value string) error` (set_config(name, value, true), transaction-scoped) for RLS and search settings. [momo-04 Task 23]

### `dialect`

Additional capability flags: `JSON`, `Arrays`, `UpdateFrom`, `Merge`, `MergeReturning`. Server-version-dependent capabilities, such as PostgreSQL MERGE (>= 15) and MERGE ... RETURNING (>= 17), are gated at runtime from the driver's `ServerVersion()`. [momo-05]

### Drivers

- `pgxdriver.WithExecMode` defaults to `pgx.QueryExecModeExec` (unnamed statements, server-side binding, PgBouncer-safe). `QueryExecModeCacheStatement` is opt-in, as are `QueryExecModeCacheDescribe` and `QueryExecModeDescribeExec`. `QueryExecModeSimpleProtocol` is rejected, because values must always be bound server-side. PgBouncer transaction pooling uses `QueryExecModeExec`. `pgxdriver.FromConnConfig(cfg *pgx.ConnConfig, opts ...Option) (*sql.DB, momo.Driver)`. [momo-03]
- SQLite read-only transactions use `PRAGMA query_only` on the pinned connection. sqlite-vec is registered once per process inside `Open` (documented). [momo-03]

### `migrate`

- `migrate.Dialect` adds `Dialect() *dialect.Dialect`, `HistoryDDL() []string`, `Setup(ctx, momo.PinnedConn, SessionOptions) (reset func() error, err error)`. `Lock` also reports the holder. There is no global registry: `NewEngine` requires `migrate.WithDialect(d)`, and the CLI selects the dialect statically. [momo-04]
- The history table adds `apply_id` (groups for rollback) and `kind` (`sql`, `go`, `custom`). Transactional migrations write their history row inside the migration transaction. Only non-transactional `failed`/`running` rows are dirty. [momo-04]
- Snapshots are stored as a wrapper `{format_version, parent, schema}`. [momo-04 Task 6]
- Go migrations: `GoMigration{Version, Name, NoTx, Up, Down func(ctx, momo.Querier) error}` and `WithGoMigrations`. [momo-11]

### Generator companion (`MomoConfig`) methods

`Index, IndexExpr, Unique, Check, Enum, Default, Comment, Collate` take literals and field references. There is no `Table` method: the table name comes from the marker tag `momo:"table:<name>"` (momo-02 Task 7). `View, Function, Trigger, Policy, RLS, Partition, PartitionOf` each take one composite literal of the matching structured schema type owned by momo-04 Task 1 (`schema.View` covers materialized views through its `Materialized` field). momo-02 parses them statically; any other form is a diagnostic.

### Test harness (`internal/testdb`)

The harness returns connection strings only, so driver packages can use it without import cycles: `PostgresURL(t)`, `PostgresAppRoleURL(t)`, `PgBouncerURL(t)`, `SQLitePath(t)`, `MySQLURL(t)`, `MariaDBURL(t)`, `Schema(t)`. `internal/testdb/testopen` opens them as `*momo.DB` (`testopen.Postgres(t)`, `testopen.SQLite(t)`, ...) for packages above the drivers. In both packages, a missing URL fails the test when `MOMO_REQUIRE_INTEGRATION=1` and skips it otherwise; no test calls `t.Skip` directly. [momo-01]

## Delivery order (user decision 2026-11-04: PostgreSQL and pgvector first)

Milestones are executed in this order. Tests are written once and are
dialect-parameterized. The harness runs a dialect only once that dialect's
driver task has landed, so a later milestone extends coverage without
rewriting tests.

1. **M1: PostgreSQL + pgvector, end to end.** momo-01; momo-02; momo-03
   PostgreSQL tasks (pgx adapter, classification, transactions, pinned
   connections, COPY/batch, FromSQLDB, PgBouncer, replicas); momo-04 core
   engine plus PostgreSQL tasks, with engine tests running on PostgreSQL;
   momo-05 on PostgreSQL; momo-06 on PostgreSQL; momo-07 pgvector tasks and
   hybrid search; momo-09 hooks, momotest and OTel.
2. **M2: SQLite + sqlite-vec parity.** momo-03 SQLite tasks; momo-04 SQLite
   inspection, rendering and rebuild; momo-07 sqlite-vec task. Adding SQLite
   to `testopen.Each` turns on every existing parameterized test for SQLite.
3. **M3: MySQL/MariaDB.** momo-08.
4. **M4: Ecosystem, adoption, performance, docs site.** momo-10, momo-11,
   momo-12, momo-13.

`testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name))` runs a subtest named after each dialect available in this build. Each driver milestone adds its entry. In CI, the requirement that every
dialect's URL is set (MOMO_REQUIRE_INTEGRATION=1) applies only to dialects
whose milestone has landed.

## Ownership decisions (round 2)

- `internal/testdb` (momo-01) provides URLs only: `PostgresURL`, `PostgresAppRoleURL`, `PgBouncerURL`, `SQLitePath`, `Schema`, `MySQL84URL`, `MySQL97URL`, `MariaDBURL`. The env vars are `MOMO_TEST_POSTGRES_URL`, `MOMO_TEST_POSTGRES_APP_URL`, `MOMO_TEST_PGBOUNCER_URL`, `MOMO_TEST_MYSQL84_URL`, `MOMO_TEST_MYSQL97_URL` and `MOMO_TEST_MARIADB_URL`.
- `internal/testdb/testopen` (momo-03; it imports drivers) provides `Postgres(t, opts ...momo.Option)`, `SQLite(t)`, `MySQL84(t)`, `MySQL97(t)`, `MariaDB(t)` returning `*momo.DB`, plus `Each`. Dialect coverage is expressed by subtests named after the dialect, never by `_Postgres`-style test-name suffixes.
- `internal/suite` (momo-05) holds the shared feature suites, with `suite.Caps` and `suite.CapabilitiesOf(d *dialect.Dialect) Caps`.
- Fixtures: the root `momo_test` package has one unit fixture, owned by momo-01 in `unit_fixtures_test.go` as type `unitDocument`. The integration fixtures live in `internal/suite` (`suite.Document`, ...).
- Keyset pagination (`After`) is implemented by momo-01, including the rule that nullable keys return ErrUnsupported. momo-05 only adds integration tests.
- Schema base types and `FormatVersion` belong to momo-01. The managed-object types (`Namespace`, `View`, `Function`, `Trigger`, `Policy`, partitions, RLS flags) belong to momo-04 Task 1. They are structured definitions, and the MomoConfig companion methods in momo-02 take the same structured arguments.
- Cross-cutting spec tests with one owner: `TestWindowsPaths` (S-26) is owned by momo-04 in the CLI task. It asserts forward slashes in both the generator import/output paths (momo-02 API) and the migration file paths. Momo-01 only checks that the windows-latest CI job exists.
- Docs pages are owned by feature plans: `docs/migrations.md` (momo-04), `docs/vectors.md` (momo-07), `docs/mysql.md` (momo-08), `docs/PERFORMANCE.md` (created by momo-09 in M1 with the hook-overhead budget section, extended by momo-12 in M4). momo-13 owns the site, nav, the remaining pages, and `TestDocsCodeBlocks`, which is the only docs code extractor.

## Integration test harness

See "Ownership decisions (round 2)". `internal/testdb` (momo-01) returns URLs and paths only. `internal/testdb/testopen` (momo-03) opens a `*momo.DB` per dialect and provides `Each`. When a URL is unset, the test fails if `MOMO_REQUIRE_INTEGRATION=1` (set in CI for every landed milestone) and skips otherwise. Only these helpers call `t.Skip`. The `//go:build integration` tag is optional per test file; the harness skip/require rule is the gate. CI and `make test-integration` always run `go test -tags integration -race ./...` with `MOMO_REQUIRE_INTEGRATION=1`. `make db-up` starts `docker compose -f internal/testdb/compose.yaml up -d` with pinned images.

### Per-test isolation

`go test ./...` runs packages in parallel against one server, so every test gets its own namespace and tests may use fixed table names such as `docs`.

1. `testdb.PostgresURL(t)` connects to `MOMO_TEST_POSTGRES_URL` with `github.com/jackc/pgx/v5` directly (not a Momo driver, so no import cycle), creates schema `t_<16 hex chars from crypto/rand>`, and returns the base URL with `search_path=t_xxx,public` (a pgx runtime parameter). It is memoized per `*testing.T` (`sync.Map` keyed by `t`), so `testopen.Postgres(t)` and raw `PostgresURL(t)` calls in one test share one schema; a subtest calling it with its own `t` gets its own schema unless it uses the parent's `db`. `t.Cleanup` runs `DROP SCHEMA ... CASCADE` (error: `t.Errorf`). Once per process (`sync.Once` plus `pg_advisory_lock(7_000_001)`) it runs `CREATE EXTENSION IF NOT EXISTS vector SCHEMA public`, so `vector` types resolve through `public`.
2. `PostgresAppRoleURL(t)` uses the same schema as `PostgresURL(t)` for that `t` (it calls it first) and adds, run as the administrator, `GRANT USAGE ON SCHEMA t_xxx TO <app role>`, `ALTER DEFAULT PRIVILEGES IN SCHEMA t_xxx GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO <app role>` and `ALTER DEFAULT PRIVILEGES IN SCHEMA t_xxx GRANT USAGE, SELECT ON SEQUENCES TO <app role>` (tables the administrator creates in that schema are covered; the app role has no `CREATE`, so `CREATE TABLE` fails with SQLSTATE 42501). The role credentials come from `MOMO_TEST_POSTGRES_APP_URL`, with `search_path` appended the same way.
3. `PgBouncerURL(t)` uses the same schema and appends `search_path` the same way. The CI PgBouncer configuration sets `track_extra_parameters = search_path` (PgBouncer 1.20 or later).
4. `SQLitePath(t)` is `filepath.Join(t.TempDir(), "momo.db")`.
5. `MySQL84URL`, `MySQL97URL` and `MariaDBURL` create database `t_<16 hex>` through `github.com/go-sql-driver/mysql` directly, return the DSN with that dbname, drop it in `t.Cleanup`, and are memoized per `t`.
6. Tests never `CREATE` or `DROP` schemas or databases themselves and need not use unique table names; `DROP TABLE IF EXISTS` is unnecessary but harmless.
7. Migration-engine tests that inspect the database introspect `current_schema()` only, unless a test explicitly manages extra namespaces. Those namespace names derive from the per-test schema name: `testdb.Schema(t)` returns `t_xxx`.

## Starting point

The uncommitted spike on branch `init` (`builder.go`, `db.go`, `driver.go`, `errors.go`, `expr.go`, `meta.go`, `select.go`, `dialect/`, `schema/`) is the starting code for plan `momo-01-foundation`; it has no tests yet and is accepted only as each task's tests pass.
