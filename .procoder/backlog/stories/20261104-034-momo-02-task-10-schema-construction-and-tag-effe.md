# momo-02 Task 10: Schema construction and tag effects

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 10: Schema construction and tag effects (`.procoder/plans/momo-02-generator.md:922`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-70. Files: `gen/schema.go`, `gen/schema_test.go`, `gen/load.go`.

## Acceptance criteria

- [ ] `TestTagVocabulary` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-1] `TestSchemaJSONRoundTrip` and `FuzzSchemaSort` (schema package): JSON round-trips losslessly and `Sort` makes equivalent schemas byte-identical; fails if a field is dropped from serialization or Sort is order-sensitive.
- [ ] [S-70] `TestTagVocabulary` asserts each tag reaches the schema, migration SQL and inspection round trip on every dialect or yields a dialect diagnostic; fails if a tag is ignored silently.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

