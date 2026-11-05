# momo-03 Task 8: COPY and batch paths on PostgreSQL

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 8: COPY and batch paths on PostgreSQL (`.procoder/plans/momo-03-drivers.md:821`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/bulk.go`, `driver/pgxdriver/bulk_test.go`, `driver/pgxdriver/bulk_tx_test.go`, `driver_access.go`, `driver/pgxdriver/driver.go`.

## Acceptance criteria

- [ ] `TestCopyInsertMatchesRows` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestBatchRunsAllStatementsInOneRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCopyAndBatchInTransaction` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

