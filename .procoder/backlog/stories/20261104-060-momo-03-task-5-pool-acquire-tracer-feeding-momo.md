# momo-03 Task 5: Pool acquire tracer feeding momo.RecordPoolWait

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Implements plan momo-03-drivers Task 5: Pool acquire tracer feeding momo.RecordPoolWait (`.procoder/plans/momo-03-drivers.md:456`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/tracer.go`, `driver/pgxdriver/tracer_test.go`, `internal/testdb/testopen/testopen.go`. Depends on momo-09 Task 1 (`momo.RecordPoolWait`), scheduled before it.

## Acceptance criteria

- [ ] `TestPoolWaitRecordedWhenExhausted` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

