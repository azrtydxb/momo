# Momo architecture and contracts

This is the contract for every implementation plan under `.procoder/plans/`.
Plans and code use the names below exactly; changing a name means changing
this file first. Requirements live in `.procoder/specs/momo.md` (cited as S-n).

The package and contract sections exist in the code today, except what the
section "Planned, not implemented" lists. Only PostgreSQL through pgx has a
driver and a migration dialect. Anything listed as planned has no code yet, and
no other document may describe it as available. The sections "Delivery order"
and "Ownership decisions" are roadmap and plan bookkeeping, not status: they
describe intended order and ownership, and do not show what has been built.

## Layering

```
              cmd/momo (CLI)
                 │
   ┌─────────────┼──────────────┐
   gen (plugin interface; gen/viz and gen/apiquery ship)
   │     migrate ── migrate/postgres
   │        │
   └──── schema ◄──────────────┐
            ▲                  │
   momo (root runtime) ── dialect, vector, types
            ▲
   driver/pgxdriver
```

Dependency rules (enforced by `TestImportGraph`, which runs `go list -deps`):

1. `momo`, `dialect`, `schema`, `vector`, `types` import only the standard
   library and each other. No driver, no x/tools, no YAML.
2. `driver/*` import `momo` plus their database driver only.
3. `gen`, `migrate/*`, `cmd/*` may import x/tools, YAML and drivers.
   Application binaries that import only `momo` + one driver never link them.
4. Generated code imports only `momo`, `vector`, `types`, `schema` and the
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
- Metadata: `Model` and `Projection` markers, `ColumnMeta{Name; Flags uint16}`, `Meta[T]{Table, Schema string; Columns; Ptr func(*T,int) any; Value func(*T,int) any; TableSchema *schema.Table; Identity, Version, SoftDelete int; SoftDeleteKind; CreatedAt, UpdatedAt int; PK []int; Fields []FieldHook[T]}`, `Modeler[T]` (`*T` with `MomoMeta() *Meta[T]`).
- Queries: `Select[T, PT Modeler[T]] (q Querier) SelectQuery[T]`, `Insert[T, PT] (q) InsertQuery[T]`, `Update[T, PT] (q) UpdateQuery[T]`, `Delete[T, PT] (q) DeleteQuery[T]`, `SoftDelete[T, PT] (q) UpdateQuery[T]`, `Exec(ctx, q, sql, args...) (int64, error)`. There is no typed raw query; `RawSubquery` and the placeholders below cover raw SQL inside a statement.
  All query values are immutable; methods return modified copies.
