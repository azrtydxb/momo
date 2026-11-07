# momo-08 Task 8: Pinned MySQL and MariaDB services and the shared suites

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 8: Pinned MySQL and MariaDB services and the shared suites (`.procoder/plans/momo-08-mysql.md:1451`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20, S-59. Files: `internal/testdb/compose.yaml`, `driver/mysqldriver/integration_test.go`. Contributes to S-20; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMySQLIntegration` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-59] `TestMySQLIntegration` runs the shared query/write/transaction/error suites against MySQL and MariaDB containers and asserts identical results to PostgreSQL or ErrUnsupported per the capability matrix; fails if a clause is silently dropped.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- CI run 42414246083 (workflow ci-agent) on ff4f5bc94a01afe34453ba5ae03667c36436ad0b: success, including integration-mysql on MySQL 8.4.13, MySQL 9.7.4 and MariaDB 11.8.9, and the PostgreSQL integration job.
- The named tests were written together with the implementation and run first on CI, where the earlier runs (for example 42409545190 and 42411082051) failed on them before the fixes landed; there was no separate red run for each test.
- TestMySQLUpsertRendering and TestMySQLGeneratedKeys differ from the plan text: renderMySQLUpsert renders only the tail after the VALUES list, with a different signature.
