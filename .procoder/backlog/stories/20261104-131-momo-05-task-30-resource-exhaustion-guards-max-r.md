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
- [x] [S-48] `TestMaxRowsGuard` and `TestInListCap` return structured errors beyond the configured limits; fails if limits are silently truncated.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- CI run 42410267553 (ci-agent, commit d310d41) green: unit, lint, fuzz-smoke and integration (postgres) jobs pass with this story's tests.
- Criteria left unticked: the red-first claim (no pre-implementation run exists; the first CI run only failed on two fixture mistakes fixed in d310d41) and the local procoder check gate (not run on this laptop).
