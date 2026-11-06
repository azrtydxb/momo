# momo-05 Task 15: Update builder, mutation guardrails and optimistic locking — S-5

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 15: Update builder, mutation guardrails and optimistic locking — S-5 (`.procoder/plans/momo-05-writes-queries.md:1138`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `update.go`, `update_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

## Acceptance criteria

- [x] `TestMutationGuardrails` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestOptimisticLock` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- FAIL before implementation: `go test -tags integration -run 'TestMutationGuardrails|TestOptimisticLock|TestUpdate' .` -> `undefined: momo.Update`.
- PASS after: `go test -tags integration -race -count=1 .` -> ok (TestMutationGuardrails, TestOptimisticLock, plus unit tests for scope, read-only/identity refusal, SetDefault on SQLite, bound values).
- Mutation check: removing the `len(where) == 0 && !allRows` guard made TestUpdateBuilderRendering fail ("no WHERE err = <nil>").
- Statement renders in `Builder.withScope` of the target table; correlated EXISTS subquery tested.
- Gate: `procoder check` 0 blocking; `make staticcheck` clean. Commits adb97cd, 9844cc0.
- The unfiltered DELETE assertion of the plan test is deferred to the delete builder story (no momo.Delete yet).
