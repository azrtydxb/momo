# momo-06 Task 10: Polymorphic, self-referencing, back-references and explicit join model

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 10: Polymorphic, self-referencing, back-references and explicit join model (`.procoder/plans/momo-06-relations.md:2540`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-82. Files: `load_test.go`.

## Acceptance criteria

- [ ] `TestPolymorphicAndThrough` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-82] `TestPolymorphicAndThrough` asserts correct association per type discriminator and join-row attributes; fails if a polymorphic load mixes types.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

