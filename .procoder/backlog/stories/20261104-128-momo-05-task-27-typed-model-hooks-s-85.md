# momo-05 Task 27: Typed model hooks (S-85)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 27: Typed model hooks (S-85) (`.procoder/plans/momo-05-writes-queries.md:2238`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-85. Files: `hooks.go`, `hooks_test.go`, `meta.go`.

## Acceptance criteria

- [ ] `TestModelHooks` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-85] `TestModelHooks` asserts invocation order, error abort before SQL, and zero reflection in the call path (benchmark allocation check); fails if a hook runs for a builder that did not opt in.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