- Subqueries and CTEs: `SelectQuery` is a `Subquery` (IN, EXISTS, select-list scalar); `With` and `WithRecursive` add CTEs whose names are in scope for the whole statement, so a recursive CTE can reference itself; `Union(all, ...)`, `FromSubquery(q, alias)` and `CTE(name)` cover set operations and ad-hoc sources; `RawSubquery(sql, args...)` is the raw-SELECT escape hatch (verbatim trusted SQL, `?` binds). Scope checks never reject these shapes.
- Terminals on `SelectQuery[T]`: `All, One, First, Each, Iter, Count, Exists, SQL`; on mutations: `Exec(ctx) (Result, error)`, `SQL()`. `type Result struct{ RowsAffected int64 }`.
- Relations (see [relations.md](relations.md)): generated `Relation[P, C]` descriptors (`relation.go`; the generator emits one value per relation field, with key accessors, `Through` for many-to-many and `Discriminator` for polymorphic relations). Loading: `Load[P, C]` (parents, one relation, then `opts ...LoadOption[C]`) and `LoadAll[P]` (parents, then `loaders ...Loader[P]`), both taking a context and a `Querier` first and returning an error. `Loader[P]` is a sealed interface implemented by `Relation` itself, `RelationLoader` (returned by `Relation.Scope(opts ...LoadOption[C])` and `Relation.Then(nested ...Loader[C])`, which chain: `rel.Scope(...).Then(...)`) and `Relation.Joined()`; options are `LoadWhere`, `LoadOrderBy`, `LoadPerParentLimit`, `LoadWithDeleted`, `LoadOnlyDeleted`. `SelectQuery.Preload(loaders ...Loader[T])` runs the loads after the rows are read, and joined loaders fold into the parent statement. On a Batcher connection (pgx) the statements of one `LoadAll` level are pipelined in one round trip. Predicates and traversal: `Has`, `HasFrom`, `HasNo`, `CountOrder(Direction)`, `From`. Writes: `Link`, `Unlink`, `Replace`, `Count`, `Include` with `CreateGraph`, `FindOrCreate`, `UpsertByPK`. Observability: `RelationHook` and `RelationEvent`. `With` stays the CTE method.
- Transactions: `DB.Tx(ctx, TxOptions, func(*Tx) error) error`, `DB.Begin`, `Tx.Commit/Rollback/Tx/Savepoint/RollbackTo/Release`; `DB.AdoptTx(DriverTx)` binds a Tx to a caller-owned transaction (Commit and Rollback return `ErrAdoptedTx`), `Tx.DriverTx()` exposes the driver transaction.
- Observability: `Hook`, an interface with `BeforeQuery` (takes a context and a query event, returns a context) and `AfterQuery` (takes a context and a query event), `QueryEvent`, options `WithHook`, `WithArgLogging`, `WithClock` (takes a function returning the current time), `WithMaxParams`.
- Errors (`errors.go`): `Error{Kind, Op, Table, Column, Constraint, Err}` plus `Detail() string`; sentinels `ErrNotFound, ErrTooManyRows, ErrUniqueViolation, ErrForeignKeyViolation, ErrCheckViolation, ErrNotNullViolation, ErrSerialization, ErrOptimisticLock, ErrUnsupported, ErrUnfilteredMutation, ErrTxDone, ErrAdoptedTx, ErrInvalidVector (= vector.ErrInvalidVector), ErrValidation, ErrBusy, ErrLimitExceeded`. Migration sentinels live in `migrate`: `ErrChecksumMismatch, ErrDirty, ErrLocked, ErrDestructive, ErrBranchConflict`.
- Placeholders in raw SQL: `?` positional, `??` literal `?`, `@name` named (via `momo.Named`), `momo.In(slice)` expands, `momo.Ident(name)` quotes.

### `vector`

`Vector []float32`, `HalfVector []float32`, `SparseVector{Dim int; Indices []int32; Values []float32}`, `Bits{Len int; Data []byte}`; each implements `sql.Scanner`, `driver.Valuer`, `momo.DialectValuer`, `Validate(dim int) error`. `var ErrInvalidVector`.

Vector fields declare their dimension with the field tag `dim:<n>` (e.g. `momo:"embedding,dim:1536"`). The Go type selects the SQL type: `vector`, `halfvec`, `sparsevec` or `bit`.

### `types`

`JSON[T]`, `Date`, `TimeOfDay`, `UUID`, `Hstore`; each a codec (`sql.Scanner` + `driver.Valuer` + `SQLType(dialect.Name) string`).

### Drivers

- `driver/pgxdriver`: `New(pool *pgxpool.Pool, opts ...Option) momo.Driver`; `FromSQLDB(db *sql.DB) momo.Driver` (pgx stdlib); option `WithExecMode(pgx.QueryExecMode)`; `SQLTx(*sql.Tx)` and `PgxTx(pgx.Tx, opts ...Option)` wrap a caller-owned transaction for `DB.AdoptTx`; `UnwrapSQLTx(*momo.Tx)` and `UnwrapPgxTx(*momo.Tx)` return the real transaction.

### `migrate`

