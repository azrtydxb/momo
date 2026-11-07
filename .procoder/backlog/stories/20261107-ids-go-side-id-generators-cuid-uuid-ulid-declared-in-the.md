# IDs: Go-side id generators (cuid, uuid, ulid) declared in the model

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

GitHub issue #35, found by the NovaCRM port (gap audit item N1). Prisma databases keep text primary keys that the client generates (`String @id @default(cuid())`); NovaCRM has 113 of them with no database default. FieldHook.Default never replaces a zero value, so every Model insert needs Omit on the id. A model should declare its generator once, with the tag idgen:cuid, idgen:uuid or idgen:ulid or the MomoConfig call IDGen, and an insert fills the field when it holds the zero value and writes the generated id back into the caller's struct. WithIDGenerator registers a custom or replacement generator. Done means every model insert path generates ids, the generator emits the metadata, and docs describe it.

## Acceptance criteria

- [ ] the idgen tag and MomoConfig IDGen call are parsed and emitted as Meta.IDGens; conflicting, non-string or badly named declarations are generator diagnostics (gen tests)
- [ ] Model, Models, Upsert and every chunk of a bulk insert fill a zero id and keep an assigned one (unit test on the fake driver)
- [ ] the generated id is written back into the passed struct or slice element, for string, named string and *string fields (unit test)
- [ ] cuid, uuid and ulid ids have the documented format and are unique across thousands of calls; ulids sort in creation order (unit test)
- [ ] WithIDGenerator registers a custom generator and can replace a built-in; an unknown generator and an empty id are ErrValidation (unit test)
- [ ] rows inserted with zero ids read back by the written-back ids on a real database (integration test on CI)
- [ ] docs/ai-guide.md and docs/capabilities.md describe the feature

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
