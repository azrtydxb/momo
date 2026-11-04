# momo-07 Task 4: Vector index declarations and validation

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: -

## Description

Implements plan momo-07-vectors Task 4: Vector index declarations and validation (`.procoder/plans/momo-07-vectors.md:681`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-15. Files: `schema/vector.go`, `schema/vector_test.go`, `c.Index(...)`.

## Acceptance criteria

- [ ] `TestVectorIndexRules` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-12] `TestPostgresObjects` on real PostgreSQL+pgvector: vector extension, enum, partial index, expression index, HNSW index with opclass and a CONCURRENTLY index apply and round-trip with no diff; fails if CONCURRENTLY runs inside a transaction.
- [ ] [S-15] `TestVectorCodec` + `FuzzVectorDecode`: vector, halfvec, sparsevec and bit round-trip; wrong dimension returns ErrInvalidVector before any SQL runs; `TestVectorDistances` matches independently computed values to 1e-5; `TestVectorIndexUsed` asserts EXPLAIN shows the HNSW/IVFFlat index; `TestSearchSettingsScoped` asserts `hnsw.ef_search` resets after the transaction; fails if settings leak to the p…
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

