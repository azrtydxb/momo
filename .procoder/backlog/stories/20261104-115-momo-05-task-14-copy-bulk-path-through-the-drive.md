# momo-05 Task 14: COPY bulk path through the driver (S-5, S-9)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 14: COPY bulk path through the driver (S-5, S-9) (`.procoder/plans/momo-05-writes-queries.md:1079`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `copy_test.go`, `insert_copy.go` (Copy lives there, not in `insert.go`), `insert_copy_unit_test.go`.

## Acceptance criteria

- [ ] `TestCopyInsert` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-5] [S-9] `TestBulkInsertChunking` inserts more rows than one statement's parameter limit and counts them; `TestCopyInsert` (PostgreSQL) writes rows via COPY, including a count above one statement's parameter limit; `TestBulkInsertChunking` (bulk_test.go, which predates this story) fails if chunk size ignores the dialect limit.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

Done (not ticked, see below):

- The COPY path exists: `InsertQuery.Copy` in `insert_copy.go`, with unit tests in `insert_copy_unit_test.go` (column and value order, implicit-transaction setup order, fail-closed on a pinned handle with a setup, hooks never see values, sensitive values, interceptor deny, BeforeInsert, unit-of-work write mark, cancelled context) and `TestCopyInsert` in `copy_test.go` (PostgreSQL: 2 rows, 70000 rows above the 65535 parameter limit, rollback inside a Tx; SQLite: ErrUnsupported).
- Green: ci-agent run recorded in the final report passes lint, unit, fuzz-smoke and PostgreSQL integration with these tests.

Remains open, with the reason:

- Criterion 1 (failed before the implementation): the red run 42455500991 (implementation reverted) failed to compile on `undefined: mergeGate` and `momo.Merge` first; the Go compiler stopped at 10 errors, so the log does not show the missing `Copy` method. No red evidence for `TestCopyInsert` exists, so the box stays unticked. A new throwaway red run with only the Copy implementation reverted would close it.
- Criterion 2: `TestBulkInsertChunking` predates this story, so no red is possible for it, and no mutation run (chunk size ignoring the limit) was made. Unticked.
- Criterion 3: the story close gate must pass once 1 and 2 are settled.
