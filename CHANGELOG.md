# Changelog

## [0.5.0](https://github.com/azrtydxb/momo/compare/v0.4.0...v0.5.0) (2026-11-10)


### ⚠ BREAKING CHANGES

* `advisory.XactLockString` and `advisory.TryXactLockString` take a trailing `...advisory.Option`, so assigning them to a variable of the old func type no longer compiles. `momo generate` now wires methods named `BeforeInsert`, `BeforeUpdate`, `BeforeDelete` and `AfterLoad` into `Meta.Hooks`, and a hook-named method with another signature fails generation. `momo generate --from-db` adds the `dbdefault` tag to NOT NULL PostgreSQL columns with a computed default, so regenerated models stop writing Go zero values to those columns. The CLI moved from `cmd/momo` into package `cli`. See docs/upgrading.md, "Upgrading to 0.5.0".

### Features

* sprint 7 Orbit port issues ([#54](https://github.com/azrtydxb/momo/issues/54)-[#69](https://github.com/azrtydxb/momo/issues/69)) and backlog stories ([#70](https://github.com/azrtydxb/momo/issues/70)) ([2b4435c](https://github.com/azrtydxb/momo/commit/2b4435c78e0cf7f92bd3704baaacb1a16d7eaa88))

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
