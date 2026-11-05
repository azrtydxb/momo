# momo-05 Task 15: Update builder, mutation guardrails and optimistic locking — S-5

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 15: Update builder, mutation guardrails and optimistic locking — S-5 (`.procoder/plans/momo-05-writes-queries.md:1138`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `update.go`, `update_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

## Acceptance criteria

- [ ] `TestMutationGuardrails` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestOptimisticLock` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

