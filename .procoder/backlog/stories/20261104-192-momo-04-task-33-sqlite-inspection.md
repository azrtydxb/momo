# momo-04 Task 33: SQLite inspection

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 33: SQLite inspection (`.procoder/plans/momo-04-migrations.md:3516`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-13, S-39. Files: `migrate/sqlite/inspect.go`. Contributes to S-13, S-39; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestSQLiteInspectRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

