# momo

Status: ready

## Problem

Go teams building data-heavy (and increasingly vector/RAG) services pick
between ORMs that each give up something: GORM is approachable but
reflection-driven and stringly typed (errors surface at runtime, zero
values are ambiguous on update); Ent gives compile-time checks but needs a
second schema DSL and leans on Atlas for migrations; Bun gives SQL control
but little type safety. None treats vectors as part of the
model → query → migration lifecycle, and the mature migration paths depend
on external tools or paid services. Momo combines the approachable model
(ordinary structs), generated compile-time checks, explicit SQL control,
first-class pgvector/sqlite-vec support and a self-contained migration
engine — with performance claims backed only by reproducible benchmarks.

Momo serves two audiences at once: it will be used in our own products and
published as open source, so documentation, benchmarks and the capability
matrix are first-class deliverables, not afterthoughts.

## Users

- **Application developers** — define models once as Go structs, write
  typed queries the compiler checks, load relations explicitly, run
  transactions; need predictable SQL and no hidden queries.
- **Platform / DBA reviewers** — review versioned SQL migrations in pull
  requests; need reviewable SQL, explicit destructive-change authorization,
  drift detection and documented recovery from partial failures.
- **Operators / CI** — run `momo generate --check`, `momo migrate validate`
  and `momo migrate apply` in pipelines; need deterministic output,
  non-zero exit codes on problems, no network/account dependencies.
- **Performance-sensitive teams** — need published, reproducible benchmark
  numbers against handwritten pgx/database-sql code and GORM, Ent, Bun.
- **Contributors** — need an architecture doc, contributor guide, CI and a
  capability matrix to extend dialects and features safely.

## In scope

- [S-1] Normalized schema representation shared by the generator and migration engine (tables, columns, PKs incl. composite, uniques, indexes incl. expression/partial/method/opclass, FKs, checks, enums, extensions).
- [S-2] Model declaration with ordinary Go structs plus struct tags for simple options and a statically inspected, typed companion declaration for complex configuration; unsupported companion expressions produce diagnostics with source positions.
- [S-3] `momo generate` code generator (go/packages + type info): typed column descriptors and predicates, mutation helpers, relation metadata, argument binding, scanners, schema metadata; deterministic, gofmt-clean, no live database, cross-package, collision-safe, marks generated files, never overwrites handwritten files, `--check` for staleness, `go generate` integration.
- [S-4] Typed query API: filters and boolean composition, ordering with NULLS placement, limit/offset, keyset pagination with deterministic tie-breaking, joins with aliases and self-joins, typed projections, aggregation/grouping, subqueries, CTEs incl. recursive, returning, conflict handling, locking, raw parameterized SQL, streaming and callback iteration, compile-without-execute; immutable reusable queries.
- [S-5] Write semantics: distinct omitted / zero / NULL / DEFAULT states; typed insert/update builders; single and bulk insert; update; delete; upsert; optimistic locking; opt-in soft delete; guardrail against unfiltered UPDATE/DELETE with explicit full-table opt-in; no implicit object-graph saves.
- [S-6] Relationships: belongs-to, has-one, has-many, many-to-many, composite keys; explicit batched eager loading with bind-limit chunking, correct parent pagination, stable association, defined empty/NULL behavior, no parent duplication.
- [S-7] Transactions: context cancellation, callback and explicit handles, savepoints, isolation options, rollback on error and panic, documented ownership; no automatic retry of callbacks.
- [S-8] Structured errors supporting errors.Is/As: not found, too many rows, unique/FK/check/not-null violations, optimistic-lock conflict, unsupported capability, invalid vector dimension, migration checksum mismatch, dirty migration.
- [S-9] Driver adapters: native pgx (pgxpool) for PostgreSQL and database/sql for SQLite; dialect capabilities separate from driver capabilities; native PostgreSQL batching/COPY bulk path.
- [S-10] Migration engine (Go API + CLI): inspect, normalize, diff, dependency-aware plan, reviewable SQL generation, execution, history with checksums and recovery status; commands generate, generate --check, migrate diff/plan/apply/status/validate/baseline/drift with defined contracts.
- [S-11] Migration safety: versioned SQL files, per-migration historical schema snapshots, explicit rename declarations, explicit destructive authorization, baselining, drift detection, unmanaged-object ownership rules, concurrency lock, transactional and non-transactional execution, partial-failure recovery, custom SQL and data migrations, staged changes, optional authored down migrations, no-change round trips.
- [S-12] PostgreSQL migration support: extensions (pgvector), enums, constraints, expression and partial indexes, index methods and operator classes, CREATE/DROP INDEX CONCURRENTLY outside transactions.
- [S-13] SQLite migration support: version-aware ALTER TABLE, 12-step table rebuild preserving indexes, constraints, triggers and data, foreign-key handling, integrity verification (foreign_key_check).
- [S-14] Development AutoMigrate: additive only (create tables, add columns/indexes), reports what it skipped.
- [S-15] pgvector: vector, halfvec, sparsevec, bit types; distance expressions; dimension/value validation; encoding/scanning; HNSW and IVFFlat index config with matching opclasses; exact vs approximate search controls; filtered nearest neighbour; transaction-scoped search settings; EXPLAIN inspection.
- [S-16] SQLite vector search via a pinned sqlite-vec version: documented platforms, loading, metrics, types, limitations and differences from pgvector; no silent full-scan substitution for an unsupported indexed operation.
- [S-17] PostgreSQL hybrid search (full-text + vector) with explicit candidate selection, rank fusion and ordering, built after the core vector path is verified.
- [S-18] Observability: optional hooks for duration, pool wait, rows, query counts, relation loading, errors and tracing; parameters excluded from logs by default; SQL inspection and explicit EXPLAIN / EXPLAIN ANALYZE helpers; optional tenant scope helper documented as not a security boundary; no hidden goroutines, implicit migrations or global mutable state.
- [S-19] Benchmarks: ORM-only microbenchmarks and real-database workloads vs handwritten pgx/database-sql, GORM, Ent, Bun; p50/p95/p99; equivalence rules; recall-matched vector benchmarks; reproducible commands, environment, raw results, repeated runs, noise-aware regression policy, measured budgets; generated-code size and compile cost.
- [S-20] Testing: unit, compile-fail fixtures, generator golden/determinism tests, real PostgreSQL+pgvector and SQLite integration tests, race detector, fuzzing of SQL building, schema normalization and value encoding; CI running all of it.
- [S-21] Documentation and examples: architecture, API docs, capability matrix, migration safety and recovery guide, contributor guide, supported-version record (indexed in `docs/README.md`); runnable examples for CRUD, zero/NULL updates, transactions, relations, projection and pagination, vector search, create/upgrade a database, custom SQL migration, adopting an existing schema.
- [S-22] Versioning and releases: standard Go semantic versioning with vX.Y.Z git tags, release-please automation from Conventional Commits, a generated changelog, `go install`-only CLI distribution, and format versions on persisted artifacts (schema snapshots, checksum file, history table, generated code).
- [S-23] Documentation site (sources indexed in `docs/README.md`) on GitHub Pages for humans and AI coding agents: MkDocs Material built and deployed by a GitHub Actions workflow; versioned per release with mike (`/latest/`, `/vX.Y/`, `/dev/` from main); `/llms.txt` index and `/llms-full.txt` full-text bundle generated at build time; every page also published as raw Markdown; a dedicated agent usage guide with canonical patterns, do/don't rules and a copy-paste AGENTS.md snippet for consumer repositories; every code block in the docs extracted and compiled or run in CI.

