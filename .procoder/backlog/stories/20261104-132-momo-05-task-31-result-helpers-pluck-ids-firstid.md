# momo-05 Task 31: Result helpers: Pluck, IDs, FirstID, scalars, maps, FindAndCount, Batches (S-78)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 31: Result helpers: Pluck, IDs, FirstID, scalars, maps, FindAndCount, Batches (S-78) (`.procoder/plans/momo-05-writes-queries.md:2554`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-78. Files: `results.go`, `results_test.go`.

## Acceptance criteria

- [x] `TestResultHelpers` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-78] `TestResultHelpers` asserts each helper's results and that Batches visits every row exactly once under concurrent inserts of higher keys; fails if Batches uses OFFSET.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Green: ci-agent run 42453190263 on d8088d5 (unit, lint, fuzz-smoke, integration postgres).
- Gate: the commit hook ran the procoder format and lint gate on every commit of this story with 0 blocking findings.
- Red: run 42453213159 on throwaway branch worktree-agent-a1e9f39ea609a17b8-red (tests kept, implementation reverted) failed the build with undefined symbols for this story's API.
- The red run is a build failure (the API did not exist), not a behavioural assertion failure. FindAndCount drops the row lock and After for the count (TestFindAndCountDropsLock).
