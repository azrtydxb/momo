# Schema: unique expression indexes (issue #55)

Status: done
Created: 2026-11-10
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

A case-insensitive identity needs `UNIQUE (lower(email))`, and a per-tenant unique name needs `UNIQUE (company_id, lower(name))`. `Config.IndexExpr` has no unique option, and `--from-db` drops such indexes. The ask is in GitHub issue #55, from the Kuvryn Owl Enterprise port.

## Acceptance criteria

- [x] `c.IndexExpr(name, exprs...).Unique()` (columns and expressions mixed, with `Where` for a partial index) renders `CREATE UNIQUE INDEX ... (company_id, lower(name))`; a unit test pins the SQL
- [x] the PostgreSQL inspector reads a unique expression index back, so diff is stable (no change on a second run), and `--from-db` emits the declaration (integration test)
- [x] a duplicate under the expression fails with momo's unique-violation error kind (integration test)
- [x] docs describe it; issue #55 is referenced in the commit

## Evidence

CI: ci-agent run 42780348454, headSha 69fa9dcd7162eae24ea58216e294601a079f3ec0, all nine jobs green.

- `c.IndexExpr(name, exprs...).Unique()`, mixed columns and expressions (an argument equal to a column name is a column key), `Where` for a partial index; the SQL is pinned by migrate/postgres/unique_expr_declared_test.go (TestDeclaredUniqueExpressionIndexes, rendered-SQL assertions) and the schema by gen/unique_expr_test.go.
- Inspector read-back with a stable second diff, and `--from-db` emission: TestDeclaredUniqueExpressionIndexes (schema.Equal and an empty second diff), gen/fromdb/fromdb_test.go (TestGenerateFromDBUniqueExpressionIndex), cmd/momo/generate_fromdb_integration_test.go (accounts_email_lower_key).
- A duplicate under the expression fails with momo.ErrUniqueViolation: TestDeclaredUniqueExpressionIndexes (errors.Is).
- Docs: docs/ai-guide.md, docs/migrations.md, docs/reference.md; commit trailer Refs #55.
- Still undeclarable from a database: an expression key carrying DESC, NULLS or an operator class (warned).

