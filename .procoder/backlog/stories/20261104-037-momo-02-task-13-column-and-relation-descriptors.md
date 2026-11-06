# momo-02 Task 13: Column and relation descriptors in the per-model subpackage

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 13: Column and relation descriptors in the per-model subpackage (`.procoder/plans/momo-02-generator.md:1250`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3, S-43. Files: `gen/emit_descriptors.go`, `gen/emit_descriptors_test.go`, `gen/relations.go`, `gen/relations_test.go`, `gen/testdata/golden/descriptors/document/momo_gen.go`, `gen/testdata/fixtures/rel/rel.go` …. Contributes to S-3, S-43; those criteria are verified in the stories that define their named tests. Depends on momo-06 Task 2 (story 139): relation descriptors use `momo.Relation`.

## Acceptance criteria

- [x] `TestRelationMetadata` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEmitDescriptorsGolden` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/emit_descriptors.go, gen/relations.go, gen/types.go, gen/model.go, gen/load.go, tests gen/emit_descriptors_test.go, gen/relations_test.go, gen/relations_emit_test.go, golden gen/testdata/golden/descriptors/document/momo_gen.go, fixture gen/testdata/fixtures/rel/rel.go. Commit b2edd5a.
- FAIL before: go test ./gen build failed: undefined: EmitDescriptors; Relations[0].Kind undefined; undefined: HasMany. PASS after: TestRelationMetadata, TestEmitDescriptorsGolden, plus TestRelationDiagnostics and TestDescriptorsCompile (all relation kinds generated into a scratch dir, then go build).
- Mutation: dropping the named-type conversion in generated key setters failed TestDescriptorsCompile (cannot use &v0 as *models.Status); reverted.
- Gate: go build, go vet, go test -race -count=1 ./... pass; procoder check: 0 blocking findings.