- [S-24] Licensing and community files: Apache-2.0 LICENSE, SECURITY.md (private vulnerability reporting via GitHub Security Advisories), CONTRIBUTING.md, CODE_OF_CONDUCT.md, GitHub issue forms and a pull-request checklist.
- [S-25] Supply-chain hygiene: govulncheck in CI, Dependabot for Go modules, GitHub Actions and the docs Python requirements, OpenSSF Scorecard workflow, every third-party action pinned by commit SHA, least-privilege workflow permissions.
- [S-26] Windows support: generator, CLI and the SQLite+sqlite-vec suite built (cgo, MinGW) and tested on a windows-latest runner; PostgreSQL integration stays on Linux.
- [S-27] Branch-safe migration numbering: UTC timestamp versions (YYYYMMDDHHMMSS), a linear snapshot-chain check that detects two branches' migrations built on the same parent, and `momo migrate rebase` to regenerate a branch's migrations on top of the target branch.
- [S-28] Migration lint and lock safety: `momo migrate lint` flags blocking or rewriting operations (NOT NULL without default on existing tables, column type changes, non-concurrent index builds on existing PostgreSQL tables, SQLite rebuilds of large tables) with a severity and a safer staged alternative; apply sets configurable `lock_timeout` and `statement_timeout` on PostgreSQL.
- [S-29] Filtered approximate vector search: expose pgvector iterative index scans (the pgvector hnsw and ivfflat iterative_scan settings and max scan tuples) as scoped search settings and document when a filtered ANN query can return fewer than k rows.
- [S-30] Connection-pooler compatibility: a pgx adapter option for the query execution mode (cached statements vs. simple/exec protocol) so Momo works behind PgBouncer transaction pooling; tested against a PgBouncer container.
- [S-31] Documented Go-to-SQL type mapping for both dialects (integers, floats, numeric/decimal, text, citext, bool, bytea/BLOB, uuid, timestamp vs timestamptz and SQLite time storage, date, interval, jsonb/JSON, arrays, enums, vectors) with a round-trip test per mapping.
- [S-32] Typed JSON and array querying: JSONB path and containment operators (`->`, `->>`, `@>`, `?`), array containment/overlap and `= ANY(...)` on PostgreSQL; SQLite JSON functions where equivalent, ErrUnsupported otherwise.
- [S-33] Window functions (ROW_NUMBER, RANK, DENSE_RANK, aggregates OVER with PARTITION BY/ORDER BY) and an explicit second relation-loading strategy that loads the top N children per parent with a window function.
- [S-34] Testing helpers for applications (package momotest): per-test transactions rolled back on cleanup, SQL assertion helpers using compile-without-execute, fixture loading, and ephemeral SQLite databases.
- [S-35] OpenTelemetry integration as a separate Go module (otelmomo) implementing the hook interface with spans and metrics following OTel database semantic conventions; the core module gains no OTel dependency.
- [S-36] PostgreSQL schemas: schema-qualified models and tables, migrations that create and manage non-public schemas, inspection and drift across configured schemas.
- [S-37] Views and materialized views as read-only models: generated query support, migrations that create, replace and drop them in dependency order, and a REFRESH MATERIALIZED VIEW helper.
- [S-38] Row-level security for tenancy: per-transaction session settings via `set_config(..., true)`, migration support for ENABLE/FORCE ROW LEVEL SECURITY and CREATE POLICY, documented as the database-enforced boundary, unlike the application-side tenant scope helper.
- [S-39] Partitioned tables, triggers and functions as managed migration objects on PostgreSQL (declarative partitioning, CREATE OR REPLACE FUNCTION, CREATE TRIGGER) and triggers on SQLite, included in inspection, diff and drift.
- [S-40] Read replicas: an explicit read handle routed to replica drivers chosen by the application; never used inside transactions or after a write in the same unit of work unless asked; replication lag documented.

- [S-41] Error redaction: constraint-violation errors exclude offending values (e.g. PostgreSQL key detail) from Error() text by default; the raw detail stays reachable through an explicit accessor and errors.As.
- [S-42] Sensitive columns: a `sensitive` field tag excludes the column's values from hook events (even with argument logging), error text and test-helper SQL dumps.
- [S-43] Safe dynamic ordering and filtering: generated allowlisted lookups from external names to typed columns (unknown names return an error), and a LIKE-pattern escaping helper.
- [S-44] Raw-SQL static analysis: a `momovet` go/analysis analyzer, runnable via `go vet -vettool`, that flags non-constant SQL strings passed to Momo's raw-SQL entry points.
- [S-45] SQLite extension safety: sqlite-vec statically linked; runtime extension loading never enabled by Momo, and the adapter verifies load_extension is unavailable from SQL.
- [S-46] CLI secret handling: database URLs only from environment variables or the config's env indirection, redacted in all CLI output and errors; warning for disabled TLS against non-local hosts.
- [S-47] Least-privilege roles: documented split between a migration (DDL) role and an application (DML) role; integration and RLS tests run as a non-superuser application role.
- [S-48] Resource-exhaustion guards: optional max-rows cap for collecting reads, an IN-list size cap, and documented context-deadline usage for statement timeouts.
- [S-49] Static security analysis in CI: CodeQL and gosec on free GitHub runners.
- [S-50] pgvector binary wire encoding: pgx binary codecs for vector, halfvec, sparsevec and bit, benchmarked against text encoding.
- [S-51] Pipelined relation loading on PostgreSQL: multiple relation loads sent in one round trip via pgx batching, with measured gain.
- [S-52] PostgreSQL bulk insert via unnest array parameters, benchmarked against chunked VALUES and COPY.
- [S-53] N+1 detection hook for development: flags repeated statement shapes within one context beyond a threshold.
- [S-54] Slow-query hook: threshold-based reporting with opt-in plain EXPLAIN capture; never EXPLAIN ANALYZE automatically.
- [S-55] Index-usage assertions: a momotest helper that fails when a query's plan does not use the named index.
- [S-56] Bounded-memory streaming: Each/Iter keep memory flat independent of result size.
- [S-57] Recommended SQLite configuration: WAL, busy_timeout, synchronous=NORMAL, single-writer pool guidance, SQLITE_BUSY mapped to a structured error.
- [S-58] Allocation-regression gating in CI with benchstat on deterministic allocation counts; timing regressions judged only on the dedicated benchmark machine.

