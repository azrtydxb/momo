# momo-08 Task 9: MySQL documentation and capability matrix

Status: done
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-08-mysql Task 9: MySQL documentation and capability matrix (`.procoder/plans/momo-08-mysql.md:1512`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21, S-59, S-60. Files: `docs/mysql.md`, `docs/README.md`, `dialect/matrix_test.go`. Contributes to S-21, S-59, S-60; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestCapabilityMatrixMatchesDialect` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Already built in an earlier sprint: dialect/matrix_test.go TestCapabilityMatrixMatchesDialect (rows Returning, InsertReturning, OnConflict, OnDuplicateKey, NullsOrdering, TransactionalDDL, Merge, ILike) against docs/mysql.md; docs/README.md indexes the page. ci-agent run 42780640447 on 8cf94c3ee066c5a183bd60c56ec1b3afeb505e37, all 9 jobs success (dialect package ok in unit).
- Red-first for criterion 1 and the extended matrix (now also Locking, RowValues, Savepoints, Vector.Storage, Vector.Metrics, Vector.Indexes, with rows in docs/mysql.md): ci-agent run 42785752323 on throwaway branch worktree-agent-a9cf43902d9e90e16-red (deleted), headSha 776ffc7, docs/mysql.md InsertReturning mariadb set to no: all jobs failed with 'matrix_test.go:71: docs/mysql.md InsertReturning mariadb = "no", dialect says yes'.
