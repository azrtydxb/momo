# momo-06 Task 6: Has-many loading, stable order, parent pagination and NULL-key emptiness

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 6: Has-many loading, stable order, parent pagination and NULL-key emptiness (`.procoder/plans/momo-06-relations.md:2153`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6. Files: `load_test.go`, `load.go`.

## Acceptance criteria

- [ ] `TestEagerLoadHasMany` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEagerLoadParentOrderAndPagination` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEagerLoadNullFKEmpty` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEagerLoadReloadClearsStaleChildren` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

