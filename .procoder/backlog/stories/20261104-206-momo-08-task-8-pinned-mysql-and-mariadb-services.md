# momo-08 Task 8: Pinned MySQL and MariaDB services and the shared suites

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 8: Pinned MySQL and MariaDB services and the shared suites (`.procoder/plans/momo-08-mysql.md:1451`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20, S-59. Files: `internal/testdb/compose.yaml`, `driver/mysqldriver/integration_test.go`.

## Acceptance criteria

- [ ] `TestMySQLIntegration` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-20] `TestCIWorkflowCoverage` parses .github/workflows/ci.yml and asserts it runs unit, generator, compile-fail, race and fuzz-smoke tests, plus integration tests against pgvector/pgvector:pg17 and SQLite+sqlite-vec, on Go 1.26 and 1.27; fails if any job is skipped.
- [ ] [S-59] `TestMySQLIntegration` runs the shared query/write/transaction/error suites against MySQL and MariaDB containers and asserts identical results to PostgreSQL or ErrUnsupported per the capability matrix; fails if a clause is silently dropped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

