# momo-02 Task 14: MomoMeta emission in the model package

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 14: MomoMeta emission in the model package (`.procoder/plans/momo-02-generator.md:1444`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/emit_meta.go`, `gen/emit_meta_test.go`, `gen/testdata/golden/meta/models/momo_gen.go`.

## Acceptance criteria

- [ ] `TestEmitMetaGolden` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

