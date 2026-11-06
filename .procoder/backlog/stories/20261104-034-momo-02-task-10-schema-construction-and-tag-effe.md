# momo-02 Task 10: Schema construction and tag effects

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 10: Schema construction and tag effects (`.procoder/plans/momo-02-generator.md:922`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-70. Files: `gen/schema.go`, `gen/schema_test.go`, `gen/load.go`. Contributes to S-1; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestTagVocabulary` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-70] `TestTagVocabulary` asserts each tag reaches the schema, migration SQL and inspection round trip on every dialect or yields a dialect diagnostic; fails if a tag is ignored silently.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestTagVocabulary failed first with `undefined: BuildSchema`; after implementation `go test ./gen` passes, and `go test -race -count=1 ./...` passes.
- Mutation check: dropping the tag default from the column made 2 tests fail; restored.
- `procoder check`: 0 blocking findings. Commit de5d24c.
- S-70: `TestTagVocabulary` (gen) covers the schema half and the sqlite diagnostics; `TestTagVocabularyRoundTrip` (`migrate/postgres/tagvocab_roundtrip_test.go`, fixture `gen/testdata/fixtures/tagvocab`) asserts every schema-affecting tag in the generated schema, in the rendered migration SQL, and through apply to PostgreSQL then inspect (declared check text canonicalized via the dialect, as the engine does for drift), with an empty second diff. Mutation check: dropping Collate and OnUpdate from the generator each fail the test. No generator/Inspect mismatch was found; PostgreSQL deparses BETWEEN so the declared CHECK needs canonicalization, which is the engine's design.
- sqlite and MySQL: sqlite yields the dialect diagnostic (tested); MySQL has no driver yet, so its half lands with that plan.
