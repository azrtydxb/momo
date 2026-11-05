# momo-05 Task 35: Shared suite groups, RunAll and the integration gate (S-4, S-20)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 35: Shared suite groups, RunAll and the integration gate (S-4, S-20) (`.procoder/plans/momo-05-writes-queries.md:2891`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-20. Files: `internal/suite/run.go`, `integration_queries_test.go`. Contributes to S-4, S-20; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestIntegrationQueries` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSuiteRunAll` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made
- [ ] momo-03 Task 9 steps 2b, 2c and 5b (deferred from story 064): `suite.RunAll` runs against the `pgxdriver.FromSQLDB` adapter, and the S-88 acceptance test passes

## Evidence

