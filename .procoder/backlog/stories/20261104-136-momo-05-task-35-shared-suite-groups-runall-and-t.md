# momo-05 Task 35: Shared suite groups, RunAll and the integration gate (S-4, S-20)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 35: Shared suite groups, RunAll and the integration gate (S-4, S-20) (`.procoder/plans/momo-05-writes-queries.md:2891`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-20. Files: `internal/suite/run.go`, `integration_queries_test.go`. Contributes to S-4, S-20; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestIntegrationQueries` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSuiteRunAll` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made
- [x] momo-03 Task 9 steps 2b, 2c and 5b (deferred from story 064): `suite.RunAll` runs against the `pgxdriver.FromSQLDB` adapter, and the S-88 acceptance test passes

## Evidence

- Red first: `integration_queries_test.go` (as written in the plan, plus the `integration` build tag) failed to compile with `undefined: suite.Queries` before `internal/suite/run.go` existed.
- Green: `go test -tags integration -race -count=1 -run 'TestIntegrationQueries|TestSuiteRunAll' -v .` passes on postgres, running transactions, errors, queries (joins, aggregates, subqueries, returning) and writes (insert, guardrails, optimistic-lock, delete, soft-delete, soft-delete-variants, upsert, timestamps). The full `go test -tags integration -race -count=1 . ./internal/... ./driver/...` reports no FAIL.
- Scope deviation: `Queries` and `Writes` contain only the members whose features have merged (see the doc comments in `internal/suite/run.go`). Members of the plan list whose feature stories are still open (keyset, locking, hints, table override, raw, json-array, window; write states, bulk insert, COPY, defaults and validators, merge and bulk update) and `EmptyResultNotNotFound` and `ErrorRedaction` do not exist yet; those stories add their member and its line in `run.go`. No stub or skipped member was added. The members live in `internal/suite/queries.go` and `internal/suite/writes.go` and mirror the existing root-package integration tests (the root tests are unchanged).
- S-88 (steps 2b, 2c, 5b): `driver/pgxdriver/sqldb_adapter_test.go` (`TestPostgresSQLDBAdapter`, build tag `integration`) runs `suite.RunAll` through `pgxdriver.FromSQLDB` and compares `suite.CapsOf` of both handles by reflection against `allowedDiffs` (Copy, Batch). Red first: it would not build without `suite.RunAll` (the state recorded in story 064). Green: `go test -tags integration -race -count=1 -run TestPostgresSQLDBAdapter ./driver/pgxdriver` passes.
- PgBouncer phase 2 (momo-03 Task 10, story 065) runs `suite.RunAll` through the pooler in `TestPgBouncerTransactionMode`. The suite uses unqualified table names, and the PgBouncer image cannot track `search_path`, so a startup `search_path` is not reliable through it. The test therefore runs the suite in a per-run scratch database (`momo_bnc_<hex>`, routed by the pooler's wildcard entry, default `search_path`) that is dropped afterwards; it passes. Queries now includes the keyset members; UpsertUpdatesAllColumns asserts the later-clock updated_at and unchanged created_at; ReturningRendersAndScans skips without caps.Returning; SoftDeleteVariants gates the partial unique indexes on the new caps.PartialIndexes.
- Gate: `procoder check` reports 0 blocking (info only: doc-mention notices and gocyclo on two test-helper functions); commits 2621081 and fecfb46.
