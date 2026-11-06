# momo-07 Task 2: Dialect vector capabilities and dialect-specific argument encoding

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 2: Dialect vector capabilities and dialect-specific argument encoding (`.procoder/plans/momo-07-vectors.md:199`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-16. Files: `dialect/vector.go`, `dialect/vector_test.go`, `dialect/dialect.go`, `vector/dialect.go`, `vector/dialect_test.go`, `vector_builder_test.go` …. Contributes to S-15, S-16; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSQLiteVectorCaps` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestDialectValues` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestVectorArgBeforeSQL` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./dialect ./vector .` failed to build (`For(SQLite).Vector undefined`, `Metric` undefined, `v.DialectValue undefined`). After the dialect and vector steps, `TestVectorArgBeforeSQL` failed with `halfvec on sqlite: err = momo/vector: type not supported by dialect: halfvec on sqlite, want ErrUnsupported`.
- Green: `go test ./dialect ./vector . -count=1` is ok for all three. Extra tables: `TestVectorCapsMatrix` (metric x dialect) and `TestDialectValuesMatrix` (type x dialect).
- Mutation: adding MetricJaccard to the SQLite caps made TestSQLiteVectorCaps and TestVectorCapsMatrix fail; reverted.
- builder.go: additive only; `Arg` wraps a `vector.ErrDialect` error as `&Error{Kind: ErrUnsupported}` and imports vector.
- Gate: `go build`, `go vet`, `go test -race -count=1 ./...`, `make staticcheck` clean; `procoder check` 0 blocking. Commit cf05848.