- Directory format: `<YYYYMMDDHHMMSS>_<name>.up.sql`, optional `.down.sql`, `<version>_<name>.snapshot.json`, `momo.sum`. Directives: `-- momo:no-transaction`, `-- momo:destructive`, `-- momo:split`. Migrations are forward only: `apply` never runs a `.down.sql` file; only `Engine.Rollback` does.
- Go API: `type Engine struct`; `NewEngine(drv momo.Driver, dir fs.FS, opts ...Option) (*Engine, error)`; methods `Diff(ctx, desired *schema.Schema, name string, opts DiffOptions) (*Migration, error)`, `Plan(ctx) ([]Pending, error)`, `Apply(ctx, ApplyOptions) (*Report, error)`, `Status(ctx)`, `Baseline(ctx, version string, force bool)`, `Drift(ctx) (*DriftReport, error)`, `Resolve(ctx, version string, state ResolveState)`, `Rehash`, `Inspect`, `AutoMigrate(ctx, desired *schema.Schema) (*AutoReport, error)`; the package function `ValidateOffline` checks a directory without a database.
- The PostgreSQL implementation in `migrate/postgres` satisfies `migrate.Dialect` (`Dialect`, `Inspect`, `Render`, `HistoryDDL`, `Setup`, `Lock`; see the additions below).
- The history table `momo_migrations` is defined by the dialect's `HistoryDDL`.

### `gen`

- `gen.Load(patterns []string, cfg Config) (*Graph, []Diagnostic, error)` → `Graph{Models []*Model; Schema *schema.Schema}`; `gen.Emit(g *Graph, w *Writer) error`; `Writer` is the only file writer (refuses non-generated targets, gofmt, deterministic).
- Plugin interface: `type Plugin interface{ Name() string; Generate(g *Graph, w *Writer) error }`.
- Generated files start with `// Code generated by momo (format 1). DO NOT EDIT.` (no momo release version, so upgrading momo does not rewrite generated files; older files that carry a version are still recognised); per-package `momo_gen.go` in the model package, per-model subpackage `<lowercase model name>/momo_gen.go`.
- `Writer.Text(path, data)` buffers non-Go generated text (Mermaid, DOT, protobuf, GraphQL schema, YAML) under the same refusal rules as `File` without formatting; the first line of `data` must be the generated header in the comment style of the format (`//`, `%%` or `#`). `Writer.Scaffold(path, data)` writes a file only when it does not exist yet: a starting point the user owns, with no header, never overwritten.
- `gen/viz`: `Edge`, `EdgesFromGraph`, `EdgesFromSchema`, `Tables`, `TablesFromSchema`, `Mermaid`, `DOT`, `Render` and `Plugin{Format, Out}` (name `viz`, writes `schema.mmd` or `schema.dot`). `momo viz [--format mermaid|dot] [--snapshot file] [--out file]` draws the models or a migration snapshot; it is a CLI command, not yet selectable from `momo.yaml`.
- `gen/graphql.SDL(g)`, `gen/grpc.Proto(g, lock, pkg)` with the field-number `Lock` (`LoadLock`, `Assign`, `Marshal`) and `gen/openapi.Document(g, opts)`: pure functions from the model graph to GraphQL SDL, a `.proto` file and an OpenAPI 3.1 JSON document. They are not yet gen.Plugin implementations and not selectable from `momo.yaml` or the CLI. The lock is JSON, so it cannot carry the generated header; writing it needs a Writer method that does not exist yet.
- `gen/apiquery`: request-side helpers shared by the API generators: `EncodeCursor`/`DecodeCursor` (opaque base64url keyset cursors), `ClampLimit` with `DefaultLimit` 50 and `MaxLimit` 200, and `ParseFilter` (a `Filter` of column, operator and text value resolved through a generated `ColumnByName`).
- Config file `momo.yaml` (parsed in `gen/config`, shared with the CLI).
- `migrate:` section: type `config.MigrateConfig` owned by momo-02, semantics by momo-04.

### Other packages

`cmd/momo` (the CLI: generate and migrate) and `momotest` (the public test kit for applications: a schema or a rolled-back transaction per test with the project migrations applied, and fixture seeding; see [testing.md](testing.md)). Every other package once listed here is under "Planned, not implemented".

