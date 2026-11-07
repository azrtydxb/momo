# Typed JSON operators for types.JSON columns (issue 38)

Status: closed
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: -

## Description

A service with many jsonb columns (NovaCRM filters after->>'via') needs typed JSON predicates: JSONText, JSONPath, JSONContains and JSONHasKey over types.JSON[T] columns, usable in Where, OrderBy and Columns, with bound keys and values, sensitive flags carried from the column, and the SQL NULL versus JSON null behaviour documented. GitHub issue 38.

## Acceptance criteria

- [x] Golden tests assert the PostgreSQL, SQLite and MySQL SQL and bound args of each operator, ErrValidation for empty or unrepresentable keys and ErrUnsupported for SQLite containment
- [x] An integration test on PostgreSQL asserts result rows for each operator, JSON null, SQL NULL and missing key, and ORDER BY on an extraction
- [x] The extraction carries the column's sensitive flag and is marked nullable
- [x] docs/capabilities.md and the expr_json.go comment document the operators and the NULL semantics

## Evidence

- Goldens: expr_json_test.go (TestJSONOperatorsGolden, TestJSONOperatorErrors, TestJSONFlags, TestJSONTextOrderAndColumns), green in the CI unit jobs.
- Rows: expr_json_integration_test.go TestJSONOperatorsAgainstDatabase, green in the CI postgres integration job on the final commit.
- Docs: docs/capabilities.md row and the comment block in expr_json.go.
