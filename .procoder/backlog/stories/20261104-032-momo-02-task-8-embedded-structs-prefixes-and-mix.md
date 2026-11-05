# momo-02 Task 8: Embedded structs, prefixes and mixins

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 8: Embedded structs, prefixes and mixins (`.procoder/plans/momo-02-generator.md:687`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-67. Files: `gen/embed.go`, `gen/embed_test.go`, `gen/testdata/fixtures/mixins/mixins.go`, `gen/model.go`.

## Acceptance criteria

- [ ] `TestEmbeddedAndMixins` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-67] `TestEmbeddedAndMixins` (generator golden + integration) asserts prefixed columns and mixin indexes exist and round-trip; fails if an embedded field is skipped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

