# momo-04 Task 25: PostgreSQL end-to-end apply with the history contract

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each
Carried: 002-generator-from-go-structs-with-its-type-prerequisites — Red-first criteria unverifiable: the tested behaviour pre-existed when the plan tests were written. Decided 2026-11-06: carry rather than tick.

## Description

Implements plan momo-04-migrations Task 25: PostgreSQL end-to-end apply with the history contract (`.procoder/plans/momo-04-migrations.md:2699`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-12. Files: `migrate/postgres/apply_test.go`. Contributes to S-11, S-12; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestCreateIndexConcurrentlyApply` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPostgresApplyRecovery` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Both tests are in `migrate/postgres/apply_test.go` (integration tag) and pass against the live PostgreSQL: `go test -tags integration -race -run 'TestCreateIndexConcurrentlyApply|TestPostgresApplyRecovery' ./migrate/postgres/` is ok.
- Reconciliation: the engine, dialect wiring and no-transaction runner already existed (migrate/runner.go), so the tests passed on first run; there was no red state to observe, so the "failed before the implementation" half of the first two criteria is not satisfied and they stay unticked. Instead each test was proven to catch a break by mutation. Disabling the no-transaction path (`if false && (p.NoTx || ...)`) fails `TestCreateIndexConcurrentlyApply`; routing every migration through the autocommit runner (`if true {`) fails `TestPostgresApplyRecovery`. runner.go was restored afterwards. Two mutants were equivalent and survived: swapping Rollback for Commit on a failed transaction (PostgreSQL rolls an aborted transaction back on COMMIT), and running statements on the pinned connection instead of the tx handle (same session, same transaction).
- Deviation from the plan sketch: the plan's `testopen.Postgres` plus `WithSum`/`NewEngine` are used through the package's existing `engineOn` helper (adds the per-test schema config), and a `queryOne` helper releases its pinned connection promptly. The tests additionally assert the history row is `applied` (concurrent index) and that no `applied` row exists after the failed migration.
- `make fmt-check vet staticcheck gosec tidy-check` clean. In the full integration run `TestDriftOneFailingTableFallsBackAlone` and `TestMigrateLifecycle` (hstore reported as unmanaged) failed once each on the database shared with parallel agents, and both pass in isolation.

