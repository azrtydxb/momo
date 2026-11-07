# Aggregate ORDER BY, WITHIN GROUP and typed string_agg, array_agg, jsonb_agg

Status: in progress
Created: 2026-11-08
Epic: momo-05-writes-queries
Sprint: 005

## Description

Issue #44: Func[T] renders name(args) only, so string_agg(x, ', ' ORDER BY y), percentile_cont(0.5) WITHIN GROUP (ORDER BY z), jsonb_agg and jsonb_build_object need TypedSQL. Done means typed helpers express them with the right Go result types and compose with CoalesceExpr, with PostgreSQL first and SQLite and MySQL mapped where the dialect has an equivalent, ErrUnsupported otherwise.

## Acceptance criteria

- [ ] AggExpr renders agg(args ORDER BY ...) through OrderBy and an ordered-set WITHIN GROUP through WithinGroup (golden tests per dialect)
- [ ] PercentileCont and PercentileDisc are typed and refuse a missing WithinGroup or a fraction outside 0 to 1 (validation test; percentile on a known distribution on CI)
- [ ] StringAgg renders string_agg, group_concat and GROUP_CONCAT SEPARATOR with an optional ORDER BY (golden tests; ordered result on CI)
- [ ] ArrayAgg returns []T on PostgreSQL and ErrUnsupported elsewhere (golden test; scan on CI)
- [ ] JSONBAgg, JSONBBuildObject and ToJSONB return types.JSON values and compose with CoalesceExpr and EmptyJSONArray (golden tests; array shape and empty group on CI)
- [ ] Unsafe separators, keys and aggregate names fail with ErrValidation
- [ ] docs list the helpers

## Evidence

Pending CI.
