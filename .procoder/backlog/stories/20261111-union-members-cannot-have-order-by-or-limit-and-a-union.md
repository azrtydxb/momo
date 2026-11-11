# Union members cannot have ORDER BY or LIMIT and a union cannot run at the top level

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #99. `momo.Union(all, members...)` rendered every member without
parentheses and returned only a Subquery, usable as a derived table.

Reproduce: build `momo.Union(true, a, b)` where `a` and `b` are
`momo.Select[...]` queries that each call `.OrderBy(...).Limit(n)` (the top N of
each kind), then render it. Observed: the members' ORDER BY and LIMIT are
written bare, `SELECT ... ORDER BY x LIMIT $1 UNION ALL SELECT ... ORDER BY x
LIMIT $2`, which PostgreSQL and MySQL reject (or bind to the whole union), so a
per-member top N is impossible. A union also could not be executed directly with
an outer ORDER BY and LIMIT. Expected: each such member is grouped so its
clauses bind to it, and a union runs at the top level into a model, projection,
tuple row or plain struct.

Root cause: `setOp.AppendSQL` rendered members through their bare `render`
method on every dialect (to keep SQLite happy, which rejects parenthesised
compound members) and no top-level executable union type existed.

Fix: a member with ORDER BY, LIMIT, OFFSET, a row lock, or a nested set
operation renders in parentheses on PostgreSQL and MySQL and as
`SELECT * FROM (member)` on SQLite; other members stay bare. New
`momo.SelectUnion[T]` returns a `UnionQuery[T]` (`OrderBy`, `Limit`, `Offset`,
`SQL`, `All`, `Rows`).

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] a member with ORDER BY, LIMIT, OFFSET or a lock renders in parentheses on PostgreSQL and MySQL, as a derived table on SQLite, and a plain member stays bare (unit SQL tests per dialect)
- [ ] a union runs at the top level with an outer ORDER BY, LIMIT and OFFSET into a tuple row and a plain struct, and per-member limits hold, on PostgreSQL, MySQL and SQLite (integration tests)
- [ ] a row lock in a member on SQLite returns ErrUnsupported
- [ ] docs/upgrading.md, docs/ai-guide.md and docs/projections.md describe the change; docscheck and apidiff-docs pass

## Evidence

<!-- Filled at close time. -->