- [S-59] MySQL/MariaDB dialect and adapter via go-sql-driver/mysql: typed queries, writes (LAST_INSERT_ID where RETURNING is unavailable, RETURNING on MariaDB where supported), upsert via ON DUPLICATE KEY UPDATE, locking, CTEs, error classification.
- [S-60] MySQL/MariaDB migrations: information_schema inspection, diff and SQL generation, GET_LOCK-based concurrency lock, explicit handling of implicit-commit DDL (every migration treated as non-transactional with statement-level recovery state), online DDL ALGORITHM/LOCK options.
- [S-61] Vector support on MariaDB (VECTOR(N), VEC_DISTANCE functions, VECTOR INDEX) and MySQL (VECTOR storage; distance queries only where the server provides distance functions, ErrUnsupported otherwise), with exact versions verified and recorded.
- [S-62] Generator plugin interface: documented, versioned extension points that receive the normalized schema and relation graph and emit files through the same deterministic, collision-safe writer.
- [S-63] First-party GraphQL generator: schema SDL, gqlgen-compatible resolvers, Relay connections with keyset pagination, typed filter and order inputs, and batched relation resolution.
- [S-64] First-party gRPC generator: protobuf messages and CRUD/list services from models, with field-number stability recorded in a lock file so regeneration never renumbers fields.
- [S-65] First-party OpenAPI generator: OpenAPI 3.1 document and optional net/http handlers for CRUD/list endpoints with filtering and keyset pagination.
- [S-66] `momo viz`: ER diagrams (Mermaid and Graphviz DOT) from models or a migration snapshot.
- [S-67] Embedded structs flattened into columns with optional prefixes, and reusable mixins that contribute columns, indexes and companion configuration.
- [S-68] Automatic created/updated timestamps via tags, stored as timestamps or Unix seconds/milliseconds/nanoseconds; explicit assignment overrides; an injectable clock.
- [S-69] Naming strategies with presets reproducing GORM, Bun and Ent table/column/join-table naming, configurable in momo.yaml and overridable per model and field.
- [S-70] Complete field tag vocabulary: size, precision, scale, type, comment, collate, default, check, per-column index sort and nulls order.
- [S-71] Field write permissions: read-only, create-only and immutable fields enforced by generated mutation builders at compile time where possible and at build time otherwise.
- [S-72] Go-side default and update-default functions and field validators, run by the write builders before SQL is built.
- [S-73] Codec interface for custom types plus a momo/types package: JSON[T], Date, TimeOfDay, UUID, hstore, with per-dialect storage.
- [S-74] Foreign-key ON DELETE/ON UPDATE actions and a per-relation option to omit the database constraint.
- [S-75] Soft-delete variants: timestamp, Unix-time and boolean marker columns, OnlyDeleted and ForceDelete, unique indexes defined over the marker.
- [S-76] Relation-derived predicates (HasX, HasXWith, HasNoX as EXISTS), traversal from parents to a typed child query, and ordering by relation count or related field.
- [S-77] Named arguments, slice expansion for IN, tuple IN for composite keys, and quoted identifier arguments in raw SQL fragments.
- [S-78] Result helpers: Pluck, IDs/FirstID, scanning into maps and scalars, FindAndCount, and Batches(n) built on keyset pagination.
- [S-79] Query composition and escape hatches: named scopes, a typed fragment extension point, NOWAIT/SKIP LOCKED, optimizer hints, runtime table-name override guarded by quoting, and a runtime-buildable filter tree validated against the column allowlist.
- [S-80] MERGE (where supported), bulk update from a VALUES list, and a Truncate helper.
- [S-81] Nested and filtered/ordered/limited eager loading, and single-query join loading for to-one relations with NULL-to-nil semantics.
- [S-82] Polymorphic has-one/has-many, many-to-many through an explicit join model with extra columns and custom naming, self-referencing relations, and optional back-references.
- [S-83] Explicit association helpers Link, Unlink, Replace and Count, each issuing exactly one documented statement (Replace: one transaction).
- [S-84] Explicit opt-in graph and convenience writes: CreateGraph with listed relations in one transaction, FindOrCreate, and Upsert by primary key.
- [S-85] Typed model hooks (BeforeInsert, BeforeUpdate, BeforeDelete, AfterLoad) invoked by generated code without reflection; hooks may set values but not rewrite SQL.
- [S-86] Per-handle interceptors that may wrap or modify typed queries and mutations (audit, policy), plus a documented privacy-rules recipe distinguished from database RLS.
- [S-87] A ready-made slog debug logger hook with levels and slow threshold.
- [S-88] PostgreSQL adapter over an existing *sql.DB (pgx stdlib), sharing classification and vector support with the native adapter.
- [S-89] Optional pure-Go SQLite driver (modernc.org/sqlite) without vector support; vector operations return ErrUnsupported.
- [S-90] Pinned single-connection access and pool statistics through the adapters.
- [S-91] `momo import gorm|bun|ent`: converts existing models (gorm/bun tags, Ent schema graph) into Momo models and companion configuration, reporting every semantic change (zero-value handling, implicit saves, Go defaults).
- [S-92] `momo generate --from-db`: generates Momo models from an inspected database on any dialect.
- [S-93] Migration history importers for Atlas directories, Bun bun_migrations, goose and golang-migrate, producing a baseline plus converted files.
- [S-94] Go-function migrations, a statement-split directive, and rollback of the last applied group.
- [S-95] Migration guides for GORM, Bun and Ent users with call-site cheat sheets whose code blocks are compiled in CI.

## Out of scope

- Databases other than PostgreSQL, SQLite and MySQL/MariaDB (SQL Server, Oracle, CockroachDB-specific behavior).
- Copying conflicting behaviors from GORM/Bun/Ent (zero-value omission, implicit association saves, implicit per-write transactions, reflection-driven loading of all relations, auto-migrate that drops or alters); each has an explicit Momo replacement in scope.
- Table sharding.
- Embedding generation or any call to an external model provider.
- Atlas or any hosted/paid migration service, accounts or remote control plane.
- Implicit lazy loading and automatic saving of object graphs.
- Automatic retry of application transaction callbacks.
- Automatic read/write routing by statement inspection (replica use is always explicit).
- Guessing renames, automatic data-lossy type changes, or promising every schema change is automatic, reversible or non-blocking.
- Compile-time validation of arbitrary raw SQL.
- Performance claims without reproducible evidence; zero-allocation promises.
- A query cache, connection pool or statement cache of Momo's own (drivers own pooling and preparation).

## Constraints

