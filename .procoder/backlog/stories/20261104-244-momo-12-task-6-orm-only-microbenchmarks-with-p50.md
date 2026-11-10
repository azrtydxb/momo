# momo-12 Task 6: ORM-only microbenchmarks with p50, p95 and p99

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-12-performance Task 6: ORM-only microbenchmarks with p50, p95 and p99 (`.procoder/plans/momo-12-performance.md:609`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/stats/percentile.go`, `bench/micro_test.go`. Contributes to S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestPercentiles` exists as written in the plan, failed before the implementation, and passes now
- [x] `BenchmarkMicro` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestPercentiles (plus TestPercentileEdges) in bench/stats; BenchmarkMicro in bench/micro_test.go runs six operations over six adapters with p50_ns, p95_ns and p99_ns metrics. BenchmarkMicro ran in the bench-module job through make bench-suite-smoke (benchtime 1x) and its output was captured in raw.txt by TestBenchResultsLayout.
  Red proof: ci-agent run 42465164717 (headSha 0c7f14e) on branch worktree-agent-a51f3a530eba5c020-adapters-red, which keeps the tests and removes the implementations: bench-module fails with `no required module provides package` for the removed adapter packages and unit fails with `momo.InsertQuery[suite.Document] has no field or method Unnest`.
  Green proof: ci-agent run 42465734475, headSha eaec549861a73104452a68df580ea15421eac7fb, all eight jobs green (lint, unit, fuzz-smoke, integration postgres, integration mysql84/mysql97/mariadb, bench-module).
  Gate: commits went through the procoder commit hook, with a hand-written go.work (untracked) so the hook can lint the nested bench module.
