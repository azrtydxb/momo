# momo-07 Task 1: Vector value types and validation

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 1: Vector value types and validation (`.procoder/plans/momo-07-vectors.md:8`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15. Files: `vector/vector.go`, `vector/codec_test.go`, `vector/fuzz_test.go`.

## Acceptance criteria

- [x] `TestVectorCodec` exists as written in the plan, failed before the implementation, and passes now
- [x] `FuzzVectorDecode` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-15, part] `TestVectorCodec`, `FuzzVectorDecode` pass for the part of S-15 this task delivers; the full criterion is verified in story 162.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test ./vector -count=1` failed to compile with `undefined: Vector` (also HalfVector, SparseVector, Bits, MaxDim) before vector/vector.go existed; this covers TestVectorCodec and FuzzVectorDecode.
- GREEN: `go test -race -count=1 ./vector` ok (TestVectorCodec, TestValidateRules, TestRoundTrips, TestInvalidVectorSentinel); full `go test -race -count=1 ./...` ok.
- Fuzz: FuzzVectorDecode 30s PASS (1.4M execs, no crash); FuzzSparseDecode 15s PASS. Both are in the Makefile FUZZ_TARGETS.
- Mutation: changing the halfvec bound from 65504 to 1e30 made TestVectorCodec and TestValidateRules fail; restored.
- Gate: `procoder check` 0 blocking findings. Commit a9fb790.
- Deviations: ErrInvalidVector message is now `momo/vector: invalid vector` and ErrDialect is added, per the plan; the root package re-exports the same variable so it compiles unchanged. Value() and Scan() validate (finite, dimension 1..MaxDim) instead of always succeeding. MaxBits 83886080 is added for Bits (the plan states no limit). Extra FuzzSparseDecode.