- Go: minimum 1.26.10 in go.mod; CI tests 1.26 and 1.27. Dependencies pinned in go.mod/go.sum.
- SQLite driver: mattn/go-sqlite3 v1.14.52 (cgo) with sqlite-vec v0.1.6 statically linked via github.com/asg017/sqlite-vec-go-bindings/cgo; SQLite builds require cgo and a C toolchain. The database/sql adapter itself stays driver-agnostic.
- PostgreSQL driver: jackc/pgx v5 (pinned) via pgxpool, plus pgx stdlib for *sql.DB adoption.
- MySQL/MariaDB: github.com/go-sql-driver/mysql (pinned); supported servers MySQL 8.4 LTS, MySQL 9.7 LTS and MariaDB 11.8 LTS (no innovation releases). Vector capability: MariaDB 11.8 full search with VECTOR INDEX; MySQL 9.7 VECTOR storage only, distance queries return ErrUnsupported (DISTANCE() is HeatWave/MySQL AI only); MySQL 8.4 has no vector columns; CI uses official MySQL and MariaDB service containers.
- Optional pure-Go SQLite: modernc.org/sqlite (pinned), without vector support.
- PostgreSQL: supported versions to be recorded (tested in CI against real PostgreSQL with pgvector, image `pgvector/pgvector:pg17` locally available; plus one older major).
- SQLite: >= 3.35 (RETURNING); the adapter checks the library version on connect.
- Generated execution path performs no struct-tag inspection, no reflection over models and no name-based field resolution per query.
- Generator and migration tooling dependencies (golang.org/x/tools etc.) stay out of the application runtime import graph; benchmark comparison ORMs live in a separate Go module.
- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- License: Apache-2.0.
- Module path: github.com/azrtydxb/momo; public repository https://github.com/azrtydxb/momo.
- CI: GitHub Actions on free GitHub-hosted runners only (public repo): ubuntu-latest for most jobs, windows-latest for the SQLite/generator suite, PostgreSQL+pgvector as a `pgvector/pgvector` service container, SQLite+sqlite-vec via cgo on the runner; a macos-latest job builds and runs the SQLite suite to cover darwin/cgo. No self-hosted or paid larger runners. Benchmarks published as release evidence are run on a documented dedicated machine, because shared runners are too noisy for regression gating; CI only smoke-runs benchmarks.
- Versioning: standard Go module SemVer. v0.x carries no compatibility promise (breaking changes bump the minor, fixes the patch); v1.0.0 is cut when the API is declared stable; after v1, minor = backward-compatible features, patch = fixes, and any breaking change requires v2.0.0 with a `/v2` module path suffix. The first tag, v0.1.0, is cut only when all delivery stages 1-9 are complete; intermediate milestones are untagged.
- Release automation: release-please (GitHub Action, free runners) reads Conventional Commits on main, maintains a release PR with the version bump and CHANGELOG.md, and tags vX.Y.Z on merge; pre-1.0 it is configured so `feat!`/BREAKING CHANGE bumps the minor, not the major. Commits on main follow Conventional Commits (enforced on PR titles in CI).
- Documentation site: https://azrtydxb.github.io/momo/ served by GitHub Pages from a `gh-pages` branch maintained by mike; MkDocs Material and mike are pinned in a docs requirements file and used only by the docs workflow (no Python in the Go module or library CI). Releases (release-please tags) publish `/vX.Y/` and move `/latest/`; pushes to main publish `/dev/`.
- CLI distribution: `go install github.com/azrtydxb/momo/cmd/momo@vX.Y.Z` only, so the CLI and library share one version; no prebuilt binaries.
- Persisted formats carry explicit format versions independent of the module version: schema snapshot JSON (`format_version`), the checksum file header, the `momo_migrations` history table (metadata row), and generated-file headers (generator version). A newer Momo reads every older format; an older Momo refuses a newer format with an actionable error instead of misreading it.

## Interfaces

**Go API (runtime, package `momo`)** — prototyped on branch `init`, to be confirmed:

- `momo.New` wraps a driver and returns a DB handle; DB and Tx handles implement the sealed Querier interface.
- Generic Select over a model type T, taking a Querier, returns an immutable `SelectQuery[T]` with `Where`, `Join/LeftJoin/CrossJoin`, `Columns`, `GroupBy`, `Having`, `OrderBy`, `Limit`, `Offset`, `After` (keyset), `With/WithRecursive`, `ForUpdate/ForShare`, `WithDeleted`, `SQL()`; terminals `All`, `One` (ErrNotFound / ErrTooManyRows), `First`, `Each`, `Iter` (iter.Seq2), `Count`, `Exists`.
- Insert over T: `.Values(assignments...)` / `.Model(&m)` / `.Models(ms)` / `.Omit(cols...)` / `.OnConflict(cols...).DoNothing()|DoUpdate(assignments...)` / `.Returning...`; Update over T: `Set` with `Where`, `Model` (version-checked), or `AllRows`; Delete and SoftDelete over T.
- Typed columns: `Column[T]` (`Eq/Ne/Lt/Le/Gt/Ge/In/Between/IsNull/...`, `Set/SetExpr/SetDefault/SetExcluded`), `NullColumn[T]` (adds `SetNull`), vector columns with distance expressions.
- Raw SQL: `momo.SQL(fragment, args...)`, `momo.TypedSQL[T]`, `momo.Exec`, raw typed query — `?` placeholders, `??` for a literal `?`.
- Transactions: `db.Tx(ctx, opts, fn)`, `db.Begin`, `tx.Tx` (savepoint scope), `Savepoint/RollbackTo/Release`.
- Relations: `momo.Load(ctx, q, parents, relation)` and query-level `.With(relation)`; one documented strategy (batched IN-list queries).
- Hooks: `momo.Hook` interface (`BeforeQuery/AfterQuery` with `QueryEvent`), `WithArgLogging()`.
- Errors: sentinels + `*momo.Error{Kind, Op, Table, Column, Constraint, Err}`.

**Model declaration** — an ordinary struct embedding the zero-size `momo.Model` marker whose tag carries table options (`momo:"table:documents"`); field options in `momo:"..."` tags; complex options (composite/expression/partial/vector indexes, checks, enums, defaults containing commas) in a `MomoConfig(c *momo.Config)` method on the model that the generator reads statically from the AST and never executes. Only literal arguments and field references of the form `&d.Field` are accepted; anything else is a diagnostic.

**Generated code** — in the model package: one file with a `MomoMeta()` method per model (scan/bind functions, schema metadata). Per model, a subpackage named after the model (e.g. `models/document`) with typed column descriptors, an `As(alias)` constructor for aliased/self joins and relation descriptors, used as `document.TenantID.Eq(x)`. Every file starts with `// Code generated by momo. DO NOT EDIT.`

**CLI `momo`**

