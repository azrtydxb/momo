# momo-02 Task 2: Struct tag grammar with the full S-70 vocabulary

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 2: Struct tag grammar with the full S-70 vocabulary (`.procoder/plans/momo-02-generator.md:154`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-70. Files: `gen/tags/tags.go`, `gen/tags/tags_test.go`. Contributes to S-70; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestParseTags` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestTagGrammar` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

