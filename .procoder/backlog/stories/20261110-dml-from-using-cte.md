# Update/Delete: UPDATE ... FROM, DELETE ... USING and data-modifying CTEs (issue #64)

Status: done
Created: 2026-11-10
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

`UpdateQuery` has no `FROM`, no statement has `With` on DML, and a DML statement cannot be a CTE body. Kuvryn Orbit has about 17 `UPDATE ... FROM` statements, and an append CTE that inserts and then `pg_notify`s. The ask is in issue #64.

## Acceptance criteria

- [x] `UpdateQuery.From(source)` and `DeleteQuery.Using(source)` take a table or subquery source typed like a join, and the WHERE can reference its columns (unit tests pin SQL; PostgreSQL integration test)
- [x] `With`/`WithRecursive` work on INSERT, UPDATE and DELETE (tests)
- [x] an INSERT/UPDATE/DELETE with RETURNING can be a CTE body of a SELECT or another DML statement (`WITH x AS (INSERT ... RETURNING id) SELECT ...`) (integration test)
- [x] MySQL renders the multi-table UPDATE/DELETE forms where they exist, or refuses with `ErrUnsupported`; SQLite likewise (tests); docs updated

## Evidence

CI: ci-agent run 42779114603, headSha cc44bfe682a6aa27f15aaaad2c96ff8e34b5fe5c, all jobs green.

- From/Using: update.go `UpdateQuery.From`, delete.go `DeleteQuery.Using`; unit `TestUpdateFromGoldenSQL`, `TestUpdateFromSubquery`, `TestDeleteUsingGoldenSQL` (write_with_test.go); PostgreSQL `TestUpdateFromIntegration`, `TestDeleteUsingIntegration` (write_with_integration_test.go).
- With/WithRecursive on INSERT/UPDATE/DELETE: write_with.go `writeWithClause`; `TestDMLWithClause`, `TestInsertWith`, `TestDMLWithIntegration`.
- DML as CTE body: `isSubquery`/`AppendSQL` on the three builders; `TestDMLAsCTEBody`, `TestDMLAsCTEIntegration`.
- MySQL multi-table UPDATE/DELETE forms: covered by golden SQL tests only (`TestUpdateFromGoldenSQL`, `TestDeleteUsingGoldenSQL`, `TestVersionedUpdateFrom`, `TestDMLWithClause`). The integration tests that run them (`TestUpdateFromIntegration`, `TestDeleteUsingIntegration`) are in the MySQL jobs, but their per-dialect pass was not read from CI logs, so it is not claimed here.
- Refused with ErrUnsupported: SQLite DELETE ... USING, MySQL RETURNING with From/Using, non-PostgreSQL data-modifying CTE body, MySQL WITH before INSERT, MariaDB WITH before UPDATE/DELETE (unverified, so refused: `TestDMLWithMariaDBRefused`).
- Guards: Where with From/Using must reference the target (`TestFromUsingGuard`); BulkUpdate with With/From is ErrValidation (`TestBulkUpdateRejectsWithAndFrom`); an INSERT CTE body over MaxParams is ErrLimitExceeded (`TestInsertCTEBodyParamLimit`).
