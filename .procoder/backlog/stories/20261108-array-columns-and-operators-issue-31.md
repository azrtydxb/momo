# Array columns and operators (issue #31)

Status: done 2026-11-08
Created: 2026-11-08
Epic: momo-05-writes-queries
Sprint: -

## Description

Split from momo-05 Task 24 (story 125). A user with `text[]` columns needs the generator to map scalar slices to PostgreSQL arrays and typed predicates for them: contains (`@>`), contained-by (`<@`), overlaps (`&&`) and `= ANY(...)`, with values bound and masked for sensitive columns. Delivered for GitHub issue #31. The JSON half of Task 24 stays in story 125 and issue #38.

## Acceptance criteria

- [x] `TestArrayOperatorsGolden` pins the SQL and the single array bind of each operator on PostgreSQL, including the empty and hostile-value cases; `TestArrayOperatorsUnsupportedOffPostgres` asserts ErrUnsupported on SQLite and MySQL.
- [x] `TestArrayOperatorsMaskSensitiveColumn` shows each of the four predicates yields RedactedArg in hook events for a sensitive column and the real value for a plain one.
- [x] `TestArrayOperatorsLive`, `TestArrayOperatorsBindHostileValues` and `TestArrayContainsOnIntegerColumns` assert result rows on PostgreSQL for contains, contained-by, overlaps and `= ANY`, on text, smallint and bigint arrays.
- [x] `TestGeneratorTypeMapFullTable` and `TestSliceFieldsMapToArrayColumns` show `[]string` maps to `text[]` (also the integer, bool and float slices) and SQLite returns ErrUnsupported.
- [x] `TestArrayColumnsMigrationRoundTrip` shows array columns diff, apply and inspect back to an empty second diff; `TestTypeMapping` round-trips every new row through a live column; `TestNilSliceIntoNotNullArrayColumn` pins the nil-slice behaviour documented in docs/TYPE_MAPPING.md.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

EVIDENCE_PLACEHOLDER
