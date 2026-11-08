# momo-09 Task 12: N+1 detector

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 12: N+1 detector (`.procoder/plans/momo-09-observability-testing.md:2176`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-53. Files: `nplusone.go`, `nplusone_internal_test.go`, `obs_nplusone_test.go`.

## Acceptance criteria

- [x] `TestNormalizeShape` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestNPlusOneHook` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-53] `TestNPlusOneHook` reports a loop of per-parent queries and stays silent for batched loading; fails if the detector misses the loop.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestNormalizeShape`, `TestNPlusOneHook`: red run 42409637729 (tests committed first in fa87dbf, build failed with undefined symbols), green run 42410680387 on 8d0f6d7.
- Mutation: deleting the IN-list collapse made `TestNormalizeShape` fail with the uncollapsed IN list (run 42415019735, fdbf299, reverted).
- Gate: lint, unit and integration jobs green on CI.
- Red here is a build failure (undefined normalizeShape, NewNPlusOneHook, WithQueryScope) plus the mutation run; shape cap and recovered reporter are pinned by unit tests in nplusone_internal_test.go.
