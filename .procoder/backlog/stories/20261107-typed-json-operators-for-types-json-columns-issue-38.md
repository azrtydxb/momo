# Typed JSON operators for types.JSON columns (issue 38)

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: -

## Description

A service with many jsonb columns (NovaCRM filters after->>'via') needs typed JSON predicates: JSONText, JSONPath, JSONContains and JSONHasKey over types.JSON[T] columns, usable in Where, OrderBy and Columns, with bound keys and values, sensitive flags carried from the column, and the SQL NULL versus JSON null behaviour documented. GitHub issue 38.

## Acceptance criteria

- [ ] Golden tests assert the PostgreSQL, SQLite and MySQL SQL and bound args of each operator, ErrValidation for empty or unrepresentable keys and ErrUnsupported for SQLite containment
- [ ] An integration test on PostgreSQL asserts result rows for each operator, JSON null, SQL NULL and missing key, and ORDER BY on an extraction
- [ ] The extraction carries the column's sensitive flag and is marked nullable
- [ ] docs/capabilities.md and the expr_json.go comment document the operators and the NULL semantics

## Evidence

