# momo-05 Task 25: Window functions, partitioned and ordered (S-33, window part only)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 25: Window functions, partitioned and ordered (S-33, window part only) (`.procoder/plans/momo-05-writes-queries.md:2080`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-33. Files: `window.go`, `window_test.go`.

## Acceptance criteria

- [ ] `TestWindowFunctions` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-33, part] `TestWindowFunctions` passes for the part of S-33 this task delivers; the full criterion is verified in story 149.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

