# momo-11 Task 13: Go-function migrations

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 13: Go-function migrations (`.procoder/plans/momo-11-adoption.md:1458`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-94. Files: `migrate/gomigration.go`, `migrate/runner.go`.

## Acceptance criteria

- [ ] `TestGoMigrations` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-94] `TestGoMigrations` asserts a registered Go migration runs once under the lock with history recorded, the split directive splits statements, and group rollback runs authored downs in reverse; fails if a Go migration bypasses history.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