| Command                                                                                       | Needs DB           | Contract                                                                                                                                                        |
| --------------------------------------------------------------------------------------------- | ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `momo generate`                                                                               | no                 | regenerate code for configured/current packages; refuses to overwrite non-generated files                                                                       |
| `momo generate --check`                                                                       | no                 | exit 1 and list files if output would change                                                                                                                    |
| `momo migrate diff <name> [--rename-table a=b] [--rename-column t.a=b] [--allow-destructive]` | no                 | diff models vs latest snapshot, write `<timestamp>_name.up.sql` (+ snapshot, + checksum entry); refuses destructive changes without the flag                    |
| `momo migrate plan`                                                                           | yes                | print pending migrations and their SQL; changes nothing                                                                                                         |
| `momo migrate apply [--to N]`                                                                 | yes                | apply pending under lock; refuses if dirty or checksums mismatch                                                                                                |
| `momo migrate status`                                                                         | yes                | applied / pending / dirty / mismatched per version                                                                                                              |
| `momo migrate validate [--db]`                                                                | no (yes with --db) | offline: checksums, numbering, snapshot chain; with --db also history vs files                                                                                  |
| `momo migrate baseline --version N`                                                           | yes                | record 1..N as applied without running, after verifying schema matches snapshot N (override with --force)                                                       |
| `momo migrate drift`                                                                          | yes                | inspected DB vs latest applied snapshot; reports changed, missing and unmanaged objects; exit 1 on drift                                                        |
| `momo migrate rebase [--onto <branch-or-version>]`                                            | no                 | regenerate this branch's unapplied migrations on top of the latest snapshot of the target, refusing if they contain hand-written SQL that cannot be regenerated |
| `momo migrate lint`                                                                           | no                 | report blocking/rewriting operations in pending migrations with severity and safer alternatives; exit 1 on errors                                               |
| `momo migrate resolve --version N --applied\|--rolled-back`                                   | yes                | operator records manual recovery of a dirty migration                                                                                                           |

**Files**: `migrations/<dialect>/<YYYYMMDDHHMMSS>_<name>.up.sql` (UTC timestamp version), optional `.down.sql`, `<version>_<name>.snapshot.json` recording its parent version, a checksum file (named momo.sum). SQL file directives: `-- momo:no-transaction`, `-- momo:destructive`. Project config: a YAML file named momo.yaml at the module root ( parsed with go.yaml.in/yaml/v3) listing model packages, per-dialect migration directories, database URL environment variables and unmanaged-object exclude patterns; CLI flags override it.

## Data

- **Model metadata**: generated Go code (package-level values); immutable at runtime.
- **Schema snapshot**: JSON serialization of `schema.Schema` per migration, committed to source control; the diff baseline independent of current models.
- **Checksums**: a momo.sum file per migration directory — SHA-256 of each migration file plus a directory hash; committed.
- **History table** `momo_migrations` in the target DB: version, name, checksum, status (`running`/`applied`/`failed`), statements_applied, error, applied_at, execution_ms. Owned by Momo.
- **Lock**: PostgreSQL session advisory lock; SQLite `momo_migration_lock` single-row table.
- **Benchmark results**: raw `go test -bench` output plus environment files under `bench/results/`.

## Edge cases

- Go zero values vs omitted vs NULL vs DEFAULT on insert and update.
- Empty IN lists (render always-false/true, not invalid SQL); empty multi-row results (empty slice, not ErrNotFound).
- Bind-parameter limits (PostgreSQL 65535, SQLite 32766) in bulk insert and eager loading.
- Composite primary and foreign keys; NULL foreign keys in relation loading; duplicate parent keys.
- Outer-join NULLs scanned into non-nullable fields must error, not yield zero values.
- Identifiers that are reserved words, contain quotes, or two Go names normalizing to the same identifier/package name.
- Models in a package other than main; import aliases; build-tag-constrained files; custom named types and codecs.
- Keyset pagination with mixed sort directions and non-unique sort keys.
- Query reuse: extending a base query concurrently must not leak predicates.
- Context cancelled mid-iteration; callback returning an error mid-stream; panic inside a transaction callback.
- SQLite ALTER limits requiring rebuild; FK enforcement during rebuild; triggers and views referencing a rebuilt table.
- PostgreSQL statements that cannot run in a transaction (CREATE INDEX CONCURRENTLY, ALTER TYPE ... ADD VALUE on older versions).
- Normalization of DB-rewritten expressions (casts, parentheses) so round-trip diffs are empty.
- Vector dimension mismatch, NaN/Inf values, sparse index bounds, halfvec range.
- Unknown DB objects (tables, triggers, functions) not described by models.
- Concurrent `migrate apply` from two processes.

## Failure modes

- **Database unreachable / slow**: operations return the driver error wrapped; context deadlines respected; no hidden retries.
- **Constraint violation**: classified `*momo.Error` with constraint/table when the driver reports them; original error reachable via errors.As.
- **Migration fails mid-way (transactional)**: transaction rolled back, history row records `failed` with the error; DB unchanged.
- **Migration fails mid-way (non-transactional)**: history row stays `failed` with statements_applied; status shows dirty; apply refuses until `migrate resolve` after manual repair (documented procedure).
- **Lock held by another process**: apply waits up to a timeout then fails with a lock error naming the holder; stale SQLite lock cleared only by explicit `--force-unlock`.
- **Checksum mismatch**: apply/validate fail listing the file; never auto-rewrite history.
- **Extension missing (pgvector / sqlite-vec)**: clear unsupported-capability error at migration or first vector query, not a silent fallback.
- **Generator input invalid**: diagnostics with file:line:col; no partial output written.
- **Unsupported dialect feature requested**: ErrUnsupported at build time, never silently dropped.

## Acceptance criteria

