# momo-04 Task 34: SQLite rendering: native ALTER versus rebuild decision

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 34: SQLite rendering: native ALTER versus rebuild decision (`.procoder/plans/momo-04-migrations.md:3606`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-13. Files: `migrate/sqlite/render.go`. Contributes to S-13; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestSQLiteAlterDecision` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteUnsupportedKindsError` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

