# momo-05 Task 25: Window functions, partitioned and ordered (S-33, window part only)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 25: Window functions, partitioned and ordered (S-33, window part only) (`.procoder/plans/momo-05-writes-queries.md:2080`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-33. Files: `window.go`, `window_test.go`.

## Acceptance criteria

- [x] Issue 33: window functions (Over with PartitionBy, OrderBy, frames; RowNumber, Rank, DenseRank, Lag, Lead), aggregate FILTER, DISTINCT ON with the ORDER BY prefix rule, LATERAL joins and unnest/generate_series FROM sources, with ErrUnsupported on dialects that lack them

- [x] `TestWindowFunctions` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-33, part] `TestWindowFunctions` passes for the part of S-33 this task delivers; the full criterion is verified in story 149.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Commit 8eece1e on branch worktree-agent-a78cf9b5b1e09d5cf: goldens and error cases in analytics_test.go, integration tests in analytics_integration_test.go; CI run 42409618987 green.

