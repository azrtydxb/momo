# momo-07 Task 12: Vector documentation

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-sqlite-vec
Sprint: -

## Description

Implements plan momo-07-vectors Task 12: Vector documentation (`.procoder/plans/momo-07-vectors.md:2094`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-16, S-17, S-21. Files: `docs/vectors.md`, `docs/README.md`, `vector/doc_test.go`.

## Acceptance criteria

- [ ] `TestVectorDocPins` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-15] `TestVectorCodec` + `FuzzVectorDecode`: vector, halfvec, sparsevec and bit round-trip; wrong dimension returns ErrInvalidVector before any SQL runs; `TestVectorDistances` matches independently computed values to 1e-5; `TestVectorIndexUsed` asserts EXPLAIN shows the HNSW/IVFFlat index; `TestSearchSettingsScoped` asserts `hnsw.ef_search` resets after the transaction; fails if settings leak to the p…
- [ ] [S-16] `TestSQLiteVectorParity`: sqlite-vec distance queries return the same top-k ids as pgvector exact search on the same data; requesting an ANN index on SQLite returns ErrUnsupported; fails if SQLite silently accepts an index option.
- [ ] [S-17] `TestHybridSearch`: a fixed corpus yields the asserted ordering with documented candidate counts and RRF k; fails if fusion is order-dependent on ties.
- [ ] [S-21] `TestExamples` runs every example end to end against both databases in CI; docs/ contains architecture, capability matrix, migration safety and recovery guide, contributor guide and supported versions; fails if an example is not wired into the test.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

