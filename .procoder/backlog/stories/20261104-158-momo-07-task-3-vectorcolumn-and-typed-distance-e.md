# momo-07 Task 3: VectorColumn and typed distance expressions

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 3: VectorColumn and typed distance expressions (`.procoder/plans/momo-07-vectors.md:370`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-16. Files: `vector_column.go`, `vector_expr.go`, `vector_expr_test.go`, `vector_integration_test.go`, `testdata/compilefail/vector_metric_mismatch/main.go`. Contributes to S-16; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestVectorDistanceSQL` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestVectorDistanceDimensionMismatch` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteVectorDistanceSQL` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestVectorDistances` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-15, part] `TestVectorDistances` passes for the part of S-15 this task delivers; the full criterion is verified in story 162.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

