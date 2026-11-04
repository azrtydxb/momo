# momo-07 Task 3: VectorColumn and typed distance expressions

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: -

## Description

Implements plan momo-07-vectors Task 3: VectorColumn and typed distance expressions (`.procoder/plans/momo-07-vectors.md:370`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-16. Files: `vector_column.go`, `vector_expr.go`, `vector_expr_test.go`, `vector_integration_test.go`, `testdata/compilefail/vector_metric_mismatch/main.go`.

## Acceptance criteria

- [ ] `TestVectorDistanceSQL` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestVectorDistanceDimensionMismatch` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteVectorDistanceSQL` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestVectorDistances` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-15] `TestVectorCodec` + `FuzzVectorDecode`: vector, halfvec, sparsevec and bit round-trip; wrong dimension returns ErrInvalidVector before any SQL runs; `TestVectorDistances` matches independently computed values to 1e-5; `TestVectorIndexUsed` asserts EXPLAIN shows the HNSW/IVFFlat index; `TestSearchSettingsScoped` asserts `hnsw.ef_search` resets after the transaction; fails if settings leak to the p…
- [ ] [S-16] `TestSQLiteVectorParity`: sqlite-vec distance queries return the same top-k ids as pgvector exact search on the same data; requesting an ANN index on SQLite returns ErrUnsupported; fails if SQLite silently accepts an index option.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

