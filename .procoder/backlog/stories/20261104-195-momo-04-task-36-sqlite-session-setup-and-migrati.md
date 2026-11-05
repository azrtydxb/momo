# momo-04 Task 36: SQLite session setup and migration lock

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 36: SQLite session setup and migration lock (`.procoder/plans/momo-04-migrations.md:3933`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-13, S-57. Files: `migrate/sqlite/lock.go`. Contributes to S-11, S-13, S-57; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestSQLiteLockTable` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteForeignKeysOffDuringRebuild` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

