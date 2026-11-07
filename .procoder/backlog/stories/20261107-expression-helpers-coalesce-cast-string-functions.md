# Expression helpers: COALESCE of expressions, CAST, lower/upper/length, concat

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Beyond #17, the kuvryn-license audit found more expressions that need TypedSQL (issue #20): COALESCE with an expression fallback such as COALESCE(l.site_name, r.license_id), CAST(SUM(x) AS BIGINT), lower/upper/length on strings, and string concatenation. Done means typed helpers cover these and compose with select lists, Where, GroupBy, OrderBy and SetExpr.

## Acceptance criteria

- [x] COALESCE over two or more typed expressions of the same type renders and type-checks (golden test)
- [x] CAST to a named SQL type renders per dialect with the result type chosen by the caller and the type name validated (golden test, invalid type name refused)
- [x] lower, upper, length and concat render per dialect (golden tests; concat uses the dialect's operator or function)
- [x] An integration query on CI uses each helper and asserts rows
- [x] docs list the helpers

## Evidence

- TestCoalesceCastConcatGolden (typed_expr_more_test.go); CI run 42376165674 on 687bcf5
- TestCoalesceCastConcatGolden and TestCastGoldenSQLite (expr_ops_test.go); TestCastTypeValidation refuses invalid type names; CI run 42376165674 on 687bcf5
- TestStringFunctionsGolden and TestCoalesceCastConcatGolden per dialect (MySQL char_length and CONCAT); CI run 42376165674 on 687bcf5
- TestStringCastCoalesceHelpers (typed_expr_more_integration_test.go) uses each helper and asserts rows; integration job runs with MOMO_REQUIRE_INTEGRATION=1; CI run 42376165674 on 687bcf5
- docs/ai-guide.md and docs/capabilities.md list the helpers; CI run 42376165674 on 687bcf5
