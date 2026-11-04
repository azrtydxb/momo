# momo-05 Task 19: Upsert by conflict target (S-5)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 19: Upsert by conflict target (S-5) (`.procoder/plans/momo-05-writes-queries.md:1586`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `upsert_test.go`, `insert.go`.

## Acceptance criteria

- [ ] `TestUpsertUpdatesAllColumns` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-5] [S-9] `TestBulkInsertChunking` inserts more rows than one statement's parameter limit and counts them; `TestCopyInsert` (PostgreSQL) inserts identical data via COPY; fails if chunk size ignores the dialect limit.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

