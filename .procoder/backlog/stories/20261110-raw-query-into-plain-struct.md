# Raw SQL: Query[T] into a plain struct, no generated metadata

Status: open
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`Query`/`QueryRows` accept a model, a generated projection or `Row1`..`Row10`. An ad-hoc struct does not work, so a raw read with more than ten columns needs a `momo.Projection` and a generator run. Scan by column name into any struct: exported fields, `momo:"column:x"` tags, snake_case of the field name otherwise, with cached reflection. GitHub issue #75.

## Acceptance criteria

- [ ] the tests are shown failing on CI before the change
- [ ] `momo.Query[T]` and `momo.QueryRows[T]` scan into a plain struct with no generated metadata, by column name; a test reads 13 columns into an ad-hoc struct on PostgreSQL, MySQL and SQLite
- [ ] a result column with no matching field, or a field with no matching column, is an error naming it (or a documented rule says which is allowed)
- [ ] the reflection plan is cached per type; a benchmark shows no per-row reflection cost beyond the first call

## Evidence
