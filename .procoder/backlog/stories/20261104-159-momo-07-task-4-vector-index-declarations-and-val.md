# momo-07 Task 4: Vector index declarations and validation

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 4: Vector index declarations and validation (`.procoder/plans/momo-07-vectors.md:681`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-15. Files: `schema/vector.go`, `schema/vector_test.go`, `c.Index(...)`. Contributes to S-12, S-15; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestVectorIndexRules` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

RED: `go test ./schema -run TestVectorIndex` failed to build with `undefined: ValidateVectorIndex`, `ErrInvalidVectorIndex`, `VectorIndexOpClass`. GREEN: `go test ./schema -count=1` ok. Mutation: raising the vector dimension limit to 2001 made TestVectorIndexRules and TestVectorIndexRulesMore fail. `procoder check`: 0 blocking findings; go build, go vet and make staticcheck clean. Commit: feat(schema): validate pgvector HNSW and IVFFlat index declarations.
