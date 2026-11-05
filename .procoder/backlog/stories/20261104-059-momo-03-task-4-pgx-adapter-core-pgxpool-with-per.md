# momo-03 Task 4: pgx adapter core (pgxpool) with per-call exec mode

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 4: pgx adapter core (pgxpool) with per-call exec mode (`.procoder/plans/momo-03-drivers.md:340`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/driver.go`, `driver/pgxdriver/driver_test.go`, `go.mod`.

## Acceptance criteria

- [x] `TestExecModeExecIsApplied` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSimpleProtocolRejected` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDefaultExecModeNoNamedStatements` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./driver/pgxdriver/` failed with `undefined: New` / `undefined: WithExecMode`. Green: `go test -race ./driver/pgxdriver/` and `make test-integration-postgres` pass against the Momo compose Postgres.
- `TestDefaultExecModeNoNamedStatements` is written as in the plan but SKIPS (`Pin lands in momo-03 Task 7`), so its criterion stays unchecked. The same property is proven now by `TestDefaultExecModeNoNamedStatementsInTx` (pg_prepared_statements count is 0 on the tx connection).
- Extra tests: context cancel propagation, Rows.Close releases the pooled connection on early exit, error classification and redaction (incl. SQLSTATE 22 echoing the argument), isolation and read-only mapping, ErrTxDone, concurrent use under -race.
- Mutation: making pgxRows.Close a no-op leaks the connection and TestRowsCloseReleasesConnection hangs at pool cleanup (detected).
- `procoder check`: 0 blocking findings.
