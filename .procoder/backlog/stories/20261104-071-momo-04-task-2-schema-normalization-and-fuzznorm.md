# momo-04 Task 2: Schema normalization and FuzzNormalize

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 2: Schema normalization and FuzzNormalize (`.procoder/plans/momo-04-migrations.md:233`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-11. Files: `schema/normalize.go`. Contributes to S-1, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestNormalizeTypes` exists as written in the plan, failed before the implementation, and passes now (as written, after the coordinator decided momo-04 Task 2 wins over momo-01)
- [x] `TestNormalizeExprDBRewrites` exists as written in the plan, failed before the implementation, and passes now (as written, after the coordinator decided momo-04 Task 2 wins over momo-01)
- [x] `TestEqualIgnoresOrder` exists as written in the plan, passes now (Equal already existed; it did not fail for a missing symbol until Normalized was referenced in the same package)
- [x] `FuzzNormalize` exists (extended with Normalized idempotence, no-mutation and Sort-order properties), failed before the implementation (undefined: Normalized), and passes now (30s fuzz run clean)
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Base: sprint-2; NormalizeType, NormalizeExpr and Equal already existed (momo-01); added Normalized() (Equal now compares JSON of it) and FuzzNormalize; added FuzzNormalize to Makefile FUZZ_TARGETS.
- FAIL: `go test ./schema/` -> `undefined: Normalized` (build failed). PASS afterwards; `go test -race -count=1 ./...` green; 30s fuzz of FuzzNormalize clean (~62k execs).
- Mutation: removing Sort() in normalizedCopy fails TestEqualIgnoresOrder, TestNormalizedSortsAndDoesNotAlias and FuzzNormalize seeds; removing the current_timestamp rewrite fails TestNormalizeExprDBRewrites.
- Decision: momo-04 Task 2 owns the canonical forms. Applied the previously omitted rules (SQLite upper-case types, no spacing inside parentheses, literal cast stripping, Function.Body normalization); momo-01 tests and plan Task 6 updated; gen/typemap now emits canonical types (character varying(n), timestamp with time zone) and a test asserts NormalizeType(d, got) == got. Plan tests ran as written and failed before the change (SQLite BIGINT, lower(name)). 30s fuzz clean again.
