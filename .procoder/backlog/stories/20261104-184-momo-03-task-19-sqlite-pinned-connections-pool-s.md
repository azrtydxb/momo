# momo-03 Task 19: SQLite pinned connections, pool statistics and busy mapping

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-sqlite
Sprint: -

## Description

Implements plan momo-03-drivers Task 19: SQLite pinned connections, pool statistics and busy mapping (`.procoder/plans/momo-03-drivers.md:1965`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/sqlitedriver/pin_test.go`, `driver/sqlitedriver/driver.go`.

## Acceptance criteria

- [ ] `TestBusyIsStructuredError` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLitePinnedAndStats` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