- [ ] [S-1] `TestSchemaJSONRoundTrip` and `FuzzSchemaSort` (schema package): JSON round-trips losslessly and `Sort` makes equivalent schemas byte-identical; fails if a field is dropped from serialization or Sort is order-sensitive.
- [ ] [S-2] [S-3] `TestGenerateCompanion` (generator golden test): a model package with tags and a MomoConfig companion generates code that `go build` accepts; `TestGenerateDiagnostics` asserts an unsupported companion expression yields file:line:col and writes no files; fails if the generator guesses or emits partial output.
- [ ] [S-3] `TestGenerateDeterministic` runs `momo generate` twice and compares bytes and gofmt output; `TestGenerateCheck` asserts `momo generate --check` exits 1 after a model edit and 0 when current; fails if map iteration or timestamps leak into output.
- [ ] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [ ] [S-4] `TestCompileFail` (testdata/compilefail fixtures built with `go build`): comparing a typed column with a wrong-typed value fails to compile; `TestValidExamplesCompile` builds the examples; fails if Column methods accept `any`.
- [ ] [S-4] `TestQueryReuseNoLeak` under `go test -race`: deriving two queries from one base, concurrently, leaves base and derived SQL unchanged; fails if a builder method appends into a shared backing array.
- [ ] [S-4] `TestPlaceholdersAndQuoting` and `FuzzBuilder`: `$n` numbering on PostgreSQL, `?` on SQLite, identifiers with quotes/reserved words quoted; fails if a value is ever interpolated into SQL text.
- [ ] [S-4] `TestIntegrationQueries` (run per dialect by `make test-integration`): aliased/self joins, projections, aggregates, subqueries, recursive CTE, keyset pagination, locking, returning and raw SQL each return asserted rows, or ErrUnsupported where the capability matrix says so; fails if a dialect silently drops a clause.
- [ ] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [ ] [S-5] `TestWriteStates` (both dialects): omitted, zero, NULL and DEFAULT writes yield four distinct stored values; fails if any write path infers omission from a Go zero value.
- [ ] [S-5] `TestMutationGuardrails`, `TestOptimisticLock`, `TestSoftDelete`: unfiltered UPDATE/DELETE returns ErrUnfilteredMutation unless AllRows; a stale version returns ErrOptimisticLock; soft-deleted rows hidden by default and returned by WithDeleted; fails if any guard is bypassed.
- [ ] [S-5] [S-9] `TestBulkInsertChunking` inserts more rows than one statement's parameter limit and counts them; `TestCopyInsert` (PostgreSQL) inserts identical data via COPY; fails if chunk size ignores the dialect limit.
- [ ] [S-6] `TestEagerLoad*` (one per relation kind incl. composite keys): a counting hook asserts the exact query count, chunking over the bind limit, parent pagination and order preserved without duplicates, NULL-FK parents left empty; fails if loading issues per-parent queries or joins that duplicate parents.
- [ ] [S-7] `TestTransactions`: callback commits on nil, rolls back on error and on panic (panic re-raised, row absent afterwards), nested savepoint rollback keeps outer work, isolation reaches PostgreSQL (`SHOW transaction_isolation`) and is ErrUnsupported on SQLite for non-default levels; fails if a panic leaves the transaction open.
- [ ] [S-8] `TestErrorClassification` (both dialects): each error kind matches errors.Is and the driver error is reachable with errors.As (*pgconn.PgError, sqlite3.Error); `TestEmptyResultNotNotFound` asserts All on zero rows returns nil error; fails if classification loses the cause.
- [ ] [S-10] [S-11] `TestMigrationRoundTrip` (both dialects, full example schema): apply generated migrations to an empty DB, inspect, diff against models: zero changes; fails if normalization misses a DB-rewritten form.
- [ ] [S-10] `TestCLIContracts`: each command's exit codes and outputs as documented in Interfaces; offline commands run with no database URL set; fails if an offline command opens a connection.
- [ ] [S-11] `TestNoRenameGuessing` and `TestDestructiveRequiresFlag`: a dropped+added column produces drop/add plus a rename hint, never a rename, and `migrate diff` exits 1 on destructive changes without `--allow-destructive`; fails if diff infers renames.
- [ ] [S-11] `TestChecksumMismatch`: editing an applied migration is reported by `migrate validate` offline and with `--db`; apply refuses; fails if checksums are recomputed silently.
- [ ] [S-11] `TestConcurrentApply`: two goroutines/processes applying the same directory result in each version applied exactly once (history count); fails if the lock is per-connection only.
- [ ] [S-11] `TestNonTransactionalFailureRecovery`: a failing `-- momo:no-transaction` migration leaves a failed history row with statements_applied, apply refuses, `migrate resolve` clears it; fails if a failure is recorded as applied.
- [ ] [S-11] `TestBaselineAndDrift`: baseline records history without executing SQL (statement-count hook = 0 DDL); drift reports an altered column and lists an unmanaged table without proposing a drop; fails if unmanaged objects appear in a generated plan.
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] [S-12] `TestPostgresObjects` on real PostgreSQL+pgvector: vector extension, enum, partial index, expression index, HNSW index with opclass and a CONCURRENTLY index apply and round-trip with no diff; fails if CONCURRENTLY runs inside a transaction.
- [ ] [S-13] `TestSQLiteRebuild`: a rebuild-requiring change preserves row count and content, indexes and triggers, and `PRAGMA foreign_key_check` returns no rows; fails if the rebuild drops a trigger or index.
- [ ] [S-14] `TestAutoMigrateAdditive`: AutoMigrate creates missing tables/columns/indexes, returns skipped changes for a type change and a removed column, and the column still exists afterwards; fails if AutoMigrate ever emits DROP or ALTER TYPE.
- [ ] [S-15] `TestVectorCodec` + `FuzzVectorDecode`: vector, halfvec, sparsevec and bit round-trip; wrong dimension returns ErrInvalidVector before any SQL runs; `TestVectorDistances` matches independently computed values to 1e-5; `TestVectorIndexUsed` asserts EXPLAIN shows the HNSW/IVFFlat index; `TestSearchSettingsScoped` asserts `hnsw.ef_search` resets after the transaction; fails if settings leak to the pooled connection.
- [ ] [S-16] `TestSQLiteVectorParity`: sqlite-vec distance queries return the same top-k ids as pgvector exact search on the same data; requesting an ANN index on SQLite returns ErrUnsupported; fails if SQLite silently accepts an index option.
- [ ] [S-17] `TestHybridSearch`: a fixed corpus yields the asserted ordering with documented candidate counts and RRF k; fails if fusion is order-dependent on ties.
- [ ] [S-18] `BenchmarkSelectNoHooks` allocations stay within the budget recorded in docs/PERFORMANCE.md; `TestHooks` asserts SQL, duration, rows and errors are reported and args only with WithArgLogging; `TestExplainAnalyzeExplicit` asserts plain Explain never executes the query; fails if disabled hooks allocate.
- [ ] [S-19] `make bench` reproduces published results; bench/results contains raw output, environment and pinned versions; `TestBenchEquivalence` asserts each compared ORM returns identical rows per workload; fails if a workload's SQL semantics differ between ORMs.
- [ ] [S-20] `TestCIWorkflowCoverage` parses .github/workflows/ci.yml and asserts it runs unit, generator, compile-fail, race and fuzz-smoke tests, plus integration tests against pgvector/pgvector:pg17 and SQLite+sqlite-vec, on Go 1.26 and 1.27; fails if any job is skipped.
- [ ] [S-21] Every example runs end to end against PostgreSQL in CI through its own `example_test.go` (decision 2026-11-10: the examples demonstrate PostgreSQL features, so they are PostgreSQL only); docs/ contains architecture, capability matrix, migration safety and recovery guide, contributor guide and supported versions; fails if an example is not wired into the test.
- [ ] [S-22] `TestReleaseConfig` parses release-please-config.json and asserts the pre-1.0 minor-bump setting and the changelog path; `TestFormatVersions` asserts that a snapshot, checksum file and history table from format N-1 are read by the current version and that a future format version fails with an error naming the required Momo version; fails if a persisted format lacks its version field.
- [ ] [S-23] `TestDocsCodeBlocks` extracts every Go code block from the docs and compiles it against the current module (runnable ones are executed against PostgreSQL, the database the blocks are written for; decision 2026-11-10); `TestLLMsTxt` builds the site and asserts `/llms.txt` lists every nav page (a release deploy also copies it to the site root; decision 2026-11-10), `/llms-full.txt` contains every page's Markdown, and each page has a raw `.md` twin; the docs workflow deploys `/dev/` on push to main and `/vX.Y/` plus `/latest/` on a release tag; fails if a documented API symbol no longer compiles or a page is missing from llms.txt.

