# momo-08 Task 4: Locking, savepoints, isolation and read-only transactions on MySQL

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 4: Locking, savepoints, isolation and read-only transactions on MySQL (`.procoder/plans/momo-08-mysql.md:492`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-7, S-59. Files: `mysql_lock_render.go`, `mysql_lock_render_test.go`, `driver/mysqldriver/tx_integration_test.go`. Contributes to S-4, S-59; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMySQLLockRendering` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestTransactions` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestMySQLLockSyntax` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-7] `TestTransactions`: callback commits on nil, rolls back on error and on panic (panic re-raised, row absent afterwards), nested savepoint rollback keeps outer work, isolation reaches PostgreSQL (`SHOW transaction_isolation`) and is ErrUnsupported on SQLite for non-default levels; fails if a panic leaves the transaction open.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- CI run 42414246083 (workflow ci-agent) on ff4f5bc94a01afe34453ba5ae03667c36436ad0b: success, including integration-mysql on MySQL 8.4.13, MySQL 9.7.4 and MariaDB 11.8.9, and the PostgreSQL integration job.
- The named tests were written together with the implementation and run first on CI, where the earlier runs (for example 42409545190 and 42411082051) failed on them before the fixes landed; there was no separate red run for each test.
- TestMySQLUpsertRendering and TestMySQLGeneratedKeys differ from the plan text: renderMySQLUpsert renders only the tail after the VALUES list, with a different signature.
- Red proof: CI run 42455266153 on branch worktree-agent-a914dd1a1533958bf-red (commit 2246ba3, origin/sprint-5 plus only the MySQL tests and CI wiring, implementation absent) failed in every job (unit, lint, fuzz-smoke, integration, integration-mysql on MySQL 8.4, MySQL 9.7 and MariaDB 11.8): the tests do not compile without the implementation.
- Green proof: CI run 42454363946 on worktree-agent-a914dd1a1533958bf at f40bdaf5b43941ef43d0c652e5a598f5199d7416 passed all seven jobs, after the review fixes.
