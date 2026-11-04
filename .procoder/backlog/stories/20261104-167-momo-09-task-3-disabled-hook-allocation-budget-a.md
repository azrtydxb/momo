# momo-09 Task 3: Disabled-hook allocation budget and docs/PERFORMANCE.md

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 3: Disabled-hook allocation budget and docs/PERFORMANCE.md (`.procoder/plans/momo-09-observability-testing.md:599`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18. Files: `obs_hooks_bench_test.go`, `obs_budget_test.go`, `docs/PERFORMANCE.md`.

## Acceptance criteria

- [ ] `BenchmarkSelectNoHooks` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSelectNoHooksAllocBudget` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPerformanceDocRecordsBudget` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-18, part] `BenchmarkSelectNoHooks` passes for the part of S-18 this task delivers; the full criterion is verified in story 169.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

