# momo-12 Task 12: Allocation gate in CI with benchstat

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 12: Allocation gate in CI with benchstat (`.procoder/plans/momo-12-performance.md:1482`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-58. Files: `bench/alloc_test.go`, `bench/cmd/benchgate/main.go`, `bench/baseline/allocs.txt`, `.github/workflows/bench-gate.yml`, `bench/gate_config_test.go`.

## Acceptance criteria

- [ ] `TestBenchGateConfig` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestBenchGate` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-58] `TestBenchGateConfig` asserts the CI benchmark job runs benchstat against the stored baseline and fails on allocation-count regressions beyond the documented threshold; fails if the job only reports timings.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

