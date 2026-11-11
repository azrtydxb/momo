# Nested Has and HasNo reuse the alias momo_r

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #89. A `Has` or `HasNo` nested inside the scope of another one
renders both subqueries with the child alias `momo_r` (`relAlias` in
`rel_pred.go`), so the inner correlation compares the inner table with itself.

Reproduce: select documents where `document.Taggings` has a tagging whose
`tagging.Tag.HasFrom("momo_r", ...)` matches a tag name. It renders `... FROM "taggings" AS "momo_r" WHERE ... AND (EXISTS (SELECT 1 FROM "tags" AS "momo_r" WHERE "momo_r"."id" = "momo_r"."tag_id" ...))`
and PostgreSQL fails with `column momo_r.tag_id does not exist`. With a plain
inner `Has` the build fails: `column reference to table "taggings", which is
not in the query's FROM or JOIN sources`.

Observed: the statement fails. Expected: the nested predicate runs and filters
by the enclosing level's row.

Root cause: the alias was a package constant and the inner correlation was
rendered against the parent table name, so neither the alias nor the
correlation depended on the nesting level. Many-to-many joins had the same
problem with the join alias `momo_j`.

## Acceptance criteria

- [x] each nesting level renders its own child alias (`momo_r`, `momo_r2`, `momo_r3`) and each many-to-many level its own join alias (`momo_j`, `momo_j2`); a single-level predicate renders as before
- [x] a nested `Has`, `HasNo` or `CountOrder` whose parent is the enclosing child correlates on the enclosing alias; `HasFrom` keeps its explicit alias
- [x] `As("momo_r")` in a scope predicate names the child of the subquery the scope belongs to, at any depth
- [x] unit tests pin the SQL for two and three levels, HasNo outside and inside, and a nested many-to-many level
- [x] a PostgreSQL integration test runs a nested `Has` inside `HasNo` and returns the right rows
- [x] a regression test pins the fix: red before the change, green after
- [x] docs and `docs/upgrading.md` describe the aliases

## Evidence

- Red: ci-agent run 42978683611 (test-only commit b821f16, branch worktree-agent-red-89, deleted). unit and integration (postgres) failed: TestNestedRelationPredicateAliases subtests two_levels, three_levels, HasNo_outside, HasNo_inside and the nested many-to-many case with "column reference to table taggings, which is not in the query's FROM or JOIN sources"; TestNestedRelationPredicatesRun at rel_pred_nested_integration_test.go:50 with the same error.
- Green: ci-agent run 42986469283, headSha 65b4964ef6d6066153a380ad8a82c7a37cc6947b, all 9 jobs success. An earlier run on 68af887 failed only on the docs fragment ceiling (TestDocsCodeBlocks); the nested tests passed there.
- Review follow-up (momo.RelAlias(level) plus self-reference, Or/Not, sibling and nested many-to-many tests): red run 42992852864 on stub commit 82c1ea6 (branch worktree-agent-red-89b, deleted): TestNestedRelationPredicateAliases/RelAlias_reaches_level_1_from_level_3 and RelAlias_reaches_level_2_from_level_3 failed with "SQL does not contain" at rel_pred_nested_test.go:104. Run 42992977432 on 1f86691 failed only on the stale docs/reference.md; docs/reference.md was then taken from that run's regenerated-docs artifact (headSha matched).
- Final green: ci-agent run 42997814927, headSha 6254ff1e8f9c6eb41c5367f1971dad25a589f3b7, all 9 jobs success. RelAlias is documented in docs/relations.md, docs/ai-guide.md and docs/upgrading.md; docs/reference.md regenerated.
- Goldens: none changed. No golden file contains momo_r and single-level SQL is unchanged.
- Docs changed: docs/relations.md, docs/ai-guide.md, docs/upgrading.md (Upgrading to 0.8.0).

