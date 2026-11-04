# momo-02 Task 11: Go-to-SQL type map for the generator

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 11: Go-to-SQL type map for the generator (`.procoder/plans/momo-02-generator.md:1008`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-3, S-31. Files: `gen/typemap/typemap.go`, `gen/typemap/typemap_test.go`, `gen/schema.go`.

## Acceptance criteria

- [ ] `TestGeneratorTypeMap` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-1] `TestSchemaJSONRoundTrip` and `FuzzSchemaSort` (schema package): JSON round-trips losslessly and `Sort` makes equivalent schemas byte-identical; fails if a field is dropped from serialization or Sort is order-sensitive.
- [ ] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [ ] [S-31] `TestTypeMapping` round-trips one value (including edge values: max int64, NaN where legal, pre-1970 time, non-UTC zone) per mapping in the documented table on both dialects; fails if a documented mapping is untested.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

