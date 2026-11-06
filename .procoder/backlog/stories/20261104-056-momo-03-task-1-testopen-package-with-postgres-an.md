# momo-03 Task 1: testopen package with Postgres and Each (milestone M1)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 1: testopen package with Postgres and Each (milestone M1) (`.procoder/plans/momo-03-drivers.md:50`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `internal/testdb/testopen/testopen.go`, `internal/testdb/testopen/testopen_test.go`.

## Acceptance criteria

- [x] `TestEachRunsPostgres` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestOptionsReachDB` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEachPassesOptions` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test ./internal/testdb/testopen/` failed to build: `undefined: Each`, `undefined: openers`, `undefined: Postgres`.
- GREEN: `go test -race -count=1 -v ./internal/testdb/testopen/` with MOMO_REQUIRE_INTEGRATION=1 against the running PostgreSQL: TestEachRunsPostgres, TestOptionsReachDB, TestEachPassesOptions, TestEachNamesSubtestAfterDialect, TestEachSubtestsAreIsolated, TestRequireFailsWithoutURL all PASS (subtest `postgres`).
- Mutation: removing `t.Cleanup(pool.Close)` made TestEachSubtestsAreIsolated fail ("pool not closed"); restored.
- `make test-integration-postgres`, `go test -race ./...` (no URL: skips), `go vet`, `make staticcheck` clean; `procoder check`: 0 blocking.
- Deviation: the count-asserting tests call `testdb.PostgresURL(t)` first so a missing URL skips/fails through testdb instead of reporting "ran 0 dialects".
