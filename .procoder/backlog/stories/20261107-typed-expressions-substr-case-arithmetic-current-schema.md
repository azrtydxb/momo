# Typed expressions: substr, CASE, column arithmetic, current_schema

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Common SQL in real stores needs momo.SQL fragments today (issue #17): substr in GROUP BY, CASE WHEN in an upsert set, column arithmetic such as quota = quota + $1 or answers = t.answers + EXCLUDED.answers, and current_schema(). Done means typed helpers cover these and compose with Set, upsert DoUpdate, Where, GroupBy and select lists.

## Acceptance criteria

- [ ] Substr (and a generic typed function call helper) render per dialect (golden tests)
- [ ] A typed CASE WHEN ... THEN ... ELSE ... END builder renders and type-checks branch values (golden test plus compile-fail test for mismatched types)
- [ ] Column arithmetic (Add, Sub, Mul) works in SetExpr and upsert sets including against Excluded (golden tests, integration test on CI)
- [ ] current_schema renders on PostgreSQL and is refused or mapped on other dialects (test)
- [ ] docs list the helpers

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
