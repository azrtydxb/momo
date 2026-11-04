# momo-02 Task 16: Identifier collisions and reserved words

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 16: Identifier collisions and reserved words (`.procoder/plans/momo-02-generator.md:1690`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/ident.go`, `gen/ident_test.go`, `gen/testdata/fixtures/collide/collide.go`, `gen/testdata/fixtures/collide/sub/sub.go`.

## Acceptance criteria

- [ ] `TestGenerateNameCollisions` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

