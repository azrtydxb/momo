# momo-03 Task 6: Transactions with isolation, read-only access and savepoints on PostgreSQL

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Implements plan momo-03-drivers Task 6: Transactions with isolation, read-only access and savepoints on PostgreSQL (`.procoder/plans/momo-03-drivers.md:603`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/driver.go`, `driver/pgxdriver/tx_test.go`.

## Acceptance criteria

- [ ] `TestTransactionsIsolationReachesServer` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestUnknownIsolationIsUnsupported` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

