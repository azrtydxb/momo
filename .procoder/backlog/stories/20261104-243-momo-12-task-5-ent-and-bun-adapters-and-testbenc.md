# momo-12 Task 5: Ent and Bun adapters and TestBenchEquivalence

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 5: Ent and Bun adapters and TestBenchEquivalence (`.procoder/plans/momo-12-performance.md:459`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/adapters/ent/schema/customer.go`, `bench/adapters/ent/ent.go`, `bench/adapters/bunad/bun.go`, `bench/equivalence_test.go`, `bench/impls_test.go`.

## Acceptance criteria

- [ ] `TestBenchEquivalence` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-19] `make bench` reproduces published results; bench/results contains raw output, environment and pinned versions; `TestBenchEquivalence` asserts each compared ORM returns identical rows per workload; fails if a workload's SQL semantics differ between ORMs.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

