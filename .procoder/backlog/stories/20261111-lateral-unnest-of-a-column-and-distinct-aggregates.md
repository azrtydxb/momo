# LATERAL unnest of a column and DISTINCT aggregates

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #98. Owl expands a `text[]` column with `CROSS JOIN LATERAL
unnest(q2.flags) AS fl(flag)` and aggregates with `array_agg(DISTINCT
fl.flag)`. Momo could express neither: `UnnestValues`/`UnnestValuesAs`
(`select_srf.go`) unnest only bound Go slices, `Lateral` takes only a
subquery, and `agg.Call` had no `DISTINCT`.

Reproduce: try to build `SELECT ... FROM t CROSS JOIN LATERAL unnest(t.flags)
AS fl(flag)` or `array_agg(DISTINCT x)` with the query builder. There is no API
for it; the only route is raw SQL.

Expected: a source `momo.UnnestColumn(col).As(alias, name)` usable in a join or
FROM item, and `agg.Call.Distinct()` for array_agg, string_agg, count and
jsonb_agg.

Root cause: missing feature, not a regression. The unnest source only took
bound values, and the aggregate spec had no distinct flag.

## Acceptance criteria

- [ ] `momo.UnnestColumn(col).As(alias, name)` renders `LATERAL unnest(col) AS "alias" ("name")` as a join or FROM source, reads back through `Column()`, and gives `ErrUnsupported` on SQLite and MySQL and `ErrValidation` for a missing or unsafe alias or a nil array
- [ ] `agg.Call.Distinct()` renders `DISTINCT` inside array_agg, string_agg, count (via `agg.Func`) and jsonb_agg, before ORDER BY and before FILTER, and gives `ErrUnsupported` where a dialect lacks it and `ErrValidation` on ordered-set aggregates
- [ ] a PostgreSQL integration test expands a `text[]` column (NULL array yields no rows) and runs the DISTINCT aggregates against real data
- [ ] docs updated: `docs/upgrading.md` (0.8.0), `docs/capabilities.md`, `docs/ai-guide.md`, and `docs/reference.md` from the CI artifact
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time. -->
