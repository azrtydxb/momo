# momo-09 Task 13: Slow-query hook with opt-in plain EXPLAIN

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 13: Slow-query hook with opt-in plain EXPLAIN (`.procoder/plans/momo-09-observability-testing.md:2409`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-54. Files: `slowquery.go`, `obs_slowquery_test.go`.

## Acceptance criteria

- [ ] `TestSlowQueryHook` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-54] `TestSlowQueryHook` reports only queries over the threshold and captures a plain EXPLAIN plan only when enabled; fails if EXPLAIN ANALYZE ever runs.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

