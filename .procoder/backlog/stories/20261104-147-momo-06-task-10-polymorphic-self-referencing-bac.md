# momo-06 Task 10: Polymorphic, self-referencing, back-references and explicit join model

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 10: Polymorphic, self-referencing, back-references and explicit join model (`.procoder/plans/momo-06-relations.md:2540`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-82. Files: `load_test.go`.

## Acceptance criteria

- [x] `TestPolymorphicAndThrough` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-82] `TestPolymorphicAndThrough` asserts correct association per type discriminator and join-row attributes; fails if a polymorphic load mixes types.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: with the discriminator predicate removed from `writeScope`, TestPolymorphicAndThrough failed with `document images: [{... OwnerType:document ... u1} {... OwnerType:author ... u2}]` (types mixed). With the discriminator in place (sprint-2 code) it passes; the plan's `Then` call needed `LoadAll` (see plan note).
- Green: `go test -tags integration -race -count=1 -run 'Polymorphic|ManyToMany|Load|Eager' . ./internal/...` ok against Postgres; `go test -race ./...` ok; make fmt-check vet staticcheck gosec tidy-check clean.
- Tests: TestPolymorphicAndThrough (document vs author images, join rows with Position and nested Tag, self-ref Reports back-reference and Manager belongs-to), TestPolymorphicChunkingKeepsDiscriminator (WithMaxParams).
- Mutations killed: no discriminator predicate; discriminator bind not budgeted (statement count changes under the limit).
