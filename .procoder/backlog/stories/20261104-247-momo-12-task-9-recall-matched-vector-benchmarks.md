# momo-12 Task 9: Recall-matched vector benchmarks

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 9: Recall-matched vector benchmarks (`.procoder/plans/momo-12-performance.md:1107`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-19. Files: `bench/vector/dataset.go`, `bench/vector/recall.go`, `bench/vector/sweep.go`, `bench/vector/recall_test.go`. Contributes to S-15, S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestRecallMatchedSweep` exists as written in the plan, failed before the implementation, and passes now
- [ ] `BenchmarkVectorTopK` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

