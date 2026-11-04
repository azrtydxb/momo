# momo-06 Task 4: Tuple IN predicate and bind-limit budget

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 4: Tuple IN predicate and bind-limit budget (`.procoder/plans/momo-06-relations.md:1376`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6, S-77. Files: `in_tuple.go`, `in_tuple_internal_test.go`.

## Acceptance criteria

- [ ] `TestTupleInRendering` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-6] `TestEagerLoad*` (one per relation kind incl. composite keys): a counting hook asserts the exact query count, chunking over the bind limit, parent pagination and order preserved without duplicates, NULL-FK parents left empty; fails if loading issues per-parent queries or joins that duplicate parents.
- [ ] [S-77] `TestRawArgs` and `FuzzRawArgs` assert named and expanded placeholders bind correctly per dialect and identifiers are quoted; fails if an identifier argument is bound as a value or a value is interpolated.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

