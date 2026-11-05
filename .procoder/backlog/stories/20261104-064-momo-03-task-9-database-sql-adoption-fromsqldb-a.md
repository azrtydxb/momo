# momo-03 Task 9: database/sql adoption: FromSQLDB and FromConnConfig (pgx stdlib)

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 9: database/sql adoption: FromSQLDB and FromConnConfig (pgx stdlib) (`.procoder/plans/momo-03-drivers.md:990`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/sqldb.go`, `driver/pgxdriver/sqldb_test.go`, `driver/pgxdriver/sqldb_adapter_test.go`. Steps 2b, 2c and 5b (`TestPostgresSQLDBAdapter`, S-88) are a separate story scheduled after momo-05 Task 35.

## Acceptance criteria

- [x] `TestFromSQLDBPinsAndCounts` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestFromConnConfigExecMode` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./driver/pgxdriver -run 'FromSQLDB|FromConnConfig'` failed to build with `undefined: FromSQLDB` and `undefined: FromConnConfig` before `sqldb.go` existed.
- Green: the same run with `-race -count=1` passes `TestFromSQLDBPinsAndCounts` and `TestFromConnConfigExecMode`, plus added tests `TestFromSQLDBTxOptionsAndDone`, `TestFromSQLDBClassifiesErrors`, `TestFromSQLDBPinReleaseClosesRows` and `TestFromSQLDBDiscardDestroysSession` (the adapter also implements `momo.Discarder`).
- Mutation (plan Step 4): with `QueryExecModeCacheStatement`, `TestFromConnConfigExecMode` fails with `exec mode left 2 named statements on the session`; restored to `QueryExecModeExec`.
- Gate: `procoder check` 0 blocking; `go vet ./...`, `gofmt -l .`, `go mod tidy -diff`, `go test ./...` and `go test -tags integration -race ./driver/...` all clean.
- Steps 2b, 2c and 5b (`TestPostgresSQLDBAdapter`, S-88) are out of this story's scope and need `suite.RunAll` (momo-05 Task 35), which does not exist yet in `internal/suite`.
- Deferred, NOT satisfied: the S-88 acceptance test (`suite.RunAll`, plan steps 2b, 2c and 5b, `TestPostgresSQLDBAdapter`) is deferred until momo-05 Task 35 provides `suite.RunAll`; it has not been run and the adapter is not yet proven against the shared suite.