## Planned, not implemented

None of the names below exist in the code. They are design intent from the
plans and may change or be dropped; do not call them from code or describe
them as available in user documentation.

- Query options and limits: `MaxRows`, `WithMaxInList`, and a typed raw query `Raw[T, PT]` returning `RawQuery[T]`.
- Interceptors: `Interceptor`, `Operation`, `WithInterceptor`.
- Read replicas and unit of work: `WithReplicas`, `DB.Replica`, `NewUnitOfWork`, `AllowStaleReads`; see docs/replicas.md.
- Generated per-model hook sets: `Meta.Hooks` as `HookSet[T]` (today `Meta.Fields` carries `FieldHook[T]` only).
- Read-only view models: marker `View` (tag `momo:"view:<name>[,materialized]"`), `WritableModeler[T]`, `MomoWritable`, and `RefreshMaterializedView`.
- Migration engine: `Engine.Lint`, `Engine.Rebase`, an online `Engine.Validate`, and the `-- momo:sqlite-foreign-keys-off` directive.
- Other databases: `driver/sqlitedriver`, `driver/sqlitepure`, `driver/mysqldriver`, `migrate/sqlite`, `migrate/mysql` and `migrate/importers/{atlas,bun,goose,golangmigrate}`. Their `testopen` entries come with them.
- Tooling packages: `momolog` (slog hook), `momovet` and `cmd/momovet` (analyzer), `importer/{bun,ent}` and the plugins and handlers of `gen/graphql`, `gen/grpc` and `gen/openapi` (their schema, protobuf and document emitters exist; see Other packages).
- Separate modules: `otelmomo` (OpenTelemetry) and `bench`.

## Contract additions (round 1, from plan reviews)

Each name below is owned by the plan in brackets; that plan's `Interfaces:`
block is the source of truth for its exact signature. Other plans consume the
name as written.

### Root `momo`

- `PinnedConn` interface: `Query`, `Exec` (same signatures as `Driver`), `Begin(ctx, TxOptions) (DriverTx, error)`, `Release() error`; obtained through `Pinner.Pin(ctx)`. `Release` closes Rows still open, then returns the connection (a repeat after success is a nil no-op); while a Query, Exec or Begin is executing in another goroutine it returns an error matching `ErrPinnedBusy` and releases nothing. Pinned connections of drivers that can destroy a session also implement the optional `Discarder{Discard() error}`: it closes the underlying connection and then releases it so the pool destroys it (open Rows closed first; `ErrPinnedBusy` while a call is in flight; nil when already released or discarded). Callers that cannot restore session state (failed advisory unlock or `RESET`) call it instead of leaving a dirty connection in the pool. `PoolStats` struct (acquired, idle, total, wait count, wait duration). `Capabilities` / `CapabilityReporter`. Exact `Copier` and `Batcher` signatures. [momo-03 Task 1]
- `func (db *DB) OnConn(pc PinnedConn) *DB`: a DB bound to one pinned connection that keeps db's hooks, classifier and options. Used for non-transactional Go migrations and session-scoped work. [momo-03]
- `testopen.Each` takes the test, a callback, and optional momo options; it passes the options to every available dialect's DB (PostgreSQL only today). [momo-03]
- `WithMaxParams(n)` lowers the per-statement bind limit for chunking tests and for databases configured with lower limits. [momo-06]
- `Batcher` is implemented by the pgx driver and by pgx transactions. The pgx pool tracer calls the observability recorder for pool wait (`RecordPoolWait`, owned by momo-09). [momo-03 implements, momo-09 defines] Batch results are buffered eagerly and without bound: `SendBatch` reads every row of every statement into memory before it returns (so the connection is free and rows can be read in any order), and a failed batch returns one error naming the statement index, never its arguments. With the default `QueryExecMode` Exec the batch is sent as unnamed extended-protocol statements, so no prepared statement is created (PgBouncer-safe); pgx has no per-batch mode, so any other `WithExecMode` is accepted only when it equals the pool connection default, else `ErrUnsupported`. Pinned connections and `OnConn` handles run COPY and batch on the pinned session.
- `QueryEvent` adds `PoolWait`, `InTx`, `NumArgs`, `Dialect`. [momo-09]
- `WithClock` (takes a function returning the current time), `ErrBusy`, `ErrValidation`, `ErrLimitExceeded`. Sentinels are declared in `errors.go` by momo-01. [momo-05]
- `Meta.Fields` holds one `FieldHook[T]` per column that needs a Go-side default or validator. [momo-05]
- The `Error.Detail` method returns the driver detail as a string; driver-level redaction keeps driver detail off the `Error` text. The compiled statement carries a sensitive-args mask used by hooks: values bound from `sensitive` columns show as `momo.RedactedArg`; raw `momo.SQL` arguments are not masked. [momo-03, momo-05]
- `Tx.SetLocal(ctx, name, value string) error` (set_config(name, value, true), transaction-scoped) for RLS and search settings. [momo-04 Task 23]

