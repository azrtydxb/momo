# momo-06 Task 12: Top-N per parent with a window function

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 12: Top-N per parent with a window function (`.procoder/plans/momo-06-relations.md:2677`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-33. Files: `load.go`, `load_test.go`.

## Acceptance criteria

- [ ] `TestEagerLoadTopNPerParent` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-33] `TestWindowFunctions` asserts ranks per partition; `TestEagerLoadTopNPerParent` asserts at most N children per parent in one query (query-count hook); fails if the strategy falls back to per-parent queries.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

