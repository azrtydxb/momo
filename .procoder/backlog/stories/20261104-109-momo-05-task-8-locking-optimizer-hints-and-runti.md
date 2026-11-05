# momo-05 Task 8: Locking, optimizer hints and runtime table override (S-79)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 8: Locking, optimizer hints and runtime table override (S-79) (`.procoder/plans/momo-05-writes-queries.md:583`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-79. Files: `integration_locking_test.go`, `select.go`. Contributes to S-79; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestLockingNowaitSkipLocked` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestWithTableNameQuoted` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestHintsByDialect` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

