# momo-05 Task 5: Projections, aggregates, GROUP BY and HAVING

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 5: Projections, aggregates, GROUP BY and HAVING (`.procoder/plans/momo-05-writes-queries.md:392`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `integration_aggregates_test.go`, `expr.go`.

## Acceptance criteria

- [x] `TestAggregatesGroupHaving` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test -tags integration -run Aggregates .` failed to build: `undefined: momo.CountAll` and `Expression[int64] has no field or method Ge`.
- GREEN: TestAggregatesGroupHaving, TestAggregatesSumMinMax and TestAggregatesSQL pass on PostgreSQL with -race; full root package passes with and without the integration tag.
- Mutation: changing `Agg.Gt` from `>` to `>=` made TestAggregatesGroupHaving fail (tenant 2 leaked into the result); reverted.
- Implementation: expr.go now has `Agg[T]` with Eq/Ne/Lt/Le/Gt/Ge/Asc/Desc, `CountAll`, `Sum[Number]`, `Avg`, `Min/Max[Ordered]`; the former `Count()` was renamed `CountAll()` (no callers).
