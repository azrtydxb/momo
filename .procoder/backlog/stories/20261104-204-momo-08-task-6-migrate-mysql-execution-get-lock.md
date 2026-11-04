# momo-08 Task 6: migrate/mysql execution, GET_LOCK, statement-level recovery and online DDL

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 6: migrate/mysql execution, GET_LOCK, statement-level recovery and online DDL (`.procoder/plans/momo-08-mysql.md:848`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-60. Files: `migrate/mysql/lock.go`, `migrate/mysql/exec.go`, `migrate/mysql/lock_test.go`, `migrate/mysql/partial_integration_test.go`, `migrate/*.go`.

## Acceptance criteria

- [ ] `TestMySQLPartialFailure` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLLockHolder` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLOnlineDDL` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-10] `TestCLIContracts`: each command's exit codes and outputs as documented in Interfaces; offline commands run with no database URL set; fails if an offline command opens a connection.
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] [S-60] `TestMySQLMigrationRoundTrip` applies the example schema on both servers with a no-change round trip, and `TestMySQLPartialFailure` asserts a mid-file failure records statements_applied; fails if a MySQL migration is reported transactional.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

