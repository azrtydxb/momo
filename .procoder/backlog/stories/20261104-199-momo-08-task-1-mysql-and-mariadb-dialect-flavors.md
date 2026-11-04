# momo-08 Task 1: MySQL and MariaDB dialect flavors

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 1: MySQL and MariaDB dialect flavors (`.procoder/plans/momo-08-mysql.md:54`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-31, S-59. Files: `dialect/dialect.go`, `dialect/mysql.go`, `dialect/mysql_test.go`.

## Acceptance criteria

- [ ] `TestMySQLQuoting` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLFlavorCapabilities` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-31] `TestTypeMapping` round-trips one value (including edge values: max int64, NaN where legal, pre-1970 time, non-UTC zone) per mapping in the documented table on both dialects; fails if a documented mapping is untested.
- [ ] [S-59] `TestMySQLIntegration` runs the shared query/write/transaction/error suites against MySQL and MariaDB containers and asserts identical results to PostgreSQL or ErrUnsupported per the capability matrix; fails if a clause is silently dropped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

