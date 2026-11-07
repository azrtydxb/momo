# Typed expressions: substr, CASE, column arithmetic, current_schema

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Common SQL in real stores needs momo.SQL fragments today (issue #17): substr in GROUP BY, CASE WHEN in an upsert set, column arithmetic such as quota = quota + $1 or answers = t.answers + EXCLUDED.answers, and current_schema(). Done means typed helpers cover these and compose with Set, upsert DoUpdate, Where, GroupBy and select lists.

## Acceptance criteria

- [x] Substr (and a generic typed function call helper) render per dialect (golden tests)
- [x] A typed CASE WHEN ... THEN ... ELSE ... END builder renders and type-checks branch values (golden test plus compile-fail test for mismatched types)
- [x] Column arithmetic (Add, Sub, Mul) works in SetExpr and upsert sets including against Excluded (golden tests, integration test on CI)
- [x] current_schema renders on PostgreSQL and is refused or mapped on other dialects (test)
- [x] docs list the helpers

## Evidence

- TestStringFunctionsGolden (typed_expr_test.go) covers Substr, Func per dialect; TestWhereOnComputedExpressions (expr_ops_test.go) covers Func comparisons; TestFuncAndCurrentSchemaExecute runs Func[T] against the database; CI run 42376165674 on 687bcf5
- TestCaseGolden (typed_expr_test.go) and TestCompileFailTypedExpressions case_mismatch (compilefail_test.go); TestOrderByComputedExpressions and TestWhereOnComputedExpressions cover Case in ORDER BY and WHERE; CI run 42376165674 on 687bcf5
- TestUpsertCaseAndArithmetic (typed_expr_integration_test.go) runs Add against Excluded; golden tests in typed_expr_test.go; CI run 42376165674 on 687bcf5
- TestStringFunctionsGolden renders current_schema on PostgreSQL and DATABASE on MySQL, TestCurrentSchemaRefusedOnSQLite refuses it; TestFuncAndCurrentSchemaExecute runs it on PostgreSQL; CI run 42376165674 on 687bcf5
- docs/ai-guide.md and docs/capabilities.md list the helpers; CI run 42376165674 on 687bcf5
