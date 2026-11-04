# momo-03 Task 12: Iterator closes the cursor on early exit (S-4 cursor guarantee)

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Implements plan momo-03-drivers Task 12: Iterator closes the cursor on early exit (S-4 cursor guarantee) (`.procoder/plans/momo-03-drivers.md:1480`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4. Files: `driver/pgxdriver/iter_test.go`.

## Acceptance criteria

- [ ] `TestIterClosesCursor` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

