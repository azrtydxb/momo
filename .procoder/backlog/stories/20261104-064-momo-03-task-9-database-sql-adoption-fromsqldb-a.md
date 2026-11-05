# momo-03 Task 9: database/sql adoption: FromSQLDB and FromConnConfig (pgx stdlib)

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 9: database/sql adoption: FromSQLDB and FromConnConfig (pgx stdlib) (`.procoder/plans/momo-03-drivers.md:990`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/sqldb.go`, `driver/pgxdriver/sqldb_test.go`, `driver/pgxdriver/sqldb_adapter_test.go`. Steps 2b, 2c and 5b (`TestPostgresSQLDBAdapter`, S-88) are a separate story scheduled after momo-05 Task 35.

## Acceptance criteria

- [ ] `TestFromSQLDBPinsAndCounts` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestFromConnConfigExecMode` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

