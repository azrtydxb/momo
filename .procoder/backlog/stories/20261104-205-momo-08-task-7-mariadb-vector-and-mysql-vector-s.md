# momo-08 Task 7: MariaDB VECTOR and MySQL VECTOR storage, distance and index capability

Status: done
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-08-mysql Task 7: MariaDB VECTOR and MySQL VECTOR storage, distance and index capability (`.procoder/plans/momo-08-mysql.md:1148`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-61. Files: `vector_mysql.go`, `vector_mysql_test.go`, `vector/dialect.go`, `schema/vector.go`, `migrate/mysql/vector.go`, `driver/mysqldriver/vector_integration_test.go`. Contributes to S-15; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMySQLVectorSQL` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestMariaDBVectorParity` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestMySQLVectorCapability` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-61] `TestMariaDBVectorParity` returns the same top-k ids as pgvector exact search; `TestMySQLVectorCapability` asserts distance queries are ErrUnsupported where the server lacks distance functions; fails if a full scan is substituted silently.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red run: branch worktree-agent-a6796cc82be02e264-red (implementation reverted, tests kept; deleted), ci-agent run 42460111115: unit failed TestMySQLVectorSQL, TestMySQLVectorBindWrapping, TestMySQLVectorSensitiveArg, TestMySQLVectorType, TestMariaDBVectorIndexRules, TestMariaDBVectorIndexClause, TestDialectValues; mysql84/mysql97/mariadb integration failed.
- Green run: ci-agent run 42460943118 on 36d1f7e2b0f41784a703bbb9b3794b109b03d8e8: lint and fuzz-smoke success; `ok driver/mysqldriver` (TestMariaDBVectorParity, TestMySQLVectorCapability) on mysql84, mysql97 and mariadb; internal/vectorparity ok. Only internal/docscheck fails there, on docs/PRIVACY_RULES.md and docs/momovet.md merged from sprint-5 (other stories).
- Deviations from the plan text: the helpers live in root vector_mysql.go and `momo.MySQLVectorType` (not migrate/mysql, which another agent builds); pgvector parity is checked through a shared Go reference (internal/vectorparity, TestPostgresVectorParityReference) because the MariaDB job has no PostgreSQL; TestMySQLVectorCapability uses VERSION() plus MySQLVectorType since no migrate/mysql engine exists yet.
- Re-verified on current sprint-7 code: ci-agent run 42780640447 on 8cf94c3ee066c5a183bd60c56ec1b3afeb505e37, all 9 jobs success. The tests live in driver/mysqldriver/vector_integration_test.go (TestMariaDBVectorParity, TestMySQLVectorCapability, integration jobs mysql84, mysql97, mariadb ok) and col_vector_mysql_test.go (unit ok).
- Criterion 4 (no silent full scan): TestMariaDBVectorParity now runs EXPLAIN FORMAT=JSON on the indexed query and requires the VECTOR INDEX ("key": "emb"), with the forced exact path ("+ 0") as a control that must not show it. Passed on the mariadb job of ci-agent run 42785740809 (driver/mysqldriver ok; that run's other failures were docs, fixed after).
