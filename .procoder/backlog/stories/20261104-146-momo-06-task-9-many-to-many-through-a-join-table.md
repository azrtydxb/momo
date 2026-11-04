# momo-06 Task 9: Many-to-many through a join table

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 9: Many-to-many through a join table (`.procoder/plans/momo-06-relations.md:2412`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6, S-82. Files: `load.go`, `load_test.go`.

## Acceptance criteria

- [ ] `TestEagerLoadManyToMany` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-6] `TestEagerLoad*` (one per relation kind incl. composite keys): a counting hook asserts the exact query count, chunking over the bind limit, parent pagination and order preserved without duplicates, NULL-FK parents left empty; fails if loading issues per-parent queries or joins that duplicate parents.
- [ ] [S-82] `TestPolymorphicAndThrough` asserts correct association per type discriminator and join-row attributes; fails if a polymorphic load mixes types.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

