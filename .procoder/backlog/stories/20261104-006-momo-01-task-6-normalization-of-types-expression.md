# momo-01 Task 6: Normalization of types, expressions and schema equality

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: -

## Description

Implements plan momo-01-foundation Task 6: Normalization of types, expressions and schema equality (`.procoder/plans/momo-01-foundation.md:565`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1. Files: `schema/normalize.go`, `schema/normalize_test.go`.

## Acceptance criteria

- [ ] `TestNormalizeType` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestNormalizeExpr` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEqualIgnoresOrderAndConcurrently` exists as written in the plan, failed before the implementation, and passes now
- [ ] `FuzzNormalizeType` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-1] `TestSchemaJSONRoundTrip` and `FuzzSchemaSort` (schema package): JSON round-trips losslessly and `Sort` makes equivalent schemas byte-identical; fails if a field is dropped from serialization or Sort is order-sensitive.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

