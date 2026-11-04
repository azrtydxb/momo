# momo-12 Task 8: Bulk insert paths: chunked VALUES, COPY and unnest

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 8: Bulk insert paths: chunked VALUES, COPY and unnest (`.procoder/plans/momo-12-performance.md:849`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-52. Files: `insert_unnest.go`, `insert_unnest_integration_test.go`, `bench/bulk_test.go`.

## Acceptance criteria

- [ ] `TestUnnestBulkInsert` exists as written in the plan, failed before the implementation, and passes now
- [ ] `BenchmarkBulkInsert` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-5] [S-9] `TestBulkInsertChunking` inserts more rows than one statement's parameter limit and counts them; `TestCopyInsert` (PostgreSQL) inserts identical data via COPY; fails if chunk size ignores the dialect limit.
- [ ] [S-52] `TestUnnestBulkInsert` inserts beyond the bind limit in one statement with identical rows to VALUES and COPY paths; fails if any path loses or reorders data.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

