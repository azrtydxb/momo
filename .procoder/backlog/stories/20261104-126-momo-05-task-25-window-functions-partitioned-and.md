# momo-05 Task 25: Window functions, partitioned and ordered (S-33, window part only)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 25: Window functions, partitioned and ordered (S-33, window part only) (`.procoder/plans/momo-05-writes-queries.md:2080`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-33. Files: `window.go`, `window_test.go`.

## Acceptance criteria

- [ ] `TestWindowFunctions` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-33] `TestWindowFunctions` asserts ranks per partition; `TestEagerLoadTopNPerParent` asserts at most N children per parent in one query (query-count hook); fails if the strategy falls back to per-parent queries.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

