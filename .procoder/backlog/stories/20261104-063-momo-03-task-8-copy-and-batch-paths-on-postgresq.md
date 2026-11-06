# momo-03 Task 8: COPY and batch paths on PostgreSQL

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 8: COPY and batch paths on PostgreSQL (`.procoder/plans/momo-03-drivers.md:821`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/bulk.go`, `driver/pgxdriver/bulk_test.go`, `driver/pgxdriver/bulk_tx_test.go`, `driver_access.go`, `driver/pgxdriver/driver.go`.

## Acceptance criteria

- [x] `TestCopyInsertMatchesRows` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestBatchRunsAllStatementsInOneRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCopyAndBatchInTransaction` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./driver/pgxdriver/ -run 'Copy|Batch|Capabilities'` failed to build: `undefined: momo.CopierOf`, `undefined: momo.BatcherOf` (the pool-path `interface conversion` failure of plan step 2b is masked by that build failure because the transaction test shares the test binary).
- Green: `make test-integration-postgres` (MOMO_REQUIRE_INTEGRATION=1) all packages ok, including TestCopyInsertMatchesRows, TestBatchRunsAllStatementsInOneRoundTrip, TestCopyAndBatchInTransaction, TestCopyAndBatchOnConnUseThePinnedSession, TestBatchFailureNamesStatementIndexWithoutArgsAndReleasesConnection, TestCopyCancelStopsAndReleasesConnection.
- Mutation: removing `nd.drv = pinnedDriver{...}` in OnConn made TestCopyAndBatchOnConnUseThePinnedSession fail (`relation "onconn_copy" does not exist`); restored.
- Gate: `procoder check` 0 blocking; `make staticcheck` clean; go vet clean.
- Commit: 588fb0c feat(pgx): COPY and batch paths.

