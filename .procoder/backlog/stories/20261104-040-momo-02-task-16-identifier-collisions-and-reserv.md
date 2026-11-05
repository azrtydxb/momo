# momo-02 Task 16: Identifier collisions and reserved words

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 16: Identifier collisions and reserved words (`.procoder/plans/momo-02-generator.md:1690`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/ident.go`, `gen/ident_test.go`, `gen/testdata/fixtures/collide/collide.go`, `gen/testdata/fixtures/collide/sub/sub.go`.

## Acceptance criteria

- [ ] `TestGenerateNameCollisions` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-3, part] `TestGenerateNameCollisions` passes for the part of S-3 this task delivers; the full criterion is verified in story 043.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

