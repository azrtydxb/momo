# momo-05 Task 30: Resource-exhaustion guards: max rows and IN-list cap (S-48)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 30: Resource-exhaustion guards: max rows and IN-list cap (S-48) (`.procoder/plans/momo-05-writes-queries.md:2483`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-48. Files: `limits.go`, `limits_test.go`.

## Acceptance criteria

- [ ] `TestMaxRowsGuard` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestInListCap` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-48] `TestMaxRowsGuard` and `TestInListCap` return structured errors beyond the configured limits; fails if limits are silently truncated.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

