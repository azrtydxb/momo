# momo-07 Task 3: VectorColumn and typed distance expressions

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 3: VectorColumn and typed distance expressions (`.procoder/plans/momo-07-vectors.md:370`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-16. Files: `vector_column.go`, `vector_expr.go`, `vector_expr_test.go`, `vector_integration_test.go`, `testdata/compilefail/vector_metric_mismatch/main.go`. Contributes to S-16; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestVectorDistanceSQL` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestVectorDistanceDimensionMismatch` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLiteVectorDistanceSQL` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestVectorDistances` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-15, part] `TestVectorDistances` passes for the part of S-15 this task delivers; the full criterion is verified in story 162.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: before vector_column.go and vector_expr.go existed, `go test . -run 'TestVectorDistanceSQL|TestVectorDistanceDimensionMismatch|TestSQLiteVectorDistanceSQL'` failed to build with `undefined: NewVectorColumn`, `undefined: L2Distance` and the other constructors. Green: all three `--- PASS`, plus the added TestVectorDistanceMatrix (metric x type x dialect), TestVectorDistanceInQueries (SELECT, WHERE, ORDER BY golden) and TestVectorColumnBasics.
- TestVectorDistances (integration, against the running pgvector container): it uses the same undefined symbols, so it could not build before the implementation; `go test -tags integration -race . -run TestVectorDistances` now prints `--- PASS`, with correct nearest-neighbour order [2 1 3] and all four dense distances within 1e-5 of Go reference values.
- Mutation: changing the cosine operator to `<->` made TestVectorDistanceSQL and TestVectorDistanceMatrix fail (`cosine SQL = ("docs"."emb" <-> $1)`); restored, green.
- Compile-fail: `go build ./testdata/compilefail/vector_metric_mismatch` exits non-zero with `vector.Bits does not satisfy momo.denseVector`; TestCompileFailVectorMetricMismatch asserts `does not satisfy`.
- Deviation: the constructors return the concrete `DistanceExpr` (it implements Expression[float64] and Orderable and has Asc/Desc) rather than the bare interface, because the plan's own integration test calls `.Asc()` on the result.
