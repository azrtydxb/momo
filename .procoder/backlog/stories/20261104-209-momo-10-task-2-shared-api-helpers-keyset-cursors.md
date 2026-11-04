# momo-10 Task 2: Shared API helpers: keyset cursors, page limits, allowlisted filters

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 2: Shared API helpers: keyset cursors, page limits, allowlisted filters (`.procoder/plans/momo-10-ecosystem-generators.md:85`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/apiquery/cursor.go`, `gen/apiquery/filter.go`, `gen/apiquery/apiquery_test.go`, `momo/column_ref.go`.

## Acceptance criteria

- [ ] `TestCursorRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPageLimit` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestFilterAllowlist` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

