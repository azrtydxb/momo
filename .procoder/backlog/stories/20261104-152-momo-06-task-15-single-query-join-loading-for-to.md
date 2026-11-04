# momo-06 Task 15: Single-query join loading for to-one relations with NULL-to-nil

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 15: Single-query join loading for to-one relations with NULL-to-nil (`.procoder/plans/momo-06-relations.md:3227`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-81. Files: `joinload.go`, `scan.go`, `scan_internal_test.go`, `preload.go`, `select.go`, `preload_test.go`.

## Acceptance criteria

- [ ] `TestScanValueNullHandling` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestJoinLoadToOne` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-81] `TestNestedFilteredEagerLoad` and `TestJoinLoadToOne` assert query counts and that a missing to-one leaves nil, not a zero struct; fails if join loading duplicates parents.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

