# Changelog

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
