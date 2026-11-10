# Schema: unique expression indexes (issue #55)

Status: open
Created: 2026-11-10
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

A case-insensitive identity needs `UNIQUE (lower(email))`, and a per-tenant unique name needs `UNIQUE (company_id, lower(name))`. `Config.IndexExpr` has no unique option, and `--from-db` drops such indexes. The ask is in GitHub issue #55, from the Kuvryn Owl Enterprise port.

## Acceptance criteria

- [ ] `c.IndexExpr(name, exprs...).Unique()` (columns and expressions mixed, with `Where` for a partial index) renders `CREATE UNIQUE INDEX ... (company_id, lower(name))`; a unit test pins the SQL
- [ ] the PostgreSQL inspector reads a unique expression index back, so diff is stable (no change on a second run), and `--from-db` emits the declaration (integration test)
- [ ] a duplicate under the expression fails with momo's unique-violation error kind (integration test)
- [ ] docs describe it; issue #55 is referenced in the commit
