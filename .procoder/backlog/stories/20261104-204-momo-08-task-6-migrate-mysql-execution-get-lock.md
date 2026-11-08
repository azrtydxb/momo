# momo-08 Task 6: migrate/mysql execution, GET_LOCK, statement-level recovery and online DDL

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 6: migrate/mysql execution, GET_LOCK, statement-level recovery and online DDL (`.procoder/plans/momo-08-mysql.md:848`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-60. Files: `migrate/mysql/lock.go`, `migrate/mysql/exec.go`, `migrate/mysql/lock_test.go`, `migrate/mysql/partial_integration_test.go`, `migrate/*.go`. Contributes to S-10, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMySQLPartialFailure` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestMySQLLockHolder` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestMySQLOnlineDDL` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-60] `TestMySQLMigrationRoundTrip` applies the example schema on both servers with a no-change round trip, and `TestMySQLPartialFailure` asserts a mid-file failure records statements_applied; fails if a MySQL migration is reported transactional.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Green run: ci-agent run 42464754775 on 4fbb8ddff7eea92368afd359ec94377897e03c89 (earlier green run: 42462329606), all jobs success. TestMySQLPartialFailure, TestMySQLLockHolder and TestMySQLOnlineDDL run on the mysql84 job (they open MySQL 8.4); the roundtrip runs on all three servers.
- Red run: ci-agent run 42460447363 (throwaway branch with the tests only, deleted): the packages did not build without the implementation (undefined: lockName, Dialect, applyDDLDirective), and TestSQLStateReachesTheMigrationEngine failed.
- Deviations from the plan text, with reasons. Lock: the plan's lockName only truncates; lockName keeps 55 characters plus a hash suffix when the key exceeds 64 so two long keys cannot collide (migrate/mysql/lock_test.go); the test file is lock_integration_test.go as the plan's own step 2 names it. TestMySQLOnlineDDL: run 42460424652 showed MySQL 8.4 rejects ALGORITHM=INSTANT together with LOCK=NONE (error 1221), so the test applies INSTANT alone and INPLACE with LOCK=NONE, then a rejected INSTANT change; the driver withholds server text from Error, so the test reads the message from the *mysql.MySQLError cause. No exec.go: the statement-level recovery already existed in migrate.RunMigration (runNonTransactional), so the engine change is that it takes that path whenever Dialect().TransactionalDDL is false (migrate/runner.go).
- Statement-level recovery: TestMySQLPartialFailure asserts table a survives, status dirty, statements_applied 2, a second apply is ErrDirty and Resolve works. A transactional run would end rolled_back, so the test fails if a MySQL migration were reported transactional.
- GET_LOCK: migrate/mysql/lock.go on the pinned connection with GET_LOCK(name, 0), IS_USED_LOCK for the holder, RELEASE_LOCK in unlock. The lock key carries the database via the HistoryLocator that migrate/mysql/session.go implements; the contender error names the holding connection (asserted).
- Online DDL: the -- momo:ddl directive is implemented in migrate/ddl_directive.go (statement scoped, ALGORITHM and LOCK options only, ALTER TABLE only, MySQL only) with unit tests in migrate/ddl_directive_internal_test.go. ParseDirectives accepts it.
- S-60 criterion: the roundtrip applies the example schema on mysql84, mysql97 and mariadb (one per CI job) with a clean no-change round trip.
- Other engine changes: ensureHistory writes the meta row with ON DUPLICATE KEY on MySQL, a missing history table is recognised by SQLSTATE 42S02 as well as 42P01, and mysqldriver's redacted error now exposes SQLState (driver/mysqldriver/sqlstate_test.go).
- Not done: momo migrate (cmd/momo) still builds only the PostgreSQL dialect, so the CLI does not select migrate/mysql yet; the story does not list cmd/momo.
- Gate: the procoder commit gate ran on every commit of this branch and passed.
- Review round: rollback now runs a MySQL down statement by statement and records a failed row (TestMySQLFailingDown); the ddl clause goes on its own line so a trailing comment cannot swallow it; lock names are cut at a character boundary. Green in run 42464754775 on 4fbb8ddff7eea92368afd359ec94377897e03c89 (the final code commit).
