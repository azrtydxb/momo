# Alias select-list expressions for subqueries and CTEs

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Queries need SELECT license_id, max(version) AS version so a subquery in FROM or a CTE can expose an aggregate under a name the outer query references (issue #15). Today only positional CTE column names via With work. Done means a typed expression can be aliased in the select list and referenced from the outer query.

## Acceptance criteria

- [x] expr As name renders expression AS quoted name in the select list (golden test per dialect)
- [x] An aliased aggregate in a subquery used in FROM or a CTE is referenced by name from the outer query (integration test on CI)
- [x] Aliases are quoted and validated like other identifiers (test with a reserved word and an invalid name)
- [x] docs show the subquery and CTE examples

## Evidence

- TestAliasGolden (typed_expr_test.go) per dialect; also TestOrderByComputedExpressions alias case, TestGroupByRejectsAlias (expr_ops_test.go); CI run 42376165674 on 687bcf5
- TestAliasedAggregateReferencedByName (typed_expr_integration_test.go) reads the alias from a FROM subquery and a CTE; integration job runs with MOMO_REQUIRE_INTEGRATION=1; CI run 42376165674 on 687bcf5
- TestAliasValidation (typed_expr_test.go) quotes a reserved word and refuses invalid names; CI run 42376165674 on 687bcf5
- docs/ai-guide.md and docs/capabilities.md show the subquery and CTE examples; CI run 42376165674 on 687bcf5
