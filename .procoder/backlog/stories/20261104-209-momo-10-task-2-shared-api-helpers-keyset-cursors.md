# momo-10 Task 2: Shared API helpers: keyset cursors, page limits, allowlisted filters

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 2: Shared API helpers: keyset cursors, page limits, allowlisted filters (`.procoder/plans/momo-10-ecosystem-generators.md:85`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/apiquery/cursor.go`, `gen/apiquery/filter.go`, `gen/apiquery/apiquery_test.go`, `momo/column_ref.go`.

## Acceptance criteria

- [x] `TestCursorRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestPageLimit` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestFilterAllowlist` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/apiquery/cursor.go, filter.go, apiquery_test.go; column_parse.go and column_parse_test.go in the root package (the plan names momo/column_ref.go, but ColumnRef lives in expr.go, so the methods are in column_parse.go); expr.go adds ParseEq and ParseCmp to the ColumnRef interface, implemented on Column, the read-only, create-only and immutable columns, and the vector columns (which refuse a text filter).
- TestCursorRoundTrip, TestPageLimit, TestFilterAllowlist red: CI run 42414302688 (tests alone on sprint-5) failed with "undefined: EncodeCursor, DecodeCursor, ClampLimit, ParseFilter". Green: CI run 42412136570 (commit 015deab) succeeded.
- Extra tests TestParseCmpBindsTypedValue and TestParseCmpRejects: the error never echoes the raw value.

