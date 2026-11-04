# momo-05 Task 35: Shared suite groups, RunAll and the integration gate (S-4, S-20)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 35: Shared suite groups, RunAll and the integration gate (S-4, S-20) (`.procoder/plans/momo-05-writes-queries.md:2891`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-20. Files: `internal/suite/run.go`, `integration_queries_test.go`.

## Acceptance criteria

- [ ] `TestIntegrationQueries` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSuiteRunAll` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [ ] [S-20] `TestCIWorkflowCoverage` parses .github/workflows/ci.yml and asserts it runs unit, generator, compile-fail, race and fuzz-smoke tests, plus integration tests against pgvector/pgvector:pg17 and SQLite+sqlite-vec, on Go 1.26 and 1.27; fails if any job is skipped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

