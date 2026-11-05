# momo-04 Task 2: Schema normalization and FuzzNormalize

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 2: Schema normalization and FuzzNormalize (`.procoder/plans/momo-04-migrations.md:233`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-11. Files: `schema/normalize.go`. Contributes to S-1, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestNormalizeTypes` exists as written in the plan, failed before the implementation, and passes now (partially: SQLite cases omitted, see Evidence)
- [ ] `TestNormalizeExprDBRewrites` exists as written in the plan, failed before the implementation, and passes now (partially: lower( name ) case differs, see Evidence)
- [x] `TestEqualIgnoresOrder` exists as written in the plan, passes now (Equal already existed; it did not fail for a missing symbol until Normalized was referenced in the same package)
- [x] `FuzzNormalize` exists (extended with Normalized idempotence, no-mutation and Sort-order properties), failed before the implementation (undefined: Normalized), and passes now (30s fuzz run clean)
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Base: sprint-2; NormalizeType, NormalizeExpr and Equal already existed (momo-01); added Normalized() (Equal now compares JSON of it) and FuzzNormalize; added FuzzNormalize to Makefile FUZZ_TARGETS.
- FAIL: `go test ./schema/` -> `undefined: Normalized` (build failed). PASS afterwards; `go test -race -count=1 ./...` green; 30s fuzz of FuzzNormalize clean (~62k execs).
- Mutation: removing Sort() in normalizedCopy fails TestEqualIgnoresOrder, TestNormalizedSortsAndDoesNotAlias and FuzzNormalize seeds; removing the current_timestamp rewrite fails TestNormalizeExprDBRewrites.
- Unchecked: TestNormalizeTypes, TestNormalizeExprDBRewrites and TestEqualIgnoresOrder are present, but TestNormalizeTypes and TestNormalizeExprDBRewrites omit plan cases that contradict momo-01's tests: SQLite upper-case types (momo-01: lower-case) and " (lower( name )) " -> "lower(name)" (momo-01 keeps spacing inside parentheses). Also not implemented: `::text` literal-cast stripping in general (momo-01 keeps 'Mixed CASE'::text; only ::character varying and 'now'::text are rewritten) and Function Body normalization. A decision on which behaviour wins is needed.
