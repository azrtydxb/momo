# Momo architecture

This page describes the packages that exist in the repository and the rules
they follow. It lists no planned packages. For what each database supports see
[capabilities](capabilities.md); for exact signatures read the package
documentation, which is the source of truth.

## Layering

```text
              cmd/momo (CLI)         cmd/momovet (analyzer binary)
                 │
   ┌─────────────┼──────────────┐
   gen (plugin interface)       migrate ── migrate/{postgres,sqlite,mysql}
   gen/{viz,fromdb,apiquery,    │
        graphql,grpc,openapi}   │
   │                            │
   └──── schema ◄───────────────┘
            ▲
   momo (root runtime) ── dialect, vector, types
            ▲
   driver/{pgxdriver,sqlitedriver,sqlitepure,mysqldriver}
```

Dependency rules. Tests in `internal/repocheck` enforce the first rule and
the two architecture rules below it; the rest are conventions that no test
checks.

1. Enforced by `TestImportGraph` (`go list -deps`): `momo`, `dialect`,
   `schema` and `vector` import only the standard library and each other (the
   root package may also import `types`). No driver, no `golang.org/x/tools`,
   no YAML.
2. Enforced by `TestInternalNeverImportsRoot`: the internal packages the root
   builds on never import the root.
3. Enforced by `TestFeaturePackagesImportOnlyAllowed`: the feature
   subpackages listed in `internal/repocheck/testdata/packages.txt` import only
   the root, the leaf packages and `internal/*`. `TestKnownPackages` forces
   every new package to be given a role in that file.
4. Convention: `driver/*` import `momo` plus their database driver only.
5. Convention: `gen`, `migrate/*`, `cli` and `cmd/*` may import `golang.org/x/tools`,
   YAML and drivers. An application binary that imports only `momo` and one
   driver never links them.
6. Convention: generated code imports only `momo`, `vector`, `types`, `schema`
   and the model package.

## Packages

### `dialect`

`dialect.Dialect` describes one database: its `dialect.Dialect.Name` (`postgres`, `sqlite` or
`mysql`), the MySQL `dialect.Flavor` (`mysql` or `mariadb`), the per-statement bind
limit `MaxParams` and a set of capability flags such as `Returning`,
`OnConflict`, `OnDuplicateKey`, `Locking`, `RowValues`, `Savepoints`,
`ILike` and `Merge`. It also quotes identifiers and writes placeholders.
`dialect.For` returns the dialect for a name and `dialect.MySQLFlavor` the
MySQL or MariaDB variant. The query builder consults these flags and returns
`ErrUnsupported` for what a dialect cannot do.

### `schema`

The neutral description of a database schema: tables, columns, indexes,
foreign keys, checks, enums, views, functions, triggers and policies. It is
what the generator produces from your models, what a migration snapshot
stores, and what each migration dialect inspects from a live database.
`schema.Load` reads a stored schema and refuses a newer `schema.FormatVersion` with
`schema.ErrNewerFormat`. `schema.Equal` and the `Normalize*` functions compare
schemas without being fooled by spelling differences.

### `momo` (root runtime)

- Handles: `DB`, `Tx` and the `Querier` interface that both satisfy.
  `momo.New(driver, opts...)` builds a `DB`.
- The driver contract: `Driver`, `DriverTx` and `Rows`, plus optional
  interfaces a driver can add: `ErrorClassifier`, `Pinner` (a pinned
  connection), `Copier` (bulk copy), `Batcher` (several statements in one
  round trip) and `PoolStater`.
- Typed statements: `Select`, `Insert`, `Update`, `Delete`, `SoftDelete` and
  `Merge`, built from generated column descriptors (`Column[T]`,
  `NullColumn[T]`, `VectorColumn[V]`) and expression builders. Query values
  are immutable: each method returns a modified copy.
- Raw SQL escape hatches: `momo.Exec`, `momo.Scalars`, `momo.Query`, `momo.QueryRows`, `momo.SQL`,
  `momo.TypedSQL` and `RawSubquery`, with `?` positional and `@name` named
  placeholders.
- Relations: generated `Relation` descriptors, `Load`, `LoadAll`, `Preload`
  and the join and write helpers; see [relations](relations.md).
- Transactions, savepoints and pinned connections (`DB.OnConn`).
- Handle options: hooks (`WithHook`), interceptors (`WithInterceptor`, see
  [interceptors](PRIVACY_RULES.md)), read replicas (`WithReplicas`, see
  [replicas](replicas.md)), limits (`WithMaxParams`, `WithMaxInList`) and a
  clock for tests.
- Errors: one `Error` type and the `Err*` sentinels. Match them with
  `errors.Is`, never on message text.

### `vector`

