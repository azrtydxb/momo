# momo-07 Task 5: PostgreSQL vector DDL round trip and extension-missing error

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 5: PostgreSQL vector DDL round trip and extension-missing error (`.procoder/plans/momo-07-vectors.md:785`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-15. Files: `schema/normalize_test.go`, `migrate/postgres/vector_roundtrip_test.go`. Contributes to S-12, S-15; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestNormalizeVectorTypes` exists as written in the plan; it passed on first run because sprint-2 normalization already lowercases and strips whitespace for vector types (no FAIL to record), and passes now
- [x] `TestVectorMigrationRoundTrip` exists (driven by Diff, OrderWithViews, Render, exec, Inspect, schema.Equal, empty second Diff instead of the Engine, since Engine Apply is story 085); mutation check proved it fails when the operator class is dropped from rendered DDL, and passes now
- [ ] `TestVectorExtensionMissing` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `go test ./schema -run TestNormalizeVectorTypes`: ok.
- `MOMO_REQUIRE_INTEGRATION=1 ... go test -tags integration -race -count=1 ./migrate/postgres ./schema`: ok. Added `TestVectorIndexMatrixRoundTrip` covering vector, halfvec, sparsevec and bit with every valid hnsw/ivfflat method and opclass of the live pgvector (24 tables, one schema).
- Mutation: removing the opclass from `createIndex` makes both round-trip tests fail with "data type vector has no default operator class for access method hnsw"; reverted.
- `TestVectorExtensionMissing` is NOT implemented: it needs `migrate.Engine.Apply` and the render-time missing-extension check owned by story 085 (Engine Apply). Criterion left unchecked.
- `go build ./...`, `go vet ./...`, `make staticcheck` clean.

