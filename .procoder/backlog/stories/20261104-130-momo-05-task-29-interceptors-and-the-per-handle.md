# momo-05 Task 29: Interceptors and the per-handle privacy recipe (S-86)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 29: Interceptors and the per-handle privacy recipe (S-86) (`.procoder/plans/momo-05-writes-queries.md:2394`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-86. Files: `interceptor.go`, `interceptor_test.go`, `docs/PRIVACY_RULES.md`.

## Acceptance criteria

- [ ] `TestInterceptors` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-86] `TestInterceptors` asserts an interceptor can add a predicate and deny a mutation, applies only to its handle, and is visible in compiled SQL; fails if an interceptor registered on one handle affects another.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

