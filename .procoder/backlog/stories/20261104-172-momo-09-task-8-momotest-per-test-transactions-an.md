# momo-09 Task 8: momotest per-test transactions and SQL assertions

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 8: momotest per-test transactions and SQL assertions (`.procoder/plans/momo-09-observability-testing.md:1304`). Spec coverage: S-34.

Scope decision (GitHub issue #34 and audit item N8): the public `momotest` package is built around the issue's ask instead of the plan's `Tx(tb, db)`/`AssertSQL` interface. It offers `New` (a schema per test), `NewTx` (a rolled-back transaction on a hash-named template schema), `Seed`/`Insert`/`Graph` fixtures, the same skip and `MOMO_REQUIRE_INTEGRATION=1` behavior as internal/testdb, and an example plus docs/testing.md. The SQL assertion helpers (`AssertSQL`), `LoadFixtures` from files, `momotest.SQLite` and `AssertUsesIndex` of the plan stay out of this story (SQLite has no driver yet).

## Acceptance criteria

- [x] [#34] `momotest.New` returns an isolated schema per test from a DSN env var, skips when it is unset and fails under `MOMO_REQUIRE_INTEGRATION=1`; unit tests cover both paths without a database
- [x] [#34] the project's embedded migrations are applied (`WithMigrations`, `migrate.Engine` over an `fs.FS`)
- [x] [#34] `momotest.NewTx` is the rollback-per-test mode: data written in a test is absent in the next one, and `Commit` on its Tx is refused with `ErrAdoptedTx`
- [x] [#34] cleanup drops the schema and rolls back the transaction; an integration test asserts the schema is gone after the test
- [x] [#34] eight parallel tests in each mode pass with `t.Parallel` on CI (the lean agent workflow; the race job covers the unit tests)
- [x] [N8] `Seed`, `Insert` and `Graph` insert fixtures in order, atomically, and read generated ids back; an integration test asserts a failed `Seed` keeps nothing
- [x] `examples/testing` runs on CI; docs/testing.md exists; momotest is out of the Planned list in docs/ARCHITECTURE.md
- [x] CI is green on the final commit, including the integration jobs

## Evidence

- CI run 42409375668 on 07ea866: unit, integration (postgres), fuzz-smoke, lint all success.
