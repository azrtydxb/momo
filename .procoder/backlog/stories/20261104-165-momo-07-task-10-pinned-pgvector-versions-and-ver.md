# momo-07 Task 10: Pinned pgvector versions and version assertion

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: -

## Description

Implements plan momo-07-vectors Task 10: Pinned pgvector versions and version assertion (`.procoder/plans/momo-07-vectors.md:1782`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-20. Files: `internal/testdb/compose.yaml`, `internal/testdb/pgvector_version_test.go`.

## Acceptance criteria

- [ ] `TestPgvectorVersionPinned` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-15] `TestVectorCodec` + `FuzzVectorDecode`: vector, halfvec, sparsevec and bit round-trip; wrong dimension returns ErrInvalidVector before any SQL runs; `TestVectorDistances` matches independently computed values to 1e-5; `TestVectorIndexUsed` asserts EXPLAIN shows the HNSW/IVFFlat index; `TestSearchSettingsScoped` asserts `hnsw.ef_search` resets after the transaction; fails if settings leak to the p…
- [ ] [S-20] `TestCIWorkflowCoverage` parses .github/workflows/ci.yml and asserts it runs unit, generator, compile-fail, race and fuzz-smoke tests, plus integration tests against pgvector/pgvector:pg17 and SQLite+sqlite-vec, on Go 1.26 and 1.27; fails if any job is skipped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

