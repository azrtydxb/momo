# momo-07 Task 5: PostgreSQL vector DDL round trip and extension-missing error

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 5: PostgreSQL vector DDL round trip and extension-missing error (`.procoder/plans/momo-07-vectors.md:785`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-15. Files: `schema/normalize_test.go`, `migrate/postgres/vector_roundtrip_test.go`. Contributes to S-12, S-15; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestNormalizeVectorTypes` exists as written in the plan; it passed on first run because sprint-2 normalization already lowercases and strips whitespace for vector types (no FAIL to record), and passes now
- [x] `TestVectorMigrationRoundTrip` exists (driven by Diff, OrderWithViews, Render, exec, Inspect, schema.Equal, empty second Diff instead of the Engine, since Engine Apply is story 085); mutation check proved it fails when the operator class is dropped from rendered DDL, and passes now
- [x] `TestVectorExtensionMissing` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `go test ./schema -run TestNormalizeVectorTypes`: ok.
- `MOMO_REQUIRE_INTEGRATION=1 ... go test -tags integration -race -count=1 ./migrate/postgres ./schema`: ok. Added `TestVectorIndexMatrixRoundTrip` covering vector, halfvec, sparsevec and bit with every valid hnsw/ivfflat method and opclass of the live pgvector (24 tables, one schema).
- Mutation: removing the opclass from `createIndex` makes both round-trip tests fail with "data type vector has no default operator class for access method hnsw"; reverted.
- `TestVectorExtensionMissing` (red first): in a scratch database without pgvector, applying a migration that declares vector(3) failed with the plain driver error "permission denied to create extension "vector" (SQLSTATE 42501), want momo.ErrUnsupported"; after `Dialect.ClassifyError` (`migrate/postgres/classify.go`, applied by `Engine.Apply` and `AutoMigrate` through `momo.ErrorClassifier`) it returns `momo.ErrUnsupported` with `extension "vector" is not installed`. Deviation from the plan text: the test connects as the application role, not the administrator, because an administrator's CREATE EXTENSION simply installs pgvector (the package is on the server) and nothing fails. The shared database is untouched. `TestClassifyMissingExtension` covers the not-available, permission and missing-type forms and that unrelated errors pass through unchanged.
- `go build ./...`, `go vet ./...`, `make staticcheck` clean.

