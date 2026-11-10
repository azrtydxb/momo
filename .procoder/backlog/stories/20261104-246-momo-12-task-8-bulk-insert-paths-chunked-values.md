# momo-12 Task 8: Bulk insert paths: chunked VALUES, COPY and unnest

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-12-performance Task 8: Bulk insert paths: chunked VALUES, COPY and unnest (`.procoder/plans/momo-12-performance.md:849`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-52. Files: `insert_unnest.go`, `insert_unnest_integration_test.go`, `bench/bulk_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestUnnestBulkInsert` exists as written in the plan, failed before the implementation, and passes now
- [x] `BenchmarkBulkInsert` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-52] `TestUnnestBulkInsert` inserts beyond the bind limit in one statement with identical rows to VALUES and COPY paths; fails if any path loses or reorders data.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestUnnestBulkInsert: insert_unnest_integration_test.go inserts 40000 rows of three columns (120000 binds as VALUES) through chunked VALUES, Unnest (asserted to send exactly one statement) and COPY, and compares the three snapshots. Unit tests: insert_unnest_test.go and insert_unnest_internal_test.go.
  Red proof: ci-agent run 42465164717 (headSha 0c7f14e) on branch worktree-agent-a51f3a530eba5c020-adapters-red, which keeps the tests and removes the implementations: bench-module fails with `no required module provides package` for the removed adapter packages and unit fails with `momo.InsertQuery[suite.Document] has no field or method Unnest`.
  Green proof: ci-agent run 42465734475, headSha eaec549861a73104452a68df580ea15421eac7fb, all eight jobs green (lint, unit, fuzz-smoke, integration postgres, integration mysql84/mysql97/mariadb, bench-module).
- BenchmarkBulkInsert: bench/bulk_test.go, 1000, 10000 and 40000 rows for values, copy and unnest. Its first CI run failed on unnest (`failed to encode args[0]`): a []any is not an array for pgx. Unnest now binds typed slices; the fix is in eaec549.
- Deviation: Unnest binds a typed slice for int, float, bool, string, time and bytea columns (NULLs as pointer slices); a model with other column Go types must use Models.
  Gate: commits went through the procoder commit hook, with a hand-written go.work (untracked) so the hook can lint the nested bench module.
