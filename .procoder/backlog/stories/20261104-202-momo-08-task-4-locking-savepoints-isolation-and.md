# momo-08 Task 4: Locking, savepoints, isolation and read-only transactions on MySQL

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 4: Locking, savepoints, isolation and read-only transactions on MySQL (`.procoder/plans/momo-08-mysql.md:492`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-7, S-59. Files: `mysql_lock_render.go`, `mysql_lock_render_test.go`, `driver/mysqldriver/tx_integration_test.go`.

## Acceptance criteria

- [ ] `TestMySQLLockRendering` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestTransactions` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLLockSyntax` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [ ] [S-7] `TestTransactions`: callback commits on nil, rolls back on error and on panic (panic re-raised, row absent afterwards), nested savepoint rollback keeps outer work, isolation reaches PostgreSQL (`SHOW transaction_isolation`) and is ErrUnsupported on SQLite for non-default levels; fails if a panic leaves the transaction open.
- [ ] [S-59] `TestMySQLIntegration` runs the shared query/write/transaction/error suites against MySQL and MariaDB containers and asserts identical results to PostgreSQL or ErrUnsupported per the capability matrix; fails if a clause is silently dropped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

