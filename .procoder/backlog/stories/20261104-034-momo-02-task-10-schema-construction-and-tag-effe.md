# momo-02 Task 10: Schema construction and tag effects

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 10: Schema construction and tag effects (`.procoder/plans/momo-02-generator.md:922`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-70. Files: `gen/schema.go`, `gen/schema_test.go`, `gen/load.go`. Contributes to S-1; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestTagVocabulary` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-70] `TestTagVocabulary` asserts each tag reaches the schema, migration SQL and inspection round trip on every dialect or yields a dialect diagnostic; fails if a tag is ignored silently.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestTagVocabulary failed first with `undefined: BuildSchema`; after implementation `go test ./gen` passes, and `go test -race -count=1 ./...` passes.
- Mutation check: dropping the tag default from the column made 2 tests fail; restored.
- `procoder check`: 0 blocking findings. Commit de5d24c.
- S-70 criterion left unchecked: the schema half and sqlite diagnostics are tested, but migration SQL and inspection round trip belong to later plans.
