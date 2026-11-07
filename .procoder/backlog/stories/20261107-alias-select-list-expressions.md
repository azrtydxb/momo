# Alias select-list expressions for subqueries and CTEs

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Queries need SELECT license_id, max(version) AS version so a subquery in FROM or a CTE can expose an aggregate under a name the outer query references (issue #15). Today only positional CTE column names via With work. Done means a typed expression can be aliased in the select list and referenced from the outer query.

## Acceptance criteria

- [ ] expr As name renders expression AS quoted name in the select list (golden test per dialect)
- [ ] An aliased aggregate in a subquery used in FROM or a CTE is referenced by name from the outer query (integration test on CI)
- [ ] Aliases are quoted and validated like other identifiers (test with a reserved word and an invalid name)
- [ ] docs show the subquery and CTE examples

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
