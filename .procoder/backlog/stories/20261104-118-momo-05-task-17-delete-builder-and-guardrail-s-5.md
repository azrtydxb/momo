# momo-05 Task 17: Delete builder and guardrail — S-5

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 17: Delete builder and guardrail — S-5 (`.procoder/plans/momo-05-writes-queries.md:1428`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `delete.go`, `delete_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestDeleteByModelAndGuard` exists as written in the plan, failed before the implementation, and passes now
- [ ] the plan's `TestMutationGuardrails` Delete assertion is restored, and Delete uses the shared effective-filter helper (`hasEffectiveFilter`) so `Where(And())` is refused
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

