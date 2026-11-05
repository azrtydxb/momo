# momo-04 Task 15: Runner, history table, failure recovery, resolve

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 15: Runner, history table, failure recovery, resolve (`.procoder/plans/momo-04-migrations.md:1720`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8, S-11, S-22. Files: `migrate/history.go`, `migrate/options.go`. Contributes to S-8, S-11, S-22; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestNonTransactionalFailureRecovery` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestTransactionalFailureRolledBack` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestHistoryFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: migrate/history.go, migrate/runner.go, migrate/engine.go (Engine, NewEngine, Plan, Apply, Resolve), migrate/types.go (Pending.Checksum, ResolveApplied/ResolveRolledBack), migrate/migratetest/migratetest.go, migrate/{runner,history,helpers}_test.go. WithDialect already existed in options.go.
- RED: before the implementation `go test -tags integration ./migrate/` failed to build (undefined migrate.Engine, migrate.NewEngine, migrate.HistoryRow, migrate.ReadHistory).
- GREEN: TestNonTransactionalFailureRecovery, TestTransactionalFailureRolledBack, TestHistoryFormatVersion, TestResolveApplied and TestNewEngineRequiresDialect pass on the live PostgreSQL with -race (per-test schemas).
- Mutation check: writing StatusFailed instead of StatusRolledBack for a transactional failure makes TestTransactionalFailureRolledBack fail (want rolled_back, got failed); reverted.
- Gate: procoder check 0 blocking, make staticcheck clean, go build and go vet pass.
- Deviation: migrate/postgres has no HistoryDDL, Setup, Lock or Dialect() yet (story 083 owns them), so tests use migratetest.Postgres(), a wrapper over mpg.Dialect with the history DDL and no-op Setup/Lock, instead of mpg.New(). After 083 merges, switch the tests to mpg.New() and delete the wrapper. Apply takes no lock, applies no session timeouts and has no checksum gate (Task 16). fk-check and dependent: hints are SQLite-only and do nothing on PostgreSQL. Story 084 carried no extra notes in this tree.
