# momo-05 Task 20: Automatic timestamps and the injectable clock (S-68)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 20: Automatic timestamps and the injectable clock (S-68) (`.procoder/plans/momo-05-writes-queries.md:1635`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-68. Files: `clock.go`, `timestamps_test.go`, `db.go`.

## Acceptance criteria

- [ ] `TestAutoTimestamps` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-68] `TestAutoTimestamps` asserts insert sets both, update sets only updated, an explicit Set wins, and the injected clock is used; fails if updated_at changes on a no-op guarded update that matched no row.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

