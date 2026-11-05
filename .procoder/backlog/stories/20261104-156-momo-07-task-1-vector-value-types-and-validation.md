# momo-07 Task 1: Vector value types and validation

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 1: Vector value types and validation (`.procoder/plans/momo-07-vectors.md:8`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15. Files: `vector/vector.go`, `vector/codec_test.go`, `vector/fuzz_test.go`.

## Acceptance criteria

- [ ] `TestVectorCodec` exists as written in the plan, failed before the implementation, and passes now
- [ ] `FuzzVectorDecode` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-15, part] `TestVectorCodec`, `FuzzVectorDecode` pass for the part of S-15 this task delivers; the full criterion is verified in story 162.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

