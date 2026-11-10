# momo-12 Task 9: Recall-matched vector benchmarks

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-12-performance Task 9: Recall-matched vector benchmarks (`.procoder/plans/momo-12-performance.md:1107`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-19. Files: `bench/vector/dataset.go`, `bench/vector/recall.go`, `bench/vector/sweep.go`, `bench/vector/recall_test.go`. Contributes to S-15, S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestRecallMatchedSweep` exists as written in the plan, failed before the implementation, and passes now
- [x] `BenchmarkVectorTopK` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestRecallMatchedSweep (bench/vector/recall_integration_test.go) loads 2000 seeded vectors of dimension 32 with an HNSW index, reaches recall 0.95 at k=10 and picks the same ef_search twice; unit tests for Recall, Generate and Sweep in recall_test.go. BenchmarkVectorTopK (topk_integration_test.go) compares the momo builder and a handwritten statement at the swept ef_search and reports recall@10; it ran in the smoke job.
  Red proof: ci-agent run 42465164717 (headSha 0c7f14e) on branch worktree-agent-a51f3a530eba5c020-adapters-red, which keeps the tests and removes the implementations: bench-module fails with `no required module provides package` for the removed adapter packages and unit fails with `momo.InsertQuery[suite.Document] has no field or method Unnest`.
  Green proof: ci-agent run 42465734475, headSha eaec549861a73104452a68df580ea15421eac7fb, all eight jobs green (lint, unit, fuzz-smoke, integration postgres, integration mysql84/mysql97/mariadb, bench-module).
- Deviation: Sweep takes a DB interface with TopK, WithSearchEf and WithExact instead of the plan's QueryIDs form, so exact and approximate results use the same call.
  Gate: commits went through the procoder commit hook, with a hand-written go.work (untracked) so the hook can lint the nested bench module.
