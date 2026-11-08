# IDs: Go-side id generators (cuid, uuid, ulid) declared in the model

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

GitHub issue #35, found by the NovaCRM port (gap audit item N1). Prisma databases keep text primary keys that the client generates (`String @id @default(cuid())`); NovaCRM has 113 of them with no database default. FieldHook.Default never replaces a zero value, so every Model insert needs Omit on the id. A model should declare its generator once, with the tag idgen:cuid, idgen:uuid or idgen:ulid or the MomoConfig call IDGen, and an insert fills the field when it holds the zero value and writes the generated id back into the caller's struct. WithIDGenerator registers a custom or replacement generator. Done means every model insert path generates ids, the generator emits the metadata, and docs describe it.

## Acceptance criteria

- [x] the idgen tag and MomoConfig IDGen call are parsed and emitted as Meta.IDGens; conflicting, non-string or badly named declarations are generator diagnostics (gen tests)
- [x] Model, Models, Upsert and every chunk of a bulk insert fill a zero id and keep an assigned one (unit test on the fake driver)
- [x] the generated id is written back into the passed struct or slice element, for string, named string and *string fields (unit test)
- [x] cuid, uuid and ulid ids have the documented format and are unique across thousands of calls; ulids sort in creation order (unit test)
- [x] WithIDGenerator registers a custom generator and can replace a built-in; an unknown generator and an empty id are ErrValidation (unit test)
- [x] rows inserted with zero ids read back by the written-back ids on a real database (integration test on CI)
- [x] docs/ai-guide.md and docs/capabilities.md describe the feature

## Evidence

GitHub Actions run 42457247159 on a66875d (merged with origin/sprint-5 and the insert refactor): unit, lint, fuzz-smoke and integration (postgres) all success. Unit tests in idgen_test.go, idgen_writeback_test.go, idgen_conflict_test.go, idgen_internal_test.go (race-safe concurrent generation, ULID overflow), gen/idgen_test.go, gen/idgen_unit_test.go and gen/tags/tags_test.go cover criteria 1 to 5. Integration tests idgen_integration_test.go, idgen_upsert_integration_test.go and idgen_conflict_integration_test.go (conflict upserts, duplicate keys, chunked upsert, non-PK ids, non-round-tripping target) cover criterion 6. docs/ai-guide.md and docs/capabilities.md cover criterion 7.