### `dialect`

Only the `Merge` capability flag exists on `dialect.Dialect` beyond the base set. The flags `JSON`, `Arrays`, `UpdateFrom` and `MergeReturning` are planned. Server-version-dependent capabilities, such as PostgreSQL MERGE (>= 15) and MERGE ... RETURNING (>= 17), are meant to be gated at runtime from the driver's reported server version. [momo-05]

### Drivers

- `pgxdriver.WithExecMode` defaults to `pgx.QueryExecModeExec` (unnamed statements, server-side binding, PgBouncer-safe). `QueryExecModeCacheStatement` is opt-in, as are `QueryExecModeCacheDescribe` and `QueryExecModeDescribeExec`. `QueryExecModeSimpleProtocol` is rejected, because values must always be bound server-side. PgBouncer transaction pooling uses `QueryExecModeExec`. `pgxdriver.FromConnConfig(cfg *pgx.ConnConfig, opts ...Option) (*sql.DB, momo.Driver)`. [momo-03]

### `migrate`

- `migrate.Dialect` adds `Dialect() *dialect.Dialect`, `HistoryDDL() []string`, `Setup(ctx, momo.PinnedConn, SessionOptions) (reset func() error, err error)`. `Lock` also reports the holder. There is no global registry: `NewEngine` requires `migrate.WithDialect(d)`, and the CLI selects the dialect statically. [momo-04]
- The history table adds `apply_id` (groups the migrations of one apply run) and `kind` (`sql`, or `go` for a `GoMigration`). Transactional migrations write their history row inside the migration transaction. Only non-transactional `failed`/`running` rows are dirty. [momo-04]
- Snapshots are stored as a wrapper `{format_version, parent, schema}`. [momo-04 Task 6]
- `migrate.GoMigration` (fields `Version`, `Name`, `Up`, `UpNoTx`, `Checksum`) and the option `migrate.WithGoMigrations`: Go migrations are ordered with the SQL files by version and recorded with `kind = 'go'`. `Up` takes a `*momo.Tx` and runs in the migration transaction; `UpNoTx` takes a `*momo.DB` on the pinned connection through `OnConn`, with the dirty/resolve semantics of a no-transaction SQL file. The recorded checksum is a hash of version, name and the optional `Checksum` string (code cannot be hashed). A version shared with a SQL file is an error. They exist only in the application's process, so the `momo` CLI cannot run them; they are forward-only. `ValidateOffline` takes the registered migrations as extra arguments to check their snapshots. `migrate.WithGoMigrationDB` supplies the application's `*momo.DB` for them. Apply returns `ErrGoMigrationUnregistered` when a snapshot-only version (a Go migration the engine does not have) would be passed by later SQL. `RefuseGoRollback` and `ErrGoMigrationNotReversible` are for the rollback code to refuse rows of kind `go`. See docs/migrations.md section 6a. [issue 43]

### Generator companion (`MomoConfig`) methods

