# momo-01 Task 8: Error sentinels, structured Error and sibling sentinels

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 8: Error sentinels, structured Error and sibling sentinels (`.procoder/plans/momo-01-foundation.md:758`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8. Files: `errors.go`, `errors_test.go`, `vector/errors.go`, `migrate/errors.go`.

## Acceptance criteria

- [ ] `TestErrorSentinels` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestErrorIsAs` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestInvalidVectorSentinel` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMigrateSentinelMessages` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-8] `TestErrorClassification` (both dialects): each error kind matches errors.Is and the driver error is reachable with errors.As (*pgconn.PgError, sqlite3.Error); `TestEmptyResultNotNotFound` asserts All on zero rows returns nil error; fails if classification loses the cause.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

