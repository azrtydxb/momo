# momo-03 Task 4: pgx adapter core (pgxpool) with per-call exec mode

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Implements plan momo-03-drivers Task 4: pgx adapter core (pgxpool) with per-call exec mode (`.procoder/plans/momo-03-drivers.md:340`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/driver.go`, `driver/pgxdriver/driver_test.go`, `go.mod`.

## Acceptance criteria

- [ ] `TestExecModeExecIsApplied` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSimpleProtocolRejected` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDefaultExecModeNoNamedStatements` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

