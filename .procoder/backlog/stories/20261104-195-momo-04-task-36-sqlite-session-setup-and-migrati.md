# momo-04 Task 36: SQLite session setup and migration lock

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 36: SQLite session setup and migration lock (`.procoder/plans/momo-04-migrations.md:3933`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-13, S-57. Files: `migrate/sqlite/lock.go`.

## Acceptance criteria

- [ ] `TestSQLiteLockTable` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteForeignKeysOffDuringRebuild` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] [S-13] `TestSQLiteRebuild`: a rebuild-requiring change preserves row count and content, indexes and triggers, and `PRAGMA foreign_key_check` returns no rows; fails if the rebuild drops a trigger or index.
- [ ] [S-57] `TestSQLiteRecommendedConfig` asserts WAL, busy_timeout and synchronous settings on adapter connections and that concurrent writers yield the structured busy error or succeed within busy_timeout; fails if SQLITE_BUSY surfaces unclassified.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

