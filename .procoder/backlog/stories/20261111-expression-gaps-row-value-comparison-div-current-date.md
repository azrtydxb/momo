# Expression gaps: row-value comparison, Div, CURRENT_DATE, infinity, Lit

Status: open
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
`momo.Lit` (numbers, bool and time.Time only, so it is injection-safe by type), and
`Join/LeftJoin/CrossJoin/GroupBy/Having/OrderBy` on `ExprSelect`.

## Acceptance criteria

- [ ] row-value Lt, Le, Gt and Ge render `(a, b) op (x, y)` on every dialect, and mismatched or short rows fail with ErrValidation
- [ ] sqlfn.Div, DivExpr, NullIf and NullIfExpr render per dialect, and a zero divisor guarded by NullIf runs on PostgreSQL
- [ ] CurrentDate renders on every dialect; Infinity and NegInfinity render on PostgreSQL and fail with ErrUnsupported elsewhere
- [ ] momo.Lit renders numbers, bool and dates inline, rejects non-finite floats, does not compile for a string, and the same Lit in SELECT and GROUP BY runs on PostgreSQL
- [ ] ExprSelect supports joins, GROUP BY, HAVING and ORDER BY, validated and run as a grouped INSERT ... SELECT on PostgreSQL
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
