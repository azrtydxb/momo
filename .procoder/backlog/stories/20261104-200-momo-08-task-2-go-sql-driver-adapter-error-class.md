# momo-08 Task 2: go-sql-driver adapter, error classification and redaction

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 2: go-sql-driver adapter, error classification and redaction (`.procoder/plans/momo-08-mysql.md:154`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8, S-41, S-59. Files: `go.mod`, `driver/mysqldriver/driver.go`, `driver/mysqldriver/errors.go`, `driver/mysqldriver/errors_test.go`, `driver/mysqldriver/driver_integration_test.go`. Contributes to S-59; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMySQLErrorClassification` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestErrorClassification` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEmptyResultNotNotFound` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestErrorRedaction` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-8] `TestErrorClassification` (both dialects): each error kind matches errors.Is and the driver error is reachable with errors.As (*pgconn.PgError, sqlite3.Error); `TestEmptyResultNotNotFound` asserts All on zero rows returns nil error; fails if classification loses the cause.
- [x] [S-41] `TestErrorRedaction`: a unique violation on an email column returns an error whose Error() lacks the email while the detail accessor and errors.As expose it; fails if driver detail is concatenated into the message.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- CI run 42414246083 (workflow ci-agent) on ff4f5bc94a01afe34453ba5ae03667c36436ad0b: success, including integration-mysql on MySQL 8.4.13, MySQL 9.7.4 and MariaDB 11.8.9, and the PostgreSQL integration job.
- The named tests were written together with the implementation and run first on CI, where the earlier runs (for example 42409545190 and 42411082051) failed on them before the fixes landed; there was no separate red run for each test.
- TestMySQLUpsertRendering and TestMySQLGeneratedKeys differ from the plan text: renderMySQLUpsert renders only the tail after the VALUES list, with a different signature.
