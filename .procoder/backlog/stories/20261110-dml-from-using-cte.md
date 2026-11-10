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
- MySQL multi-table UPDATE/DELETE forms rendered; SQLite DELETE ... USING and non-PostgreSQL data-modifying CTE and MySQL WITH-before-INSERT return ErrUnsupported (golden tests above). Docs: docs/ai-guide.md, docs/capabilities.md, docs/reference.md.
- Unverified: MySQL multi-table forms are golden-tested only (the integration tests for them ran on the MySQL jobs, which passed).