The pgvector value types `vector.Vector`, `vector.HalfVector`, `vector.SparseVector` and `vector.Bits`,
each a `sql.Scanner` and `driver.Valuer` with a `Validate` method, and
`ErrInvalidVector`. A vector field declares its dimension with the tag
`dim:<n>`; the Go type selects the SQL type. See [vectors](vectors.md).

### `types`

Codec types for columns that have no plain Go equivalent: `types.JSON[T]`, `types.Date`,
`types.TimeOfDay`, `types.UUID`, `types.Hstore` and the full-text `types.TSVector` and `types.TSQuery`. See
[type mapping](TYPE_MAPPING.md).

### `driver/*`

| Package               | Database                   | Notes                                           |
| --------------------- | -------------------------- | ----------------------------------------------- |
| `driver/pgxdriver`    | PostgreSQL                 | pgx v5 pool or `database/sql`; copy and batch   |
| `driver/sqlitedriver` | SQLite                     | cgo, not on Windows; vectors through sqlite-vec |
| `driver/sqlitepure`   | SQLite                     | pure Go, no vectors                             |
| `driver/mysqldriver`  | MySQL 8.4, 9.7 and MariaDB | vectors: see [capabilities](capabilities.md)    |

### `migrate`

The migration engine: `migrate.Engine` with `migrate.Engine.Diff`, `Plan`, `migrate.Engine.Apply`, `Status`,
`Baseline`, `Drift`, `Resolve`, `Rehash`, `Inspect`, `AutoMigrate`, `Rollback`
and `Rebase`, plus `migrate.ValidateOffline`. A migration directory holds
`<version>_<name>.up.sql`, optional hand-written `.down.sql`, snapshots and
`momo.sum`. A database implements `migrate.Dialect`; the implementations are
`migrate/postgres`, `migrate/sqlite` and `migrate/mysql`, and
`migrate/migratetest` holds helpers for tests. Which of them the `momo migrate`
command runs is generated in [capabilities](capabilities.md); the others are
library APIs. See
[migrations](migrations.md).

### `gen`

The generator. `gen.Load` reads model packages into a `gen.Graph`, `gen.Emit`
writes the generated files through the `Writer`, which is the only file writer
(it refuses targets that are not generated, formats the output and is
deterministic). `gen.Plugin` is the extension interface. Generated Go files
start with `// Code generated by momo (format 1). DO NOT EDIT.`; the header
carries the format number and no release version.

| Package                                 | Purpose                                                       |
| --------------------------------------- | ------------------------------------------------------------- |
| `gen/config`                            | `momo.yaml` parsing, shared with the CLI                      |
| `gen/fromdb`                            | models from a PostgreSQL database (`momo generate --from-db`) |
| `gen/viz`                               | Mermaid and DOT diagrams (`momo viz`), also a `gen.Plugin`    |
| `gen/apiquery`                          | cursor, limit and filter helpers for generated APIs           |
| `gen/graphql`                           | GraphQL SDL from the model graph                              |
| `gen/grpc`                              | `.proto` text with a field-number lock                        |
| `gen/openapi`                           | OpenAPI 3.1 document                                          |
| `gen/naming`, `gen/tags`, `gen/typemap` | naming presets, tag parsing, Go type to SQL type              |

### Other packages

- `cli`: the CLI as a library (`generate`, `migrate`, `viz`, `help`).
  `cli.Main(plugins...)` builds a momo binary with your own generator
  plugins; `cmd/momo` is a thin caller of it with none.
- `momovet` and `cmd/momovet`: the analyzer for non-constant raw SQL; see
  [momovet](momovet.md).
- `momotest`: a schema or a rolled-back transaction per test with your
  migrations applied; `momotest/sqlitetest` is the SQLite variant, kept apart
  so PostgreSQL tests do not link cgo. See [testing](testing.md).
- `momolog`: a `log/slog` hook for statements and relation loads.
- `obs`: ready-made `Hook` and `RelationHook` implementations (a recorder and
  an N+1 relation-load detector); imports only `momo`. See
  [observability](observability.md).
- `importer`, `importer/{gorm,bun,ent}`: static readers of other ORMs' models;
  see [importers](importers.md).
- `bench` is a separate Go module with the comparison benchmarks; see
  [performance](PERFORMANCE.md).

## Test layout

Database tests carry the `integration` build tag, so the default `go test`
run needs no database. `internal/testdb` returns connection strings for a
per-test schema or database, and `internal/testdb/testopen` opens a
`*momo.DB` per dialect, with `testopen.Each` running a test once per dialect
available in the build. CI sets `MOMO_REQUIRE_INTEGRATION=1`, which turns a
missing database URL from a skip into a failure. Shared feature suites live in
`internal/suite`. See [contributing](contributing.md) for the commands.
