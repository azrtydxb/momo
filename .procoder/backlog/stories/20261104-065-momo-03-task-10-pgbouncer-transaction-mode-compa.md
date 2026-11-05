# momo-03 Task 10: PgBouncer transaction-mode compatibility with QueryExecModeExec

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 10: PgBouncer transaction-mode compatibility with QueryExecModeExec (`.procoder/plans/momo-03-drivers.md:1158`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-30. Files: `driver/pgxdriver/pgbouncer_test.go`, `internal/testdb/compose.yaml`, `internal/testdb/pgbouncer/pgbouncer.ini`, `internal/testdb/pgbouncer/userlist.txt`.

## Acceptance criteria

- [ ] `TestPgBouncerTransactionMode` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-30] `TestPgBouncerTransactionMode` runs the query suite through a PgBouncer container in transaction mode with the simple-protocol option; fails if any statement relies on a named prepared statement.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

