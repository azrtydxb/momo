# momo-07 Task 11: sqlite-vec loading, metrics and ANN rejection

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-07-vectors-sqlite-vec
Sprint: -

## Description

Implements plan momo-07-vectors Task 11: sqlite-vec loading, metrics and ANN rejection (`.procoder/plans/momo-07-vectors.md:1819`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-16. Files: `driver/sqlitedriver/vec.go`, `driver/sqlitedriver/vec_test.go`, `driver/sqlitedriver/parity_integration_test.go`.

## Acceptance criteria

- [x] `TestSQLiteVecLoaded` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLiteVecMetrics` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLiteANNIndexUnsupported` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLiteVectorParity` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-16] `TestSQLiteVectorParity`: sqlite-vec distance queries return the same top-k ids as pgvector exact search on the same data; requesting an ANN index on SQLite returns ErrUnsupported; fails if SQLite silently accepts an index option.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Red: ci-agent run 42466619808 (throwaway branch worktree-agent-aadb520be05ad24c8-red, headSha b0bda384f9725c615c70fa3f217eb37de6215549, tests only on top of sprint-5, branch deleted). unit and the PostgreSQL integration job failed to build: `undefined: SQLiteVectorType`, `undefined: SQLiteBitsType`, `undefined: L2Distance`/`ExactTopK` in internal/vectorparity, `undefined: parseFixtures`; every job of the run failed.

Green: ci-agent run 42470332826 (headSha fac622492dfa05166e9ec2e031f54933fbedd8fb): lint, unit, fuzz-smoke, PostgreSQL integration, MySQL 8.4, MySQL 9.7 and MariaDB integration all success. The SQLite tests run in the PostgreSQL job and in the MySQL/MariaDB jobs (no server needed, cgo on the runner); the pgvector leg of `TestSQLiteVectorParity` runs in the PostgreSQL job.

Tests added: `driver/sqlitedriver/vec_test.go` (`TestSQLiteVecLoaded`, `TestSQLiteVecMetrics`, `TestSQLiteANNIndexUnsupported`, `TestCheckVecVersion`), `driver/sqlitedriver/parity_integration_test.go` (`TestSQLiteVectorParity`), `vector_sqlite_test.go` (unit), `vector_sqlite_integration_test.go` (distances against the plain-Go reference, bit vectors, nullable pointers, refusals, search settings, sensitive redaction on a real SQLite database), `vector/dialect_test.go` (SQLite DialectValue validates).

Deviations from the plan, all deliberate:

- `Open` already existed (open.go, M2 story). `vec.go` holds `SQLiteVecVersion` and `checkVecVersion`; `Open` now runs `SELECT vec_version()` and returns ErrUnsupported on another version. `sqlite_vec.Auto()` was already called there.
- `TestSQLiteANNIndexUnsupported`: there is no `migrate/sqlite` and no SQLite migration engine, so the plan's `sqliteEngineDiff` cannot exist. The ANN refusal is tested at the two places that exist: new `momo.CheckVectorIndexMethod(d, method)` (ErrUnsupported on SQLite for hnsw, ivfflat and vector) and `schema.ValidateVectorIndex` (ErrInvalidVectorIndex for SQLite). The `migrate/sqlite` wrapping stays with the SQLite migration story. The generator still reports vector fields and indexes on SQLite as unsupported (unchanged); tables are created with own DDL.
- New helpers not in the plan: `momo.SQLiteVectorType` and `momo.SQLiteBitsType` (BLOB with the length CHECK the plan describes), `bindSQLiteVector` in vector_sqlite.go (a nil `*vector.Vector` or `*vector.Bits` bound as NULL, which previously panicked in DialectValue; same pattern as vector_mysql.go, dispatched from argFor), and validation inside `Vector.DialectValue` and `Bits.DialectValue` for SQLite (NaN, empty and bad lengths are ErrInvalidVector instead of being stored).
- `TestSQLiteVectorParity` uses the typed builders and the shared dataset of internal/vectorparity (300 vectors, dim 8, cosine, L2 and L1, via new `ExactTopK`, `L2Distance`, `L1Distance`) instead of 500 raw-SQL vectors; the pgvector comparison is a subtest that needs a PostgreSQL URL. It is in package `sqlitedriver_test` with tag `integration && cgo && !windows`.
- The vector builder tests use `testopen.SQLite`, not `testopen.Each`: a vector column needs the sqlite-vec DDL (BLOB plus CHECK), not DDL shared with PostgreSQL. MySQL and MariaDB rendering is untouched (bindMySQLVector runs first and the SQLite binding is gated on the dialect).
- Also fixed on the way: internal/compat/gormfixture/go.sum carried two stale `go-sqlite3 v1.14.22` lines that made `make tidy-check` fail (left by sprint-5).
