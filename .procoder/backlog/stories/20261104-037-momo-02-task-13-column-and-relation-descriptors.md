# momo-02 Task 13: Column and relation descriptors in the per-model subpackage

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 13: Column and relation descriptors in the per-model subpackage (`.procoder/plans/momo-02-generator.md:1250`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3, S-43. Files: `gen/emit_descriptors.go`, `gen/emit_descriptors_test.go`, `gen/relations.go`, `gen/relations_test.go`, `gen/testdata/golden/descriptors/document/momo_gen.go`, `gen/testdata/fixtures/rel/rel.go` ….

## Acceptance criteria

- [ ] `TestRelationMetadata` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEmitDescriptorsGolden` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [ ] [S-43] `TestColumnByName` rejects unknown names and returns the typed column for known ones; `TestEscapeLike` matches literal `%`, `_` and the escape character on both dialects; fails if a name is ever quoted into SQL without the allowlist.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

