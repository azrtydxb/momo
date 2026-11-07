# momo-05 Task 24: JSON and array operators (S-32)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 24: JSON and array operators (S-32) (`.procoder/plans/momo-05-writes-queries.md:1997`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-32. Files: `json_array.go`, `json_array_test.go`.

## Scope note

Issue #31 delivered the array half of this task: scalar slices map to PostgreSQL arrays in the generator, and `ArrayContains`, `ArrayContainedBy`, `ArrayOverlaps` and `AnyOf` exist in `array_ops.go`. The JSON operators are tracked in issue #38, so this story closes on the array scope.

## Acceptance criteria

- [x] `TestArrayOperatorsGolden` pins the SQL and the single array bind of each operator on PostgreSQL, including the empty and hostile-value cases; `TestArrayOperatorsUnsupportedOffPostgres` asserts ErrUnsupported on SQLite and MySQL.
- [x] `TestArrayOperatorsLive` and `TestArrayOperatorsBindHostileValues` assert result rows on PostgreSQL for contains, contained-by, overlaps and `= ANY`.
- [x] `TestGeneratorTypeMapFullTable` and `TestSliceFieldsMapToArrayColumns` show `[]string` maps to `text[]` (also `[]int16`, `[]int32`, `[]int64`, `[]bool`, `[]float32`, `[]float64`) and SQLite returns ErrUnsupported.
- [x] `TestArrayColumnsMigrationRoundTrip` shows array columns diff, apply and inspect back to an empty second diff; `TestTypeMapping` round-trips every new row through a live column.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Verified on GitHub Actions, workflow ci-agent run 42411030882 on 5e7a038, conclusion success (unit and integration postgres jobs ran every test named above).
- Earlier runs failed on test-fixture mistakes only (nullable tag, projection column count); fixed in c682aea and 5e7a038.
- The commit hook ran the gate clean on every commit.
