# momo-05 Task 18: Soft delete variants (S-75) and the soft-delete read filter (S-5)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 18: Soft delete variants (S-75) and the soft-delete read filter (S-5) (`.procoder/plans/momo-05-writes-queries.md:1480`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-75. Files: `softdelete.go`, `softdelete_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

## Acceptance criteria

- [ ] `TestSoftDelete` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSoftDeleteVariants` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-75] `TestSoftDeleteVariants` asserts each marker kind hides rows by default, OnlyDeleted and ForceDelete behave as documented and a unique index permits re-creating a soft-deleted key; fails if ForceDelete soft-deletes.
- [ ] plain `Delete` on a soft-delete model fails with ErrValidation (since Task 17); `ForceDelete` is the only hard delete for such models and bypasses that refusal, with a test for both
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

