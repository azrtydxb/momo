# momo-05 Task 18: Soft delete variants (S-75) and the soft-delete read filter (S-5)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 18: Soft delete variants (S-75) and the soft-delete read filter (S-5) (`.procoder/plans/momo-05-writes-queries.md:1480`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-75. Files: `softdelete.go`, `softdelete_test.go`.

## Acceptance criteria

- [ ] `TestSoftDelete` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSoftDeleteVariants` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-5] [S-9] `TestBulkInsertChunking` inserts more rows than one statement's parameter limit and counts them; `TestCopyInsert` (PostgreSQL) inserts identical data via COPY; fails if chunk size ignores the dialect limit.
- [ ] [S-75] `TestSoftDeleteVariants` asserts each marker kind hides rows by default, OnlyDeleted and ForceDelete behave as documented and a unique index permits re-creating a soft-deleted key; fails if ForceDelete soft-deletes.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

