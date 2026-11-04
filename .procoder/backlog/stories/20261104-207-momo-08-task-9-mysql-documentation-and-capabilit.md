# momo-08 Task 9: MySQL documentation and capability matrix

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 9: MySQL documentation and capability matrix (`.procoder/plans/momo-08-mysql.md:1512`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21, S-59, S-60. Files: `docs/mysql.md`, `docs/README.md`, `dialect/matrix_test.go`.

## Acceptance criteria

- [ ] `TestCapabilityMatrixMatchesDialect` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-21] `TestExamples` runs every example end to end against both databases in CI; docs/ contains architecture, capability matrix, migration safety and recovery guide, contributor guide and supported versions; fails if an example is not wired into the test.
- [ ] [S-59] `TestMySQLIntegration` runs the shared query/write/transaction/error suites against MySQL and MariaDB containers and asserts identical results to PostgreSQL or ErrUnsupported per the capability matrix; fails if a clause is silently dropped.
- [ ] [S-60] `TestMySQLMigrationRoundTrip` applies the example schema on both servers with a no-change round trip, and `TestMySQLPartialFailure` asserts a mid-file failure records statements_applied; fails if a MySQL migration is reported transactional.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

