# momo-05 Task 31: Result helpers: Pluck, IDs, FirstID, scalars, maps, FindAndCount, Batches (S-78)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 31: Result helpers: Pluck, IDs, FirstID, scalars, maps, FindAndCount, Batches (S-78) (`.procoder/plans/momo-05-writes-queries.md:2554`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-78. Files: `results.go`, `results_test.go`.

## Acceptance criteria

- [ ] `TestResultHelpers` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-78] `TestResultHelpers` asserts each helper's results and that Batches visits every row exactly once under concurrent inserts of higher keys; fails if Batches uses OFFSET.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

