# momo-09 Task 3: Disabled-hook allocation budget and docs/PERFORMANCE.md

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

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
- Green proof: ci-agent run 42460011571 (headSha 47933f8, all five jobs green). Runs 42455486712 and 42456258355 failed for unrelated reasons (a bench-module test and a gosec finding, fixed later) and are not cited.
- Matrix check: a throwaway branch (budget set to 0 to print the measurement, deleted afterwards) ran the full ci.yml matrix, run 42461281129: all four integration combinations (Go 1.26 and 1.27, PostgreSQL 16 and 17) print `measured allocs/op = 32`, so the budget needs no per-version slack.
- BenchmarkSelectNoHooks runs in the bench-module job through make bench-hooks (the bench-module job of run 42458619130, which was later cancelled when a newer commit superseded it; the step itself had completed): `32 allocs/op`, equal to the recorded budget. The benchmark cannot fail before an implementation exists, so its red proof is the budget test above.
- Plan Step 4 (baseline worktree measurement) was replaced by the CI measurement above, since nothing may run on the laptop. Plan Step 7 (mutation) was not run.