- [ ] [S-24] `TestRepoFiles` asserts LICENSE (Apache-2.0 text), SECURITY.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md and .github issue/PR templates exist and SPDX headers are present in Go files; fails if a file is missing.
- [ ] [S-25] `TestWorkflowsPinned` parses .github/workflows and asserts every third-party `uses:` is pinned by a 40-character SHA and every workflow declares `permissions:`; CI runs govulncheck and the Scorecard workflow; fails if an action is referenced by tag.
- [ ] [S-26] The windows-latest CI job runs `go test ./...` for the generator, CLI and SQLite+sqlite-vec packages; `TestWindowsPaths` asserts generated imports and migration paths use forward slashes; fails if the job is skipped.
- [ ] [S-27] `TestMigrationBranchConflict`: two migrations with the same parent snapshot fail `momo migrate validate`; `TestMigrateRebase` regenerates the later one on top of the other and the result applies with a no-change round trip; fails if two heads are accepted.
- [ ] [S-28] `TestMigrateLint` asserts each listed risky operation is reported with severity and an alternative and that `momo migrate lint` exits 1 on errors; `TestLockTimeoutApplied` asserts `SHOW lock_timeout` inside an applying migration equals the configured value; fails if apply runs without timeouts on PostgreSQL.
- [ ] [S-29] `TestFilteredANNIterativeScan`: a selective filter with HNSW returns fewer than k rows with iterative scan off and k rows with it on, settings reset after the transaction; fails if the setting leaks to the pool.
- [ ] [S-30] `TestPgBouncerTransactionMode` runs the query suite through a PgBouncer container in transaction mode with the simple-protocol option; fails if any statement relies on a named prepared statement.
- [ ] [S-31] `TestTypeMapping` round-trips one value (including edge values: max int64, NaN where legal, pre-1970 time, non-UTC zone) per mapping in the documented table on both dialects; fails if a documented mapping is untested.
- [ ] [S-32] `TestJSONAndArrayOperators` asserts each operator's SQL and result rows on PostgreSQL and the SQLite equivalent or ErrUnsupported; fails if a value is interpolated rather than bound.
- [ ] [S-33] `TestWindowFunctions` asserts ranks per partition; `TestEagerLoadTopNPerParent` asserts at most N children per parent in one query (query-count hook); fails if the strategy falls back to per-parent queries.
- [ ] [S-34] `TestMomotestTxRollback` asserts data written in a momotest transaction is absent after cleanup; `TestMomotestSQLAssert` asserts the helper fails on SQL mismatch; fails if cleanup commits.
- [ ] [S-35] `TestOtelSpans` (otelmomo module) asserts span names, `db.system.name` (current OpenTelemetry database semantic conventions), `db.query.text` without parameter values, and error status via an in-memory exporter; `go list -deps` on the core module shows no OpenTelemetry package; fails if the core imports OTel.
- [ ] [S-36] `TestPostgresSchemas`: models in two non-public schemas migrate, query and round-trip with no diff, and drift reports objects per schema; fails if an identifier is not schema-qualified.
- [ ] [S-37] `TestViewsAsModels`: a view and a materialized view are created by migrations, queried as read-only models, refreshed by the helper; mutation builders on them do not compile; fails if a view is dropped before its dependents.
- [ ] [S-38] `TestRowLevelSecurity`: with a policy applied by migration, a transaction scoped to tenant A cannot read tenant B's rows even through raw SQL, and the setting does not persist on the pooled connection; fails if `set_config` is session-scoped.
- [ ] [S-39] `TestManagedFunctionsTriggersPartitions`: a function, trigger and partitioned table are created by migrations, round-trip with no diff, and a changed function body produces CREATE OR REPLACE; fails if they are reported as unmanaged.
- [ ] [S-40] `TestReadReplicaRouting`: queries on the read handle reach the replica driver (driver-identity hook), queries inside a transaction never do; fails if a read inside a transaction is routed to the replica.

- [ ] [S-41] `TestErrorRedaction`: a unique violation on an email column returns an error whose Error() lacks the email while the detail accessor and errors.As expose it; fails if driver detail is concatenated into the message.
- [ ] [S-42] `TestSensitiveColumns`: with argument logging on, hook events, error text and momotest dumps contain a placeholder instead of a sensitive column's value; fails if any path prints it.
- [ ] [S-43] `TestColumnByName` rejects unknown names and returns the typed column for known ones; `TestEscapeLike` matches literal `%`, `_` and the escape character on both dialects; fails if a name is ever quoted into SQL without the allowlist.
- [ ] [S-44] `TestMomovet` (analysistest fixtures) reports non-constant SQL arguments and accepts constants; fails if a concatenated string passes.
- [ ] [S-45] `TestSQLiteNoLoadExtension` asserts `SELECT load_extension('x')` fails on a Momo SQLite connection while vec functions work; fails if extension loading is enabled.
- [ ] [S-46] `TestCLIRedactsURL` runs failing CLI commands with a password-bearing URL and asserts the password never appears in output; `TestCLITLSWarning` asserts the warning for sslmode=disable on a remote host; fails if a URL is echoed.
- [ ] [S-47] `TestAppRoleCannotDDL` runs in integration CI, which connects as a non-superuser application role; `TestAppRoleCannotDDL` asserts DDL fails for it; fails if tests run as superuser.
- [ ] [S-48] `TestMaxRowsGuard` and `TestInListCap` return structured errors beyond the configured limits; fails if limits are silently truncated.
- [ ] [S-49] `TestWorkflowsPinned` also asserts CodeQL and gosec workflows exist and run on pull requests; fails if either is absent.
- [ ] [S-50] `TestVectorBinaryCodec` round-trips all four types in binary mode and matches text-mode results; `BenchmarkVectorEncoding` results recorded under bench/results; fails if binary and text decode differently.
- [ ] [S-51] `TestPipelinedEagerLoad` asserts two relations load in one round trip (driver round-trip counter) with identical results to sequential loading; fails if results differ.
- [ ] [S-52] `TestUnnestBulkInsert` inserts beyond the bind limit in one statement with identical rows to VALUES and COPY paths; fails if any path loses or reorders data.
- [ ] [S-53] `TestNPlusOneHook` reports a loop of per-parent queries and stays silent for batched loading; fails if the detector misses the loop.
- [ ] [S-54] `TestSlowQueryHook` reports only queries over the threshold and captures a plain EXPLAIN plan only when enabled; fails if EXPLAIN ANALYZE ever runs.
- [ ] [S-55] `TestAssertUsesIndex` passes for an indexed lookup and fails for a sequential scan; fails if the helper ignores the plan.
- [ ] [S-56] `TestStreamingBoundedMemory` iterates one million rows with Each and asserts heap growth stays under a fixed bound; fails if rows are buffered.
- [ ] [S-57] `TestSQLiteRecommendedConfig` asserts WAL, busy_timeout and synchronous settings on adapter connections and that concurrent writers yield the structured busy error or succeed within busy_timeout; fails if SQLITE_BUSY surfaces unclassified.
- [ ] [S-58] `TestBenchGateConfig` asserts the CI benchmark job runs benchstat against the stored baseline and fails on allocation-count regressions beyond the documented threshold; fails if the job only reports timings.

