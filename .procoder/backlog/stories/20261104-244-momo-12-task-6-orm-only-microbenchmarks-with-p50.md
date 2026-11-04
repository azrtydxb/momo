# momo-12 Task 6: ORM-only microbenchmarks with p50, p95 and p99

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 6: ORM-only microbenchmarks with p50, p95 and p99 (`.procoder/plans/momo-12-performance.md:609`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/stats/percentile.go`, `bench/micro_test.go`.

## Acceptance criteria

- [ ] `TestPercentiles` exists as written in the plan, failed before the implementation, and passes now
- [ ] `BenchmarkMicro` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-19] `make bench` reproduces published results; bench/results contains raw output, environment and pinned versions; `TestBenchEquivalence` asserts each compared ORM returns identical rows per workload; fails if a workload's SQL semantics differ between ORMs.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

