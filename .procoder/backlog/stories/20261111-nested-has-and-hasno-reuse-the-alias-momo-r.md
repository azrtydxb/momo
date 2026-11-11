# Nested Has and HasNo reuse the alias momo_r

Status: open
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

- [ ] each nesting level renders its own child alias (`momo_r`, `momo_r2`, `momo_r3`) and each many-to-many level its own join alias (`momo_j`, `momo_j2`); a single-level predicate renders as before
- [ ] a nested `Has`, `HasNo` or `CountOrder` whose parent is the enclosing child correlates on the enclosing alias; `HasFrom` keeps its explicit alias
- [ ] `As("momo_r")` in a scope predicate names the child of the subquery the scope belongs to, at any depth
- [ ] unit tests pin the SQL for two and three levels, HasNo outside and inside, and a nested many-to-many level
- [ ] a PostgreSQL integration test runs a nested `Has` inside `HasNo` and returns the right rows
- [ ] a regression test pins the fix: red before the change, green after
- [ ] docs and `docs/upgrading.md` describe the aliases

## Evidence

<!-- Filled at close time. -->
