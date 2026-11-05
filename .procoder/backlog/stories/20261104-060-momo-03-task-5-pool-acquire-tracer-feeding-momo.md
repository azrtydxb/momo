# momo-03 Task 5: Pool acquire tracer feeding momo.RecordPoolWait

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 5: Pool acquire tracer feeding momo.RecordPoolWait (`.procoder/plans/momo-03-drivers.md:456`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/tracer.go`, `driver/pgxdriver/tracer_test.go`, `internal/testdb/testopen/testopen.go`. Depends on momo-09 Task 1 (`momo.RecordPoolWait`), scheduled before it.

## Acceptance criteria

- [x] `TestPoolWaitRecordedWhenExhausted` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./driver/pgxdriver/ -run PoolWait` failed to build with `undefined: InstrumentPool` / `undefined: NewPool`. Green: `go test -race -count=3` passes against the Momo compose Postgres; full `make test-integration-postgres` run at the end of the sprint-2 batch.
- Deviations: (1) the plan test holds the only connection with `Pin`, which lands in Task 7, so it uses `pool.Acquire` here (same effect on the pool); (2) the test lives in package `pgxdriver` (not `pgxdriver_test`) to reuse the guarded `closePool`; (3) `testopen.Postgres` is not changed because `internal/testdb/testopen` (story 056, Task 1) is not on sprint-2 yet; (4) per the sprint brief, wait is recorded only for real waits: an acquire served from an idle connection records nothing (pgxpool `EmptyAcquireCount` unchanged), a failed or timed-out acquire records the time it waited.
- Extra tests: idle acquire records nothing, timed-out acquire records its wait, previous query tracer is still called, `NewPool` rejects a bad URL.
- Mutation: always recording (ignoring the empty-acquire check) fails `TestPoolWaitNotRecordedWhenIdle` (`PoolWait = 3.4us on warm idle acquires, want 0`); the no-`InstrumentPool` mutation is the test `TestPoolWaitNotRecordedWithoutInstrumentation`.
- `procoder check`: 0 blocking findings. `make staticcheck` clean.
