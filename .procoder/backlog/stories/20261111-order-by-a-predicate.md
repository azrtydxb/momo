# ORDER BY a predicate

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #110. `OrderBy` takes `Orderable`, and a `Predicate` is not one, so
`ORDER BY (col IS NULL)` needs `momo.SQL("col IS NULL")`.

Reproduce: passing a Predicate such as `col.IsNull()` to the `OrderBy` of a
select does not compile.

Observed: no typed way to order by a boolean condition. Expected: a predicate
orders with Asc and Desc, rendered `ORDER BY (<predicate>) ASC`, with NULLS
placement chainable as on columns.

Root cause: `Predicate` is the interface `Expression[bool]`, which has no
`order()` method, and `Order` has a `Desc` field, so neither the interface nor
`Order` can carry `Asc()`/`Desc()` methods. The fix is `momo.OrderByPredicate(p)`
(the alternative the issue names), a `PredicateOrder` with `Asc()` and `Desc()`
returning an `Order` over a parenthesised expression.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] `OrderByPredicate(p).Asc()` and `.Desc()` render `ORDER BY (p) ASC|DESC` on PostgreSQL, SQLite and MySQL
- [ ] bound arguments inside the predicate keep their order relative to WHERE arguments
- [ ] `(col IS NULL) ASC` sorts NULLs last, and DESC first, on PostgreSQL, MySQL and MariaDB
- [ ] docs/ai-guide.md and docs/upgrading.md (0.8.0) describe it; docs/reference.md regenerated from CI

## Evidence

- Red: ci-agent run 42993252352 on worktree-agent-red-110 (test-only commit 9e8f392):
  `./select_golden_test.go:74:17: undefined: momo.OrderByPredicate`,
  `./expr_ops_integration_test.go:69:20: undefined: momo.OrderByPredicate`,
  `FAIL github.com/azrtydxb/momo [build failed]`.
- Run 42993325388 (b6fa5c0): the new tests pass; only `TestReferenceListsEveryIdentifier`
  and `gen-check` fail because docs/reference.md lacked 4 identifiers, now taken from
  that run's regenerated-docs artifact.
- Green run: pending.
