# momo-07 Task 6: Scoped search settings, exact search, and iterative scans

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: -

## Description

Implements plan momo-07-vectors Task 6: Scoped search settings, exact search, and iterative scans (`.procoder/plans/momo-07-vectors.md:992`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-29. Files: `search.go`, `search_test.go`, `search_integration_test.go`.

## Acceptance criteria

- [ ] `TestSearchSettingsValidation` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSearchSQL` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSearchSetLocalBinds` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSearchSettingsScoped` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestFilteredANNIterativeScan` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-15] `TestVectorCodec` + `FuzzVectorDecode`: vector, halfvec, sparsevec and bit round-trip; wrong dimension returns ErrInvalidVector before any SQL runs; `TestVectorDistances` matches independently computed values to 1e-5; `TestVectorIndexUsed` asserts EXPLAIN shows the HNSW/IVFFlat index; `TestSearchSettingsScoped` asserts `hnsw.ef_search` resets after the transaction; fails if settings leak to the p…
- [ ] [S-29] `TestFilteredANNIterativeScan`: a selective filter with HNSW returns fewer than k rows with iterative scan off and k rows with it on, settings reset after the transaction; fails if the setting leaks to the pool.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

