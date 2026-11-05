# momo-09 Task 1: QueryEvent pool wait, transaction flag, argument count and dialect

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-09-observability-testing Task 1: QueryEvent pool wait, transaction flag, argument count and dialect (`.procoder/plans/momo-09-observability-testing.md:136`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18. Files: `db.go`, `internal/obstest/obstest.go`, `obs_poolwait_test.go` (tests run on `internal/fakedriver`, postgres and sqlite dialects; plan amended in 217b724). Contributes to S-18; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestQueryEventPoolWait` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestRecordPoolWaitWithoutHooks` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestQueryEventInTx` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestQueryEventPoolWait`: before the implementation `go test ./ -run 'TestQueryEventPoolWait|TestRecordPoolWaitWithoutHooks|TestQueryEventInTx' -count=1` failed to compile with `undefined: momo.RecordPoolWait` and `e.PoolWait undefined (type obstest.Event has no field or method PoolWait)`; after, `--- PASS: TestQueryEventPoolWait (postgres, sqlite)`.
- `TestRecordPoolWaitWithoutHooks`: same failing compile before; after, `--- PASS: TestRecordPoolWaitWithoutHooks (postgres, sqlite)`.
- `TestQueryEventInTx`: same failing compile before; after, `--- PASS: TestQueryEventInTx (postgres, sqlite)`.
- Gate and commit: `go build ./... && go vet ./... && go test -race -count=1 ./...` all ok; `procoder check` printed `0 unformatted, 0 unchecked, 0 blocking`; commit e21da9d.
