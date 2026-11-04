# momo-04 Task 35: SQLite 12-step rebuild

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 35: SQLite 12-step rebuild (`.procoder/plans/momo-04-migrations.md:3679`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-13. Files: `migrate/sqlite/rebuild.go`.

## Acceptance criteria

- [ ] `TestSQLiteRebuild` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteRebuildFKStillEnforced` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteRebuildOrphanRollsBack` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteRebuildBlocked` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-13] `TestSQLiteRebuild`: a rebuild-requiring change preserves row count and content, indexes and triggers, and `PRAGMA foreign_key_check` returns no rows; fails if the rebuild drops a trigger or index.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

