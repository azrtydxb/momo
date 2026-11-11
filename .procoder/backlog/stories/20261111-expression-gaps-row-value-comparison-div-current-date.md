# Expression gaps: row-value comparison, Div, CURRENT_DATE, infinity, Lit

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #97, found while porting Kuvryn-Owl to 0.7. Five expression gaps
each forced a `momo.SQL` or `TypedSQL` fragment:

1. No row-value ordered comparison: `(w.created_at, w.id) < (me.at, $1)`.
2. No `sqlfn.Div` and no `NullIf`: `avg(final_marks / NULLIF(max_marks, 0))`.
3. No builder for `CURRENT_DATE`, `'infinity'::timestamptz` or `'-infinity'::timestamptz`.
4. No inlined literal: `COALESCE(col, 'none')` in both SELECT and GROUP BY binds
   two parameters, and PostgreSQL rejects the query (42803, the expressions differ).
5. `ExprSelect` had only From and Where: no GROUP BY, HAVING, ORDER BY or joins, so a
   grouped `INSERT ... SELECT` with more than 10 columns had no source.

Reproduce: try to build any of the above with the typed API. Observed: not
expressible (compile error, or a bound parameter that PostgreSQL rejects in
GROUP BY). Expected: typed builders with the documented SQL.

Root cause: the API never had these builders. Fix: `momo.Row(...).Lt/Le/Gt/Ge`,
`sqlfn.Div/DivExpr/NullIf/NullIfExpr`, `sqlfn.CurrentDate/Infinity/NegInfinity`,
`momo.Lit` (numbers, bool, string and midnight-UTC time.Time; strings with a backslash or NUL and dates with a time of day are refused), and
`Join/LeftJoin/CrossJoin/GroupBy/Having/OrderBy` on `ExprSelect`.

## Acceptance criteria

- [x] row-value Lt, Le, Gt and Ge render `(a, b) op (x, y)` on every dialect, and mismatched or short rows fail with ErrValidation
- [x] sqlfn.Div, DivExpr, NullIf and NullIfExpr render per dialect, and a zero divisor guarded by NullIf runs on PostgreSQL
- [x] CurrentDate renders on every dialect; Infinity and NegInfinity render on PostgreSQL and fail with ErrUnsupported elsewhere
- [x] momo.Lit renders numbers, bool, strings (quote doubled) and midnight-UTC dates inline; it refuses non-finite floats, a string with a backslash or NUL, and a date with a time of day; the same Lit in SELECT and GROUP BY runs on PostgreSQL (TestLitGolden, TestLitRefusesUnsafeValues, TestLitStringGroupByExecutes)
- [x] ExprSelect supports joins, GROUP BY, HAVING and ORDER BY, validated and run as a grouped INSERT ... SELECT on PostgreSQL
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- Red: ci-agent run 42979501760 (branch worktree-agent-red-97, tests only, `cf11600`) failed to compile: `./expr_row_lit_test.go:27:23: undefined: momo.RowExpr`, `undefined: momo.Row`, `undefined: momo.Lit`, `sqlfn/sqlfn_expr97_test.go:25:14: undefined: sqlfn.DivExpr`, `undefined: sqlfn.NullIf`. The branch is deleted.
- Green: ci-agent run 42986531687 and docs run 42986531735 succeeded on headSha 7b013c972dc334e3c8126dbdca311bb860b5285d (branch worktree-agent-a8396fdcdb29ac8fc). A first fix run, 42979652626 on 7db7819, failed on test-fixture table names, a missing NOT NULL column in the join test, bare doc names and a stale reference.md; all fixed in 7b013c9.
- Criteria: TestRowComparisonGolden, TestRowComparisonValidation, TestRowComparisonExecutes (row values); TestDivAndNullIfGolden, TestNullIfExprGolden, TestDivNullIfExecutes (Div, NullIf); TestCurrentDateAndInfinityGolden, TestInfinityComparison, TestCurrentDateAndInfinityExecute (CURRENT_DATE, infinity); TestLitGolden, TestLitRejectsNonFinite, TestLitSelectAndGroupByIdentical, TestCompileFailLitString, TestLitGroupByExecutes (Lit); TestExprSelectGroupHavingOrderGolden, TestExprSelectJoinsGolden, TestExprSelectClauseValidation, TestExprSelectGroupedInsertExecutes, TestExprSelectJoinExecutes (ExprSelect). All passed in the green run.
- Docs changed: docs/ai-guide.md, docs/capabilities.md, docs/upgrading.md (new "Upgrading to 0.8.0" section), docs/reference.md (from the regenerated-docs artifact).
- Review round: red ci-agent run 42994326154 (branch worktree-agent-red-97b, tests only, `282216a3`, deleted) failed to compile: `./expr_lit_strings_test.go:23:33: string does not satisfy momo.Literal`. Green: ci-agent run 42994601430 succeeded on headSha b76ff08c (gen-check passed with the committed docs/reference.md; no regenerated-docs artifact was produced). New tests: TestLitRefusesUnsafeValues, TestLitStringSelectAndGroupByIdentical, TestLitStringGroupByExecutes, TestLitFloatBoolDateExecutes, TestExprSelectBoundArgsInClauseOrder, TestRowComparisonExecutes and TestCurrentDateExecutes (every dialect).
