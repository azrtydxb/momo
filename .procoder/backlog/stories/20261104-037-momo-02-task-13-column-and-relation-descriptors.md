# momo-02 Task 13: Column and relation descriptors in the per-model subpackage

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 13: Column and relation descriptors in the per-model subpackage (`.procoder/plans/momo-02-generator.md:1250`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3, S-43. Files: `gen/emit_descriptors.go`, `gen/emit_descriptors_test.go`, `gen/relations.go`, `gen/relations_test.go`, `gen/testdata/golden/descriptors/document/momo_gen.go`, `gen/testdata/fixtures/rel/rel.go` …. Contributes to S-3, S-43; those criteria are verified in the stories that define their named tests. Depends on momo-06 Task 2 (story 139): relation descriptors use `momo.Relation`.

## Acceptance criteria

- [ ] `TestRelationMetadata` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEmitDescriptorsGolden` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

