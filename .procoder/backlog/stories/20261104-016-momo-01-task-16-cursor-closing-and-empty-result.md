# momo-01 Task 16: Cursor closing and empty-result semantics

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 16: Cursor closing and empty-result semantics (`.procoder/plans/momo-01-foundation.md:1365`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-8. Files: `cursor_test.go`.

## Acceptance criteria

- [ ] `TestIterClosesCursor` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEmptyResultNotNotFound` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [ ] [S-8] `TestErrorClassification` (both dialects): each error kind matches errors.Is and the driver error is reachable with errors.As (*pgconn.PgError, sqlite3.Error); `TestEmptyResultNotNotFound` asserts All on zero rows returns nil error; fails if classification loses the cause.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

