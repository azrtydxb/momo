# momo-07 Task 8: pgx binary codecs for all four vector types

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 8: pgx binary codecs for all four vector types (`.procoder/plans/momo-07-vectors.md:1339`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-50. Files: `driver/pgxdriver/vector_codec.go`, `driver/pgxdriver/vector_codec_test.go`, `driver/pgxdriver/vector_bench_test.go`, `driver/pgxdriver/pgxdriver.go`.

## Acceptance criteria

- [ ] `TestVectorBinaryCodec` exists as written in the plan, failed before the implementation, and passes now
- [ ] `BenchmarkVectorEncoding` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-50] `TestVectorBinaryCodec` round-trips all four types in binary mode and matches text-mode results; `BenchmarkVectorEncoding` results recorded under bench/results; fails if binary and text decode differently.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

