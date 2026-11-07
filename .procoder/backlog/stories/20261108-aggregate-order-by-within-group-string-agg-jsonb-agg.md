# Aggregate ORDER BY, WITHIN GROUP and typed string_agg, array_agg, jsonb_agg

Status: done 2026-11-08
Created: 2026-11-08
Epic: momo-05-writes-queries
Sprint: 005

## Description

Issue #44: Func[T] renders name(args) only, so string_agg(x, ', ' ORDER BY y), percentile_cont(0.5) WITHIN GROUP (ORDER BY z), jsonb_agg and jsonb_build_object need TypedSQL. Done means typed helpers express them with the right Go result types and compose with CoalesceExpr, with PostgreSQL first and SQLite and MySQL mapped where the dialect has an equivalent, ErrUnsupported otherwise.

## Acceptance criteria

- [x] AggExpr renders agg(args ORDER BY ...) through OrderBy and an ordered-set WITHIN GROUP through WithinGroup (golden tests per dialect)
- [x] PercentileCont and PercentileDisc are typed and refuse a missing WithinGroup or a fraction outside 0 to 1 (validation test; percentile on a known distribution on CI)
- [x] StringAgg renders string_agg, group_concat and GROUP_CONCAT SEPARATOR with an optional ORDER BY (golden tests; ordered result on CI)
- [x] ArrayAgg returns []T on PostgreSQL and ErrUnsupported elsewhere (golden test; scan on CI)
- [x] JSONBAgg, JSONBBuildObject and ToJSONB return types.JSON values and compose with CoalesceExpr and EmptyJSONArray (golden tests; array shape and empty group on CI)
- [x] Unsafe separators, keys and aggregate names fail with ErrValidation
- [x] docs list the helpers

## Evidence

TestAggregateGolden, TestAggregateValidation, TestAggregateImmutable (expr_agg_test.go) and TestStringAggOrderExecutes, TestPercentileOnKnownDistribution, TestArrayAggExecutes, TestJSONBAggShapeAndEmptyGroup (expr_agg_integration_test.go); docs/ai-guide.md and docs/capabilities.md list the helpers; CI run 42411790531 on 5656d64 green.
