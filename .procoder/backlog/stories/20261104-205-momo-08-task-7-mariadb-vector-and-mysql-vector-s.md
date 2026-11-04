# momo-08 Task 7: MariaDB VECTOR and MySQL VECTOR storage, distance and index capability

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 7: MariaDB VECTOR and MySQL VECTOR storage, distance and index capability (`.procoder/plans/momo-08-mysql.md:1148`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-61. Files: `vector_mysql.go`, `vector_mysql_test.go`, `vector/dialect.go`, `schema/vector.go`, `migrate/mysql/vector.go`, `driver/mysqldriver/vector_integration_test.go`. Contributes to S-15; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestMySQLVectorSQL` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMariaDBVectorParity` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLVectorCapability` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-61] `TestMariaDBVectorParity` returns the same top-k ids as pgvector exact search; `TestMySQLVectorCapability` asserts distance queries are ErrUnsupported where the server lacks distance functions; fails if a full scan is substituted silently.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

