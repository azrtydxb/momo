# momo-02 Task 28: Relation descriptor emission (plan momo-06 Task 2 shape, polymorphic)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 28: Relation descriptor emission (plan momo-06 Task 2 shape, polymorphic) (`.procoder/plans/momo-02-generator.md:3080`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/relation_shape_test.go`, `gen/testdata/fixtures/poly/poly.go`, `gen/testdata/fixtures/polybad/bad.go`, `gen/testdata/golden/poly/document_descriptors.go`, `gen/emit_descriptors.go`, `gen/relations.go` …. Depends on momo-06 Task 2 (story 139): relation descriptors use `momo.Relation`.

## Acceptance criteria

- [x] `TestRelationDescriptorShape` exists as in the plan (with two test-side fixes, see Evidence), failed before the fixes, and passes now
- [x] `TestPolymorphicDescriptorGolden` exists as written in the plan, failed before the golden file existed (differs from golden), and passes now
- [x] `TestPolymorphicDiagnostics` exists as written in the plan and passes now (NOT red-first: the diagnostics were already implemented by earlier tasks)
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- The emitter (`gen/emit_descriptors.go`) and discovery with the three diagnostics (`gen/relations.go`) already implemented the full momo-06 Task 2 shape and the polymorphic rules from earlier momo-02 tasks, so this task added only the fixtures, the tests and the golden file.
- Red-first: `go test ./gen -run 'TestRelationDescriptorShape|TestPolymorphic'` failed before the fixes below (`"Name:" appears 3 times, want 1`, a load error for the example module, and `polymorphic descriptors differ from golden`). The plan's test text had two defects, fixed in the test only: the example module path is one level too high from `gen/` (now `../internal/testmodels/example`), and the label counts matched substrings inside `TableRef{Name: ...}` and gofmt-aligned columns (labels are now matched at line start on whitespace-normalised text). The golden was generated with `-update` and reviewed; it equals the plan's expected literal exactly.
- `Field.Base` from the plan was not added: `Field.Base` already exists with another meaning and the emitter derives the base Go type per key part, so nothing needs it.
- Green: `go test ./...` passes, `go vet ./...` and `go vet -tags integration ./...` clean, `momo generate --check` in `internal/testmodels/example` clean.

