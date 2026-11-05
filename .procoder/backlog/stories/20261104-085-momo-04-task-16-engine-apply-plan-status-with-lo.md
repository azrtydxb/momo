# momo-04 Task 16: Engine Apply, Plan, Status with lock, checksum and dirty gating

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Note (Task 9 review): use `migrate.OrderErr` (or `OrderWithViews`), not `Order`, and surface the cycle error it returns.

Implements plan momo-04-migrations Task 16: Engine Apply, Plan, Status with lock, checksum and dirty gating (`.procoder/plans/momo-04-migrations.md:1903`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11. Files: `migrate/engine.go`, `migrate/engine_apply_test.go`. Contributes to S-10, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestConcurrentApply` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestApplyTo` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestChecksumMismatchRefusesApply` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Deferred from story 082 (owner: this story, once the Engine and `migratetest` exist): (1) `TestPostgresObjects` run through the Engine (`migratetest.NewEngine`, `e.Diff`, `e.Apply`, `e.Inspect`) exactly as the plan's Task 13 Step 1 writes it, including the CONCURRENTLY index applied with `NoTx`; (2) the S-39 read-back for `TestManagedFunctionsTriggersPartitions`: no diff after apply and no function, trigger or table reported as unmanaged by `e.Drift`. The function and trigger read-back also needs story 081's inspection.

## Notes

- Story 083 review: Apply must run the Setup reset before the Lock unlock (register the unlock defer first, the reset defer second), so a small statement timeout cannot cancel the unlock. A failed reset or unlock already discards the pinned connection in the PostgreSQL dialect; Apply only joins the errors. See the Task 16 note in the plan.

- Done: `migrate/engine.go` (Apply, Plan, Status, resolveSchemas, lock wait loop), `migrate/options.go` (`WithLockWait`), `migrate/postgres/dialect.go` (the real dialect lacked the `Dialect()` method, added), `migrate/migratetest` (the PG stub, its history DDL, Setup and Lock are deleted; `EmptyFS`, `FS`, `Diff` added), `migrate/engine_apply_test.go`. Every engine test now uses `mpg.New()`: real advisory lock, session setup and history DDL.
- Failing first: before the implementation `go vet -tags integration ./migrate/` failed with `undefined: migrate.WithLockWait` and `Status undefined`; after it `MOMO_REQUIRE_INTEGRATION=1 ... go test -tags integration -race -count=1 ./migrate/... ./driver/...` passes for migrate and migrate/postgres (three runs). `./driver/pgxdriver` fails only `TestBatchThroughPgBouncerTransactionPooling` because `MOMO_TEST_PGBOUNCER_URL` is not set; it does not touch this change.
- Concurrency: `TestConcurrentApply` (plan text, `WithLockWait(0)`): two engines, one gets ErrLocked or finds nothing to do, 2 applied rows exactly. `TestConcurrentApplyWaits`: 4 engines with a 20s wait all succeed, the 2 versions are applied once in total. `TestApplyLockWaitTimesOutWithHolder` (ErrLocked naming `held by pid=`, waits the budget, applies nothing) and `TestApplyWaitsForLockRelease`.
- Cleanup: `TestApplyResetsBeforeUnlock` (a recording dialect asserts reset then unlock), `TestApplyJoinsResetFailureAndStillUnlocks` (reset error joined, unlock still runs, Report kept), `TestApplyCancelledStillUnlocks`. Mutation: registering reset first and unlock second fails both order tests (`cleanup order = "unlock,reset"`); restored.
- Gates: `TestChecksumMismatchRefusesApply`, `TestSumFileMismatchRefusesApply` (momo.sum vs directory), `TestApplyRefusesDirtyAndReleasesLock`, `TestApplySchemasMustMatchCurrentSchema`, `TestStatusStates` (applied, pending, dirty, mismatch, missing).
- Deferred check from story 082: `TestPostgresObjects` now applies through `migratetest.NewEngine` and `e.Apply`, the CONCURRENTLY index in its own no-transaction migration file; `TestManagedFunctionsTriggersPartitions` applies the partitioned table, function and trigger through the Engine, reads them back with `schema.Equal`, an empty second diff and exact object counts, and applies the body change (CREATE OR REPLACE) through the Engine too.
- Deviations: (1) EnsureHistory runs after the lock and Setup, not before the lock, so two first-time callers cannot race on CREATE TABLE (plan text updated). (2) `Engine.Diff`, `Engine.Inspect` and `Engine.Drift` do not exist until Tasks 17 and 18, so those tests build the migration files from Diff, OrderWithViews (errors surfaced) and Render, apply them through `Engine.Apply`, and inspect with the dialect directly; "not unmanaged" is asserted as schema.Equal plus exact object counts, not `e.Drift`. (3) A directory without momo.sum skips the momo.sum check (the plan's literal tests have none); the per-row checksum check still runs. (4) `Apply` refuses a `To` version that has no up file. (5) Plan and Status run EnsureHistory without the lock, so on a database with no history table they can race a concurrent first Apply on CREATE TABLE. (6) `ApplyOptions.ForceUnlock` is not used here (owned by Task 29).
- Finding for Task 17/18: `Dialect.Inspect` reports the history table `momo_migrations` and does not apply `InspectOptions.Exclude`; the test helper drops it. Partition bounds are compared as text: a `timestamptz` bound written as `'2026-01-01'` reads back as `'2026-01-01 00:00:00+00'` (session time zone), so the S-39 fixture partitions by a `date` column.
