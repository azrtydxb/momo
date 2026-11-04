# momo-03 Task 16: Error classification for SQLite (mattn) with redaction-safe causes

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-sqlite
Sprint: -

## Description

Implements plan momo-03-drivers Task 16: Error classification for SQLite (mattn) with redaction-safe causes (`.procoder/plans/momo-03-drivers.md:1728`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/sqlitedriver/classify.go`, `driver/sqlitedriver/classify_test.go`.

## Acceptance criteria

- [ ] `TestClassifyUniqueKeepsCause` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestClassifyBusy` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestClassifyPassesThroughUnknown` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

