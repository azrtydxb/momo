# momo-04 Task 33: SQLite inspection

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 33: SQLite inspection (`.procoder/plans/momo-04-migrations.md:3516`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-13, S-39. Files: `migrate/sqlite/inspect.go`.

## Acceptance criteria

- [ ] `TestSQLiteInspectRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-13] `TestSQLiteRebuild`: a rebuild-requiring change preserves row count and content, indexes and triggers, and `PRAGMA foreign_key_check` returns no rows; fails if the rebuild drops a trigger or index.
- [ ] [S-39] `TestManagedFunctionsTriggersPartitions`: a function, trigger and partitioned table are created by migrations, round-trip with no diff, and a changed function body produces CREATE OR REPLACE; fails if they are reported as unmanaged.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

