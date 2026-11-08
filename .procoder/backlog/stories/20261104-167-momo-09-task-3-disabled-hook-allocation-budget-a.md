# momo-09 Task 3: Disabled-hook allocation budget and docs/PERFORMANCE.md

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 3: Disabled-hook allocation budget and docs/PERFORMANCE.md (`.procoder/plans/momo-09-observability-testing.md:599`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18. Files: `obs_hooks_bench_test.go`, `obs_budget_test.go`, `docs/PERFORMANCE.md`.

## Acceptance criteria

- [x] `BenchmarkSelectNoHooks` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSelectNoHooksAllocBudget` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestPerformanceDocRecordsBudget` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-18, part] `BenchmarkSelectNoHooks` passes for the part of S-18 this task delivers; the full criterion is verified in story 169.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red proof: ci-agent run 42454720294 on branch worktree-agent-a51f3a530eba5c020-red (96b7b1b, before the implementation): TestSelectNoHooksAllocBudget fails with `measured allocs/op = 32` then `allocs/op 32 exceeds budget 0`, and TestPerformanceDocRecordsBudget fails in the unit and integration jobs because docs/PERFORMANCE.md did not exist.
- Green proof: ci-agent run 42458869175 on the final commit; the budget 32 also passed on runs 42455486712, 42456258355 and 42457722231.
- BenchmarkSelectNoHooks runs in the bench-module job through make bench-hooks (run 42458619130): `32 allocs/op`, equal to the recorded budget. The benchmark cannot fail before an implementation exists, so its red proof is the budget test above.
- Plan Step 4 (baseline worktree measurement) was replaced by the CI measurement above, since nothing may run on the laptop. Plan Step 7 (mutation) was not run.
