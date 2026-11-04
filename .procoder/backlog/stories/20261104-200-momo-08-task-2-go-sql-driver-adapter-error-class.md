# momo-08 Task 2: go-sql-driver adapter, error classification and redaction

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 2: go-sql-driver adapter, error classification and redaction (`.procoder/plans/momo-08-mysql.md:154`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8, S-41, S-59. Files: `go.mod`, `driver/mysqldriver/driver.go`, `driver/mysqldriver/errors.go`, `driver/mysqldriver/errors_test.go`, `driver/mysqldriver/driver_integration_test.go`.

## Acceptance criteria

- [ ] `TestMySQLErrorClassification` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestErrorClassification` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEmptyResultNotNotFound` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestErrorRedaction` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-8] `TestErrorClassification` (both dialects): each error kind matches errors.Is and the driver error is reachable with errors.As (*pgconn.PgError, sqlite3.Error); `TestEmptyResultNotNotFound` asserts All on zero rows returns nil error; fails if classification loses the cause.
- [ ] [S-41] `TestErrorRedaction`: a unique violation on an email column returns an error whose Error() lacks the email while the detail accessor and errors.As expose it; fails if driver detail is concatenated into the message.
- [ ] [S-59] `TestMySQLIntegration` runs the shared query/write/transaction/error suites against MySQL and MariaDB containers and asserts identical results to PostgreSQL or ErrUnsupported per the capability matrix; fails if a clause is silently dropped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

