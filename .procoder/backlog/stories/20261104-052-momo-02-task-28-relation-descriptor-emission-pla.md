# momo-02 Task 28: Relation descriptor emission (plan momo-06 Task 2 shape, polymorphic)

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 28: Relation descriptor emission (plan momo-06 Task 2 shape, polymorphic) (`.procoder/plans/momo-02-generator.md:3080`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/relation_shape_test.go`, `gen/testdata/fixtures/poly/poly.go`, `gen/testdata/fixtures/polybad/bad.go`, `gen/testdata/golden/poly/document_descriptors.go`, `gen/emit_descriptors.go`, `gen/relations.go` …. Depends on momo-06 Task 2 (story 139): relation descriptors use `momo.Relation`.

## Acceptance criteria

- [ ] `TestRelationDescriptorShape` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPolymorphicDescriptorGolden` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPolymorphicDiagnostics` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