`Index, IndexExpr, Unique, Check, Enum, Default, Comment, Collate` take literals and field references. There is no `Table` method: the table name comes from the marker tag `momo:"table:<name>"` (momo-02 Task 7). `View, Function, Trigger, Policy, RLS, Partition, PartitionOf` each take one composite literal of the matching structured schema type owned by momo-04 Task 1 (`schema.View` covers materialized views through its `Materialized` field). momo-02 parses them statically; any other form is a diagnostic.

### Test harness (`internal/testdb`)

The harness returns connection strings only, so driver packages can use it without import cycles: `PostgresURL`, `PostgresAppRoleURL`, `PgBouncerURL`, `SQLitePath`, `MySQL84URL`, `MySQL97URL`, `MariaDBURL` and `Schema`, each taking the `*testing.T`. `internal/testdb/testopen` opens PostgreSQL as a `*momo.DB` (`testopen.Postgres(t)`, `testopen.PostgresIn`) for packages above the drivers; only PostgreSQL has a driver, so there is no `testopen.SQLite` or MySQL opener yet. In both packages, a missing URL fails the test when `MOMO_REQUIRE_INTEGRATION=1` and skips it otherwise; no test calls `t.Skip` directly. [momo-01]

### Public test kit (`momotest`)

`momotest` is the public counterpart for applications; it is independent of `internal/testdb` (it does not require pgvector). `momotest.New(t, opts...)` returns a `*momo.DB` on a per-test schema `momotest_<16 hex>` with the project migrations applied (`WithMigrations(fs.FS)`, run through `migrate.Engine` with `WithoutSum`), dropped in `t.Cleanup` after the pool is closed. `momotest.NewTx(t, opts...)` returns a `*momo.Tx` from `DB.AdoptTx` on a template schema named `momotest_tpl_<hash of the migration files and extensions>`, built once under a PostgreSQL advisory lock and marked ready with a schema comment, so a half-built template left by a killed run is rebuilt; the transaction is rolled back in `t.Cleanup`. `momotest.Seed`, `Insert` and `Graph` insert fixtures in order inside one transaction or savepoint. The URL comes from `MOMO_TEST_POSTGRES_URL` (`WithEnv` changes it), with the skip and `MOMO_REQUIRE_INTEGRATION=1` behavior of `testdb`. See [testing.md](testing.md).

## Delivery order (user decision 2026-11-04: PostgreSQL and pgvector first)

This is a roadmap, not a status report. Milestones are executed in this order. Tests are written once and are
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
- `internal/testdb/testopen` (momo-03; it imports drivers) provides `Postgres(t, opts ...momo.Option)` and `PostgresIn` returning `*momo.DB`, plus `Each`. Dialect coverage is expressed by subtests named after the dialect, never by `_Postgres`-style test-name suffixes.
- `internal/suite` (momo-05) holds the shared feature suites, with `suite.Caps` and `suite.CapabilitiesOf(d *dialect.Dialect) Caps`.
- Fixtures: the root `momo_test` package has one unit fixture, owned by momo-01 in `unit_fixtures_test.go` as type `unitDocument`. The integration fixtures live in `internal/suite` (`suite.Document`, ...).
- Keyset pagination (`After`) is implemented by momo-01, including the rule that nullable keys return ErrUnsupported. momo-05 only adds integration tests.
- Schema base types and `FormatVersion` belong to momo-01. The managed-object types (`Namespace`, `View`, `Function`, `Trigger`, `Policy`, partitions, RLS flags) belong to momo-04 Task 1. They are structured definitions, and the MomoConfig companion methods in momo-02 take the same structured arguments.
- Cross-cutting spec tests with one owner: `TestWindowsPaths` (S-26) is owned by momo-04 in the CLI task. It asserts forward slashes in both the generator import/output paths (momo-02 API) and the migration file paths. Momo-01 only checks that the windows-latest CI job exists.
- Docs pages are owned by feature plans: `docs/migrations.md` (momo-04), `docs/vectors.md` (momo-07). `docs/mysql.md` and `docs/PERFORMANCE.md` do not exist yet. momo-13 owns the site, nav and the remaining pages.

