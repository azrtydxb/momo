# momo-06 Task 16: Relation predicates, traversal and count ordering

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 16: Relation predicates, traversal and count ordering (`.procoder/plans/momo-06-relations.md:3589`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-76. Files: `relpred.go`, `relpred_test.go`.

## Acceptance criteria

- [ ] `TestRelationPredicates` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestRelationTraverseAndOrder` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-76] `TestRelationPredicates` asserts SQL shape (EXISTS) and rows for each; fails if a predicate duplicates parents via JOIN.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

