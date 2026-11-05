# momo-07 Task 7: EXPLAIN inspection and index-usage verification

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 7: EXPLAIN inspection and index-usage verification (`.procoder/plans/momo-07-vectors.md:1230`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-18. Files: `explain.go`, `explain_integration_test.go`. Contributes to S-18; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestVectorIndexUsed` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-15] `TestVectorCodec` + `FuzzVectorDecode`: vector, halfvec, sparsevec and bit round-trip; wrong dimension returns ErrInvalidVector before any SQL runs; `TestVectorDistances` matches independently computed values to 1e-5; `TestVectorIndexUsed` asserts EXPLAIN shows the HNSW/IVFFlat index; `TestSearchSettingsScoped` asserts `hnsw.ef_search` resets after the transaction; fails if settings leak to the p…
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: with only the tests written, `go vet -tags integration .` failed with a compiler error: SelectQuery has no field or method Explain, and `go test -run TestExplain .` failed to build for the same reason. Then `explain.go` was added.
- Green: `go test -tags integration -race -count=1 -v -run TestVectorIndexUsed .` passes, with the HNSW check and the `IVFFlat` subtest both PASS. Unit tests `TestExplainStatementPerDialect`, `TestExplainBindsArgsNeverInterpolates` and `TestExplainBuildErrorRunsNothing` (explain_test.go) pass. They assert plain EXPLAIN with no ANALYZE, EXPLAIN QUERY PLAN on SQLite, ErrUnsupported on MySQL with no SQL issued, args identical to `SQL()` with no interpolation, and a build error that issues no statement.
- Full suite: `go vet ./...`, `go vet -tags integration ./...`, `gofmt -l .` and `go test ./...` are clean. `go test -tags integration -race -count=1 ./...` passes everywhere. The first run had two timeouts in `./migrate` (shared Postgres under load from parallel agents); a rerun of `./migrate` passed.
- Explain never runs EXPLAIN ANALYZE; ExplainAnalyze belongs to story 169.
- The S-15 criterion is NOT ticked: it is a composite that depends on other stories' tests (TestVectorCodec, FuzzVectorDecode, TestSearchSettingsScoped, ...). This story only contributes TestVectorIndexUsed.
- S-15: fresh review confirmed every named test exists and passes on the merged tree: `TestVectorCodec` and `FuzzVectorDecode` (vector/), `TestVectorDistances` (vector_integration_test.go), `TestSearchSettingsScoped` (search_integration_test.go, asserts no leak after the transaction), `TestVectorIndexUsed` (explain_integration_test.go, HNSW and IVFFlat).
- Fresh review: Nothing found; arg-binding mutation killed by `TestExplainBindsArgsNeverInterpolates`.
