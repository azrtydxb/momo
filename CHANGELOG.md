# Changelog

## [0.5.0](https://github.com/azrtydxb/momo/compare/v0.4.0...v0.5.0) (2026-11-10)

_Momo now carries everything Kuvryn Orbit needs to move onto it: composite foreign keys, DML in CTEs, raw multi-column queries, database-defaulted columns and more row-lock modes. It also adds hybrid search, EXPLAIN and slow-query tooling, OpenTelemetry in the new `otelmomo` module, and a `cli` package for building your own `momo` binary. Upgrading needs a regenerate and one advisory call-site check: follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.5.0/docs/upgrading.md). [Changes since 0.4.0](https://github.com/azrtydxb/momo/compare/v0.4.0...v0.5.0)._

### ⚠ BREAKING CHANGES

- **gen:** `momo generate --from-db` adds the `dbdefault` tag to NOT NULL PostgreSQL columns with a computed default. Regenerated models stop writing Go zero values to those columns and read the database value back with `RETURNING` ([#69](https://github.com/azrtydxb/momo/issues/69))
- **gen:** `momo generate` wires methods named `BeforeInsert`, `BeforeUpdate`, `BeforeDelete` and `AfterLoad` into `Meta.Hooks`. A hook-named method with another signature now fails generation ([#70](https://github.com/azrtydxb/momo/pull/70))
- **advisory:** `advisory.XactLockString` and `advisory.TryXactLockString` take a trailing `...advisory.Option`, so assigning them to a variable of the old func type no longer compiles ([#67](https://github.com/azrtydxb/momo/issues/67))
- **cli:** the command line moved from `cmd/momo` into package `cli`. `go install github.com/azrtydxb/momo/cmd/momo@v0.5.0` still works ([#70](https://github.com/azrtydxb/momo/pull/70))

### Features

- **schema:** composite foreign keys through `Config.ForeignKey` with `ON DELETE` and `ON UPDATE` actions, unique expression indexes, `identity:always`, named foreign keys (`fk_name`, `Config.FKName`) and an explicit primary key column order (`Config.PrimaryKey`) ([#54](https://github.com/azrtydxb/momo/issues/54), [#55](https://github.com/azrtydxb/momo/issues/55), [#56](https://github.com/azrtydxb/momo/issues/56), [#59](https://github.com/azrtydxb/momo/issues/59), [#60](https://github.com/azrtydxb/momo/issues/60))
- **types:** `[]types.UUID` maps to `uuid[]` and works with the array operators; `tsvector` maps to `types.TSVector` in `--from-db` ([#57](https://github.com/azrtydxb/momo/issues/57), [#61](https://github.com/azrtydxb/momo/issues/61))
- **query:** `UpdateQuery.From`, `DeleteQuery.Using`, and `With` / `WithRecursive` on INSERT, UPDATE and DELETE, so a DML statement can be a CTE body on PostgreSQL ([#64](https://github.com/azrtydxb/momo/issues/64))
- **query:** raw multi-column queries with `momo.Query` and `momo.QueryRows`, and a query registry for listing and freezing SQL: `momo.RegisterQuery`, `momotest.QueryGolden` ([#63](https://github.com/azrtydxb/momo/issues/63), [#66](https://github.com/azrtydxb/momo/issues/66))
- **query:** `ForNoKeyUpdate` and `ForKeyShare` row locks, session advisory locks, `advisory.Hash64`, `timeop.ClockTimestamp` and `timeop.StatementTimestamp` ([#65](https://github.com/azrtydxb/momo/issues/65), [#67](https://github.com/azrtydxb/momo/issues/67), [#68](https://github.com/azrtydxb/momo/issues/68))
- **insert:** the `dbdefault` tag leaves a zero-valued column to its database default and reads the value back ([#69](https://github.com/azrtydxb/momo/issues/69))
- **vectors:** `momo.HybridSearch` combines full-text and vector ranking ([#70](https://github.com/azrtydxb/momo/pull/70))
- **observability:** `momo.Explain`, `momo.ExplainAnalyze`, a slow-query hook (`obs.NewSlowQuery`), and OpenTelemetry spans and a duration histogram in the separately released `otelmomo` module ([#70](https://github.com/azrtydxb/momo/pull/70))
- **momotest:** `momotest.AssertSQL` and `momotest.AssertUsesIndex` ([#70](https://github.com/azrtydxb/momo/pull/70))
- **migrate:** `momo migrate lint`, `rollback`, `rebase` and `drift --strict`, and history import from Atlas, goose, golang-migrate and Bun ([#70](https://github.com/azrtydxb/momo/pull/70))
- **cli:** build a custom `momo` binary with generator plugins through `cli.Main` ([#70](https://github.com/azrtydxb/momo/pull/70))
- **models:** view models: embed `momo.View`, and every write path fails with `ErrValidation` ([#70](https://github.com/azrtydxb/momo/pull/70))

### Bug Fixes

- **gen:** trigger `UPDATE OF` keeps its written column order, and `momo generate` no longer fails on its own stale `momo_gen.go` ([#58](https://github.com/azrtydxb/momo/issues/58), [#62](https://github.com/azrtydxb/momo/issues/62))
- **sqlite:** a NaN float is rejected with `ErrValidation` instead of being stored ([#70](https://github.com/azrtydxb/momo/pull/70))

## [0.4.0](https://github.com/azrtydxb/momo/compare/v0.3.0...v0.4.0) (2026-11-10)

_Momo now runs on MySQL, MariaDB and SQLite as well as PostgreSQL, and the root package is split into feature subpackages whose docs are checked against the code. Upgrading needs import changes: follow [docs/upgrading.md](https://github.com/azrtydxb/momo/blob/v0.4.0/docs/upgrading.md). [Changes since 0.3.0](https://github.com/azrtydxb/momo/compare/v0.3.0...v0.4.0)._

### ⚠ BREAKING CHANGES

- **packages:** feature APIs moved out of the root `momo` package into `arrayop`, `fts`, `sqlfn`, `jsonop`, `agg`, `timeop`, `advisory`, `vecsearch`, `vecop` and `obs`; advisory locks are free functions taking `*momo.Tx`. `docs/upgrading.md` lists every moved identifier with its replacement and a rewrite recipe ([#52](https://github.com/azrtydxb/momo/issues/52))
- **core:** `Error()` no longer includes raw server text, the simple protocol is refused, and typed `In` is capped at the bind limit ([#46](https://github.com/azrtydxb/momo/issues/46))
- **gen:** generated files must be regenerated once with `momo generate` after upgrading; `generate --check` reports the header change until then. The exported constant `gen.Version` is removed ([#24](https://github.com/azrtydxb/momo/issues/24))

### Features

- **drivers:** `driver/mysqldriver` for MySQL 8.4, 9.7 and MariaDB 11.8; `driver/sqlitepure`, a pure-Go SQLite driver without cgo; sqlite-vec typed vectors on the cgo SQLite driver ([#46](https://github.com/azrtydxb/momo/issues/46))
- **migrate:** `migrate/mysql` and `migrate/sqlite` (rebuild-based ALTER) with AutoMigrate, Go migrations recorded in history, externally managed extensions and role verification ([#46](https://github.com/azrtydxb/momo/issues/46))
- **importer:** read GORM, Bun and Ent models into momo schemas ([#46](https://github.com/azrtydxb/momo/issues/46))
- **query:** full-text search, array operators, typed JSON operators, date/time expressions, unnest sources, window functions, FILTER, DISTINCT ON, LATERAL, ordered aggregates, advisory transaction locks, FOR UPDATE OF, `LoadCounts` for many parents and PostgreSQL `$n` in raw SQL ([#27](https://github.com/azrtydxb/momo/issues/27) to [#45](https://github.com/azrtydxb/momo/issues/45))
- **write:** `Copy`, `Merge` (PostgreSQL 15+), `BulkUpdate`, `Truncate`, `InsertQuery.Unnest`, and Go-side id generators (cuid, uuid, ulid) ([#46](https://github.com/azrtydxb/momo/issues/46))
- **tx:** a per-transaction setup hook, implicit transactions for RLS tenant scoping, and read replicas ([#46](https://github.com/azrtydxb/momo/issues/46))
- **momotest:** a public test kit for applications built on momo ([#46](https://github.com/azrtydxb/momo/issues/46))
- **docs:** generated capability matrix, type-mapping table and API reference; every backticked identifier in the docs must resolve, and an exported API change needs a docs change ([#52](https://github.com/azrtydxb/momo/issues/52))

### Security

- encode errors are redacted and sensitive arguments masked, LIKE uses an explicit escape, the migration history table is schema-qualified and its owner checked ([#46](https://github.com/azrtydxb/momo/issues/46))

## 0.3.0 (2026-11-07)

_Momo now carries a real PostgreSQL application end to end: kuvryn-license runs fully on it, its queries all on the typed API. Supported backend: PostgreSQL with pgvector (pgx driver). [Changes since 0.2.0](https://github.com/azrtydxb/momo/compare/v0.2.0...v0.3.0)._

### ⚠ BREAKING CHANGES

- **expr:** `momo.As` is generic and takes a typed expression; `Aliased` is `Aliased[T]` with unexported fields. Wrap a bare SQL fragment in `TypedSQL` ([#21](https://github.com/azrtydxb/momo/issues/21))
- **expr:** `Column[T].Excluded` returns `ExcludedExpr[T]`, which still implements `Expression[T]` ([#21](https://github.com/azrtydxb/momo/issues/21))
- **migrate:** the PostgreSQL inspector reports serial columns as identity `serial` instead of `by_default`; a model that declares `pk,identity` for a BIGSERIAL column must switch to the `serial` tag (see the upgrade note in docs/migrations.md) ([#14](https://github.com/azrtydxb/momo/issues/14))
- **migrate:** `ApplyOptions.ForceUnlock` and `momo migrate apply --force-unlock` are removed; they were a no-op on every dialect ([#12](https://github.com/azrtydxb/momo/issues/12))

### Features

- **pgxdriver:** run momo inside a transaction the caller already opened, and hand legacy code the transaction behind a momo one: `AdoptTx`, `SQLTx`, `PgxTx`, `UnwrapSQLTx`, `UnwrapPgxTx` ([#11](https://github.com/azrtydxb/momo/issues/11))
- **gen:** foreign keys on a column that is part of a composite primary key ([#13](https://github.com/azrtydxb/momo/issues/13))
- **schema:** `serial` tag for smallserial, serial and bigserial ids, recognised by the inspector ([#14](https://github.com/azrtydxb/momo/issues/14))
- **query:** select-list aliases usable from subqueries and CTEs ([#15](https://github.com/azrtydxb/momo/issues/15)), and tuple result rows `Row1` to `Row10` with no per-shape projection struct ([#19](https://github.com/azrtydxb/momo/issues/19))
- **insert:** `INSERT ... SELECT` through `Insert[T].FromSelect`, including FROM-less selects guarded by `WHERE NOT EXISTS` ([#16](https://github.com/azrtydxb/momo/issues/16))
- **expr:** typed `Substr`, `Case`, column arithmetic in SET and upserts, `CurrentSchema`, `Func`, `CoalesceExpr`, `Cast`, `Lower`, `Upper`, `Length` and `Concat`, usable in `Where` and `OrderBy` ([#17](https://github.com/azrtydxb/momo/issues/17), [#20](https://github.com/azrtydxb/momo/issues/20))
- **migrate:** `--enforce-not-null` for staged columns, concurrent indexes split into a no-transaction migration, lock-safe UNIQUE and CHECK, chain and order checks on apply, `momo migrate rehash`, URL redaction ([#12](https://github.com/azrtydxb/momo/issues/12))

## 0.2.0 (2026-11-06)

Conditional upserts, the AI coder guide and runnable examples. Supported backend: PostgreSQL with pgvector (pgx driver). [Changes since 0.1.0](https://github.com/azrtydxb/momo/compare/v0.1.0...v0.2.0).

### Features

- **insert:** WHERE on ON CONFLICT target and DO UPDATE ([#9](https://github.com/azrtydxb/momo/issues/9)) ([ffae95a](https://github.com/azrtydxb/momo/commit/ffae95a6c8eed9e551758d7f5fa41181af9aa00f))

### Documentation

- Guide for AI coding agents (`docs/ai-guide.md`, `llms.txt`), capability matrix, supported versions, migration safety and recovery guide, and nine runnable examples tested in CI ([#7](https://github.com/azrtydxb/momo/issues/7))

### Bug Fixes

- **schema:** keep `E'…\'…'` escape-string literals intact when normalising expressions for migration diffs ([#9](https://github.com/azrtydxb/momo/issues/9))

## 0.1.0 (2026-11-06)

First release. Supported backend: PostgreSQL with pgvector (pgx driver), including PgBouncer. SQLite and MySQL are not usable yet; only their dialect metadata exists.

### Features

- **dialect:** add MySQL dialect metadata (flavours and capability set); the MySQL backend is not usable yet ([d19c341](https://github.com/azrtydxb/momo/commit/d19c34165c2d286c969d431430c9eb65e6f49ed6))
- **errors:** add the sentinel set, Detail accessor and vector and migrate sentinels ([da81d21](https://github.com/azrtydxb/momo/commit/da81d211f24383c4a2e8f275751bf75ee8cdb6fd))
- **momo:** keyset pagination with deterministic tie-breaking ([0421baf](https://github.com/azrtydxb/momo/commit/0421bafaa7902e44d2493d0f4d0fad7f42d4716c))
- **relations:** eager loading, predicates, associations, graph writes ([#5](https://github.com/azrtydxb/momo/issues/5)) ([c744711](https://github.com/azrtydxb/momo/commit/c74471152e515379901eb92213513c2c44e50208))
- **schema:** base snapshot types with format_version and JSON round-trip ([81950d4](https://github.com/azrtydxb/momo/commit/81950d40076f41766c68293a3886caf1847fb971))
- **schema:** normalize types and expressions and compare schemas canonically ([c0db748](https://github.com/azrtydxb/momo/commit/c0db748c0a5a0ad28a806aea18e1256a30c959bb))
- **schema:** read snapshots by format version and refuse newer formats ([5de8c39](https://github.com/azrtydxb/momo/commit/5de8c395aa58e2ec387eb17226448055f3c844ee))
- sprint 2 — generator, PostgreSQL driver, migrations, pgvector ([#4](https://github.com/azrtydxb/momo/issues/4)) ([8f62dc9](https://github.com/azrtydxb/momo/commit/8f62dc9ac67d9d0da9e19f960804613f4c9fd974))

### Bug Fixes

- **momo:** keep the soft-delete filter with restricted columns and align Meta with the contract ([634869f](https://github.com/azrtydxb/momo/commit/634869f78f7e04d8f013015a32369386fef54ef1))
- **momo:** MySQL distinct-from and clone variadic slices ([8c6657f](https://github.com/azrtydxb/momo/commit/8c6657f8ea596e0b695969f37f447c640ae342c1))
- **momo:** parenthesise raw parts of And/Or ([aa00889](https://github.com/azrtydxb/momo/commit/aa0088946b7af4c36e86fd241c23399abc2d3a6b))
- **momo:** qualify columns of schema-qualified tables by table name ([ff1c4a3](https://github.com/azrtydxb/momo/commit/ff1c4a3e103cf2f0285d95e547867a31d03f38bc))
- **momo:** render internal bytes without discarding an always-nil error ([28f4378](https://github.com/azrtydxb/momo/commit/28f437854fa86115c7c66d9ef73ecd8f21e7313c))
- **momo:** share transaction done state with savepoint scopes ([28d90ac](https://github.com/azrtydxb/momo/commit/28d90ac172673566ae3956dd83256732963bf2ab))
- **momo:** validate select inputs instead of silently ignoring them ([4cfad2c](https://github.com/azrtydxb/momo/commit/4cfad2cf9510a20c74fec832e3eb8236b3084884))
- **schema:** keep type suffix after precision when normalising ([4803895](https://github.com/azrtydxb/momo/commit/48038953b13ccb7963789a92372e7a1c1f45ddd8))
- **testdb:** fail when search_path cannot be set ([e17281c](https://github.com/azrtydxb/momo/commit/e17281cdacee0286e4518169c2f2a89adf2c3e1e))
- **testdb:** report ensureVector errors to every caller, keep password out of DO ([d67c1e6](https://github.com/azrtydxb/momo/commit/d67c1e6ea166d23ff786cf1872bb0e0ed6324633))
