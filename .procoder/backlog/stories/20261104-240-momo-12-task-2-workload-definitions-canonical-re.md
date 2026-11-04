# momo-12 Task 2: Workload definitions, canonical results and the Impl contract

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 2: Workload definitions, canonical results and the Impl contract (`.procoder/plans/momo-12-performance.md:129`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/workload/model.go`, `bench/workload/impl.go`, `bench/workload/seed.go`, `bench/workload/sql.go`, `bench/workload/canonical_test.go`. Contributes to S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestCanonical` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestWorkloadSQLSemantics` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

