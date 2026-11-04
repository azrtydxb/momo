# momo-07 Task 5: PostgreSQL vector DDL round trip and extension-missing error

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: -

## Description

Implements plan momo-07-vectors Task 5: PostgreSQL vector DDL round trip and extension-missing error (`.procoder/plans/momo-07-vectors.md:785`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-15. Files: `schema/normalize_test.go`, `migrate/postgres/vector_roundtrip_test.go`.

## Acceptance criteria

- [ ] `TestNormalizeVectorTypes` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestVectorMigrationRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestVectorExtensionMissing` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-12] `TestPostgresObjects` on real PostgreSQL+pgvector: vector extension, enum, partial index, expression index, HNSW index with opclass and a CONCURRENTLY index apply and round-trip with no diff; fails if CONCURRENTLY runs inside a transaction.
- [ ] [S-15] `TestVectorCodec` + `FuzzVectorDecode`: vector, halfvec, sparsevec and bit round-trip; wrong dimension returns ErrInvalidVector before any SQL runs; `TestVectorDistances` matches independently computed values to 1e-5; `TestVectorIndexUsed` asserts EXPLAIN shows the HNSW/IVFFlat index; `TestSearchSettingsScoped` asserts `hnsw.ef_search` resets after the transaction; fails if settings leak to the p…
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

