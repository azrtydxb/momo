# momo-08 Task 6: migrate/mysql execution, GET_LOCK, statement-level recovery and online DDL

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 6: migrate/mysql execution, GET_LOCK, statement-level recovery and online DDL (`.procoder/plans/momo-08-mysql.md:848`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-60. Files: `migrate/mysql/lock.go`, `migrate/mysql/exec.go`, `migrate/mysql/lock_test.go`, `migrate/mysql/partial_integration_test.go`, `migrate/*.go`. Contributes to S-10, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestMySQLPartialFailure` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLLockHolder` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLOnlineDDL` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-60] `TestMySQLMigrationRoundTrip` applies the example schema on both servers with a no-change round trip, and `TestMySQLPartialFailure` asserts a mid-file failure records statements_applied; fails if a MySQL migration is reported transactional.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

