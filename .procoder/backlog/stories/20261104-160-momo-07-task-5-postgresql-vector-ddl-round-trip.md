# momo-07 Task 5: PostgreSQL vector DDL round trip and extension-missing error

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 5: PostgreSQL vector DDL round trip and extension-missing error (`.procoder/plans/momo-07-vectors.md:785`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-15. Files: `schema/normalize_test.go`, `migrate/postgres/vector_roundtrip_test.go`. Contributes to S-12, S-15; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestNormalizeVectorTypes` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestVectorMigrationRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestVectorExtensionMissing` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