## Integration test harness

See "Ownership decisions (round 2)". `internal/testdb` (momo-01) returns URLs and paths only. `internal/testdb/testopen` (momo-03) opens a `*momo.DB` per dialect and provides `Each`. When a URL is unset, the test fails if `MOMO_REQUIRE_INTEGRATION=1` (set in CI for every landed milestone) and skips otherwise. Only these helpers call `t.Skip`. Every test file that uses these helpers carries the `//go:build integration` tag, enforced by `TestIntegrationTagConvention` in `internal/repocheck`; untagged builds (`make test`, the macOS and Windows jobs) therefore contain no database test. The harness skip/require rule still decides skip versus fail inside a tagged run. CI and `make test-integration` always run `go test -tags integration -race ./...` with `MOMO_REQUIRE_INTEGRATION=1`. `make db-up` starts `docker compose -f internal/testdb/compose.yaml up -d` with pinned images.

### Per-test isolation

`go test ./...` runs packages in parallel against one server, so every test gets its own namespace and tests may use fixed table names such as `docs`.

1. `testdb.PostgresURL(t)` connects to `MOMO_TEST_POSTGRES_URL` with `github.com/jackc/pgx/v5` directly (not a Momo driver, so no import cycle), creates schema `t_<16 hex chars from crypto/rand>`, and returns the base URL with `search_path=t_xxx,public` (a pgx runtime parameter). It is memoized per `*testing.T` (`sync.Map` keyed by `t`), so `testopen.Postgres(t)` and raw `PostgresURL(t)` calls in one test share one schema; a subtest calling it with its own `t` gets its own schema unless it uses the parent's `db`. `t.Cleanup` runs `DROP SCHEMA ... CASCADE` (error: `t.Errorf`). Once per process (`sync.Once` plus `pg_advisory_lock(7_000_001)`) it runs `CREATE EXTENSION IF NOT EXISTS vector SCHEMA public`, so `vector` types resolve through `public`.
2. `PostgresAppRoleURL(t)` uses the same schema as `PostgresURL(t)` for that `t` (it calls it first) and adds, run as the administrator, `GRANT USAGE ON SCHEMA t_xxx TO <app role>`, `ALTER DEFAULT PRIVILEGES IN SCHEMA t_xxx GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO <app role>` and `ALTER DEFAULT PRIVILEGES IN SCHEMA t_xxx GRANT USAGE, SELECT ON SEQUENCES TO <app role>` (tables the administrator creates in that schema are covered; the app role has no `CREATE`, so `CREATE TABLE` fails with SQLSTATE 42501). The role credentials come from `MOMO_TEST_POSTGRES_APP_URL`, with `search_path` appended the same way.
3. `PgBouncerURL(t)` uses the same schema and appends `search_path` the same way. The test PgBouncer image cannot track `search_path`, so a startup `search_path` is not reliable through the pooler. `PgBouncerURLFor(t, dbname)` returns the pooler URL for another database (no `search_path`; the pooler routes every database name) and `ScratchDatabase(t, prefix)` creates and later drops such a database, so the pooled shared suite runs in a scratch database of its own.
4. `SQLitePath(t)` is `filepath.Join(t.TempDir(), "momo.db")`.
5. `MySQL84URL`, `MySQL97URL` and `MariaDBURL` create database `t_<16 hex>` through `github.com/go-sql-driver/mysql` directly, return the DSN with that dbname, drop it in `t.Cleanup`, and are memoized per `t`.
6. Tests never `CREATE` or `DROP` schemas or databases themselves and need not use unique table names; `DROP TABLE IF EXISTS` is unnecessary but harmless.
7. Migration-engine tests that inspect the database introspect `current_schema()` only, unless a test explicitly manages extra namespaces. Those namespace names derive from the per-test schema name: `testdb.Schema(t)` returns `t_xxx`.
