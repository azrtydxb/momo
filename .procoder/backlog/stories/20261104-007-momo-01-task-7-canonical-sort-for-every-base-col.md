# momo-01 Task 7: Canonical Sort for every base collection

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 7: Canonical Sort for every base collection (`.procoder/plans/momo-01-foundation.md:671`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1. Files: `schema/schema.go`, `schema/sort_test.go`.

## Acceptance criteria

- [ ] `TestSortCoversAllCollections` exists as written in the plan, fails under a deliberate mutation of `Sort` (the spike's `Sort` predates the test), and passes on the restored code
- [ ] `FuzzSchemaSort` exists as written in the plan, fails under a deliberate mutation of `Sort` (the spike's `Sort` predates the test), and passes on the restored code
- [ ] [S-1] `TestSchemaJSONRoundTrip` and `FuzzSchemaSort` (schema package): JSON round-trips losslessly and `Sort` makes equivalent schemas byte-identical; fails if a field is dropped from serialization or Sort is order-sensitive.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

