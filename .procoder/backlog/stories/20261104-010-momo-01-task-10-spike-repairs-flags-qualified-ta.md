# momo-01 Task 10: Spike repairs: flags, qualified table references and soft-delete filter

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: -

## Description

Implements plan momo-01-foundation Task 10: Spike repairs: flags, qualified table references and soft-delete filter (`.procoder/plans/momo-01-foundation.md:948`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-75. Files: `meta.go`, `select_repair_test.go`.

## Acceptance criteria

- [ ] `TestSoftDeleteFilterWithColumns` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestTableRefSchemaQualified` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [ ] [S-75] `TestSoftDeleteVariants` asserts each marker kind hides rows by default, OnlyDeleted and ForceDelete behave as documented and a unique index permits re-creating a soft-deleted key; fails if ForceDelete soft-deletes.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

