# momo-09 Task 12: N+1 detector

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 12: N+1 detector (`.procoder/plans/momo-09-observability-testing.md:2176`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-53. Files: `nplusone.go`, `nplusone_internal_test.go`, `obs_nplusone_test.go`.

## Acceptance criteria

- [ ] `TestNormalizeShape` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestNPlusOneHook` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-53] `TestNPlusOneHook` reports a loop of per-parent queries and stays silent for batched loading; fails if the detector misses the loop.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

