# momo-12 Task 7: Real-database mixed workload with concurrency

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 7: Real-database mixed workload with concurrency (`.procoder/plans/momo-12-performance.md:756`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/loadgen/loadgen.go`, `bench/cmd/loadgen/main.go`, `bench/loadgen/loadgen_test.go`.

## Acceptance criteria

- [ ] `TestLoadgenOutputSchema` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-19] `make bench` reproduces published results; bench/results contains raw output, environment and pinned versions; `TestBenchEquivalence` asserts each compared ORM returns identical rows per workload; fails if a workload's SQL semantics differ between ORMs.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

