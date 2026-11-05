# momo-05 Task 6: Subqueries, CTEs (including recursive) and set operations

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 6: Subqueries, CTEs (including recursive) and set operations (`.procoder/plans/momo-05-writes-queries.md:476`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `integration_subquery_test.go`.

## Acceptance criteria

- [x] `TestSubqueriesCTEsRecursive` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test -tags integration -run TestSubquer .` failed to build (`undefined: momo.RawSubquery`); the plan's `momo.Subquery(sql)` cannot exist because `Subquery` is already the interface type, so the raw constructor is `RawSubquery`, and `WithRecursive` is the existing SelectQuery method rather than a curried helper. Green: same command on PostgreSQL ok (TestSubqueriesCTEsRecursive, TestSubqueryShapesExecute).
- Scope check needed no change: unit test TestScopeAcceptsSubqueryAndCTEShapes (both dialects) accepts a self-referencing recursive CTE, CTE from main query and IN, select-list scalar, IN, correlated EXISTS (one and two levels) and still rejects an out-of-scope table.
- Mutation: disabling the hand-off of unresolved qualifiers to the enclosing scope in popScope failed the correlated cases; restored.
- Gate: go build, go vet, make staticcheck clean; `procoder check` 0 blocking findings.
