# momo-12 Task 4: Momo and GORM adapters

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 4: Momo and GORM adapters (`.procoder/plans/momo-12-performance.md:348`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/models/momo/customer.go`, `bench/adapters/momo/momo.go`, `bench/adapters/gormad/gorm.go`, `bench/adapters/gormad/gorm_test.go`, `bench/adapters/gormad/open.go`.

## Acceptance criteria

- [ ] `TestGormAdapter` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-19] `make bench` reproduces published results; bench/results contains raw output, environment and pinned versions; `TestBenchEquivalence` asserts each compared ORM returns identical rows per workload; fails if a workload's SQL semantics differ between ORMs.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

