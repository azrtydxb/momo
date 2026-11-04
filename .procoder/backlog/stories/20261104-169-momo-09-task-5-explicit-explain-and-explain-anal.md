# momo-09 Task 5: Explicit EXPLAIN and EXPLAIN ANALYZE helpers

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 5: Explicit EXPLAIN and EXPLAIN ANALYZE helpers (`.procoder/plans/momo-09-observability-testing.md:865`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18. Files: `explain.go`, `obs_explain_test.go`.

## Acceptance criteria

- [ ] `TestExplainAnalyzeExplicit` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-18] `BenchmarkSelectNoHooks` allocations stay within the budget recorded in docs/PERFORMANCE.md; `TestHooks` asserts SQL, duration, rows and errors are reported and args only with WithArgLogging; `TestExplainAnalyzeExplicit` asserts plain Explain never executes the query; fails if disabled hooks allocate.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

