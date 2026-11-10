# Update/Delete: UPDATE ... FROM, DELETE ... USING and data-modifying CTEs (issue #64)

Status: open
Created: 2026-11-10
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

`UpdateQuery` has no `FROM`, no statement has `With` on DML, and a DML statement cannot be a CTE body. Kuvryn Orbit has about 17 `UPDATE ... FROM` statements, and an append CTE that inserts and then `pg_notify`s. The ask is in issue #64.

## Acceptance criteria

- [ ] `UpdateQuery.From(source)` and `DeleteQuery.Using(source)` take a table or subquery source typed like a join, and the WHERE can reference its columns (unit tests pin SQL; PostgreSQL integration test)
- [ ] `With`/`WithRecursive` work on INSERT, UPDATE and DELETE (tests)
- [ ] an INSERT/UPDATE/DELETE with RETURNING can be a CTE body of a SELECT or another DML statement (`WITH x AS (INSERT ... RETURNING id) SELECT ...`) (integration test)
- [ ] MySQL renders the multi-table UPDATE/DELETE forms where they exist, or refuses with `ErrUnsupported`; SQLite likewise (tests); docs updated
