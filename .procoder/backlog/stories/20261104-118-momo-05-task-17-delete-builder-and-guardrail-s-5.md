# momo-05 Task 17: Delete builder and guardrail — S-5

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 17: Delete builder and guardrail — S-5 (`.procoder/plans/momo-05-writes-queries.md:1428`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `delete.go`, `delete_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestDeleteByModelAndGuard` exists as written in the plan, failed before the implementation, and passes now
- [x] the plan's `TestMutationGuardrails` Delete assertion is restored, and Delete uses the shared effective-filter helper (`hasEffectiveFilter`) so `Where(And())` is refused
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestDeleteByModelAndGuard: before delete.go the build failed with `undefined: momo.Delete`; now PASS on postgres with -race, plus the full integration suite for package momo.
- delete.go uses hasEffectiveFilter; the test asserts Where(And()) is refused. Mutation check: replacing the helper with len(where)==0 made the test fail (`Where(And()) delete err = <nil>`); restored.
- TestMutationGuardrails in update_test.go has its Delete assertion again.
- DELETE renders inside Builder.withScope on the target table.

