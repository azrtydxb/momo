# Expression helpers: COALESCE of expressions, CAST, lower/upper/length, concat

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Beyond #17, the kuvryn-license audit found more expressions that need TypedSQL (issue #20): COALESCE with an expression fallback such as COALESCE(l.site_name, r.license_id), CAST(SUM(x) AS BIGINT), lower/upper/length on strings, and string concatenation. Done means typed helpers cover these and compose with select lists, Where, GroupBy, OrderBy and SetExpr.

## Acceptance criteria

- [ ] COALESCE over two or more typed expressions of the same type renders and type-checks (golden test)
- [ ] CAST to a named SQL type renders per dialect with the result type chosen by the caller and the type name validated (golden test, invalid type name refused)
- [ ] lower, upper, length and concat render per dialect (golden tests; concat uses the dialect's operator or function)
- [ ] An integration query on CI uses each helper and asserts rows
- [ ] docs list the helpers

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