- [ ] [S-59] `TestMySQLIntegration` runs the shared query/write/transaction/error suites against MySQL and MariaDB containers and asserts identical results to PostgreSQL or ErrUnsupported per the capability matrix; fails if a clause is silently dropped.
- [ ] [S-60] `TestMySQLMigrationRoundTrip` applies the example schema on both servers with a no-change round trip, and `TestMySQLPartialFailure` asserts a mid-file failure records statements_applied; fails if a MySQL migration is reported transactional.
- [ ] [S-61] `TestMariaDBVectorParity` returns the same top-k ids as pgvector exact search; `TestMySQLVectorCapability` asserts distance queries are ErrUnsupported where the server lacks distance functions; fails if a full scan is substituted silently.
- [ ] [S-62] `TestGeneratorPlugin` runs a fixture plugin and asserts deterministic output and refusal to overwrite handwritten files; fails if plugin output bypasses the writer.
- [ ] [S-63] `TestGraphQLGenerator` generates for the example models, builds a gqlgen server and asserts a query with filter, pagination and nested relation returns expected data with the batched query count; fails if nested fields cause per-row queries.
- [ ] [S-64] `TestProtoGenerator` asserts generated .proto compiles with protoc, a CRUD round trip over an in-process gRPC server succeeds, and renaming a field keeps its number; fails if field numbers shift.
- [ ] [S-65] `TestOpenAPIGenerator` validates the document against the OpenAPI 3.1 schema and exercises generated handlers with httptest; fails if the document and handlers disagree on a field.
- [ ] [S-66] `TestViz` asserts golden Mermaid and DOT output for the example schema; fails if a relation or table is omitted.
- [ ] [S-67] `TestEmbeddedAndMixins` (generator golden + integration) asserts prefixed columns and mixin indexes exist and round-trip; fails if an embedded field is skipped.
- [ ] [S-68] `TestAutoTimestamps` asserts insert sets both, update sets only updated, an explicit Set wins, and the injected clock is used; fails if updated_at changes on a no-op guarded update that matched no row.
- [ ] [S-69] `TestNamingPresets` asserts each preset's names for a fixture set match the names those ORMs generate (recorded fixtures); fails if a preset diverges.
- [ ] [S-70] `TestTagVocabulary` asserts each tag reaches the schema, migration SQL and inspection round trip on every dialect or yields a dialect diagnostic; fails if a tag is ignored silently.
- [ ] [S-71] `TestFieldPermissions` plus compile-fail fixtures assert Set on a read-only/immutable column does not compile and create-only columns are rejected in updates; fails if any path writes them.
- [ ] [S-72] `TestDefaultsAndValidators` asserts defaults apply only to omitted fields, validators reject invalid values with a structured validation error before any SQL runs (statement-count hook = 0); fails if validation runs after execution.
- [ ] [S-73] `TestCodecsRoundTrip` round-trips each type on every dialect; fails if a codec's SQL type and encoding disagree.
- [ ] [S-74] `TestFKActions` asserts each action's effect on every dialect and that the no-constraint option produces no FK in inspection; fails if actions are lost in a round trip.
- [ ] [S-75] `TestSoftDeleteVariants` asserts each marker kind hides rows by default, OnlyDeleted and ForceDelete behave as documented and a unique index permits re-creating a soft-deleted key; fails if ForceDelete soft-deletes.
- [ ] [S-76] `TestRelationPredicates` asserts SQL shape (EXISTS) and rows for each; fails if a predicate duplicates parents via JOIN.
- [ ] [S-77] `TestRawArgs` and `FuzzRawArgs` assert named and expanded placeholders bind correctly per dialect and identifiers are quoted; fails if an identifier argument is bound as a value or a value is interpolated.
- [ ] [S-78] `TestResultHelpers` asserts each helper's results and that Batches visits every row exactly once under concurrent inserts of higher keys; fails if Batches uses OFFSET.
- [ ] [S-79] `TestCompositionHelpers` asserts each renders correctly or returns ErrUnsupported per dialect and the filter tree rejects unknown columns; fails if an override bypasses identifier quoting.
- [ ] [S-80] `TestMergeAndBulkUpdate` asserts row results on supporting dialects and ErrUnsupported elsewhere; fails if bulk update touches rows outside the VALUES list.
- [ ] [S-81] `TestNestedFilteredEagerLoad` and `TestJoinLoadToOne` assert query counts and that a missing to-one leaves nil, not a zero struct; fails if join loading duplicates parents.
- [ ] [S-82] `TestPolymorphicAndThrough` asserts correct association per type discriminator and join-row attributes; fails if a polymorphic load mixes types.
- [ ] [S-83] `TestAssociationHelpers` asserts results and statement count per helper; fails if a helper issues an undeclared statement.
- [ ] [S-84] `TestCreateGraphAndFindOrCreate` asserts only listed relations are written, FindOrCreate is race-safe under concurrency (ON CONFLICT based), and Upsert updates all columns; fails if an unlisted relation is saved.
- [ ] [S-85] `TestModelHooks` asserts invocation order, error abort before SQL, and zero reflection in the call path (benchmark allocation check); fails if a hook runs for a builder that did not opt in.
- [ ] [S-86] `TestInterceptors` asserts an interceptor can add a predicate and deny a mutation, applies only to its handle, and is visible in compiled SQL; fails if an interceptor registered on one handle affects another.
- [ ] [S-87] `TestSlogHook` asserts log records contain SQL, duration and rows and never parameter values unless enabled; fails if params leak.
- [ ] [S-88] `TestPostgresSQLDBAdapter` runs the PostgreSQL suite through *sql.DB; fails if any capability differs without being listed in the matrix.
- [ ] [S-89] `TestSQLitePureGo` runs the SQLite suite with CGO_ENABLED=0 and asserts vector queries return ErrUnsupported; fails if the build requires cgo.
- [ ] [S-90] `TestPinnedConn` asserts session state set on a pinned connection is visible to later statements on it and the pool stats reflect acquisition; fails if statements hop connections.
- [ ] [S-91] `TestImportGORM`, `TestImportBun`, `TestImportEnt` convert fixture projects, assert generated Momo models produce the same schema as the source ORM's migration (inspected) with zero diff, and assert the report lists each semantic change; fails if a source field is dropped silently.
- [ ] [S-92] `TestGenerateFromDB` inspects the example database, generates models, and asserts they diff to zero against it; fails if a column type is unmapped silently.
- [ ] [S-93] `TestHistoryImporters` imports each fixture history, asserts status shows all versions applied and drift is zero; fails if an applied migration would re-run.
- [ ] [S-94] `TestGoMigrations` asserts a registered Go migration runs once under the lock with history recorded, the split directive splits statements, and group rollback runs authored downs in reverse; fails if a Go migration bypasses history.
- [ ] [S-95] `TestDocsCodeBlocks` covers the guide pages and `TestMigrationGuidesComplete` asserts each guide has sections for models, queries, relations, hooks, transactions, migrations and errors; fails if a guide section is missing.

## Open questions
