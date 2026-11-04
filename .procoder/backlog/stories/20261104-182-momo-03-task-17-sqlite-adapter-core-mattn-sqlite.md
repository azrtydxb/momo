# momo-03 Task 17: SQLite adapter core (mattn + sqlite-vec) with recommended configuration

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-sqlite
Sprint: -

## Description

Implements plan momo-03-drivers Task 17: SQLite adapter core (mattn + sqlite-vec) with recommended configuration (`.procoder/plans/momo-03-drivers.md:1795`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-57. Files: `driver/sqlitedriver/open.go`, `driver/sqlitedriver/open_test.go`, `go.mod`.

## Acceptance criteria

- [ ] `TestCheckVersion` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteRecommendedConfig` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-57] `TestSQLiteRecommendedConfig` asserts WAL, busy_timeout and synchronous settings on adapter connections and that concurrent writers yield the structured busy error or succeed within busy_timeout; fails if SQLITE_BUSY surfaces unclassified.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

