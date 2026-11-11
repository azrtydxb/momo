# Raw SQL: Query[T] into a plain struct, no generated metadata

Status: done 2026-11-10
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`Query`/`QueryRows` accept a model, a generated projection or `Row1`..`Row10`. An ad-hoc struct does not work, so a raw read with more than ten columns needs a `momo.Projection` and a generator run. Scan by column name into any struct: exported fields, `momo:"column:x"` tags, snake_case of the field name otherwise, with cached reflection. GitHub issue #75.

## Acceptance criteria

- [x] the tests are shown failing on CI before the change
- [x] `momo.Query[T]` and `momo.QueryRows[T]` scan into a plain struct with no generated metadata, by column name; a test reads 13 columns into an ad-hoc struct on PostgreSQL, MySQL and SQLite
- [x] a result column with no matching field, or a field with no matching column, is an error naming it (or a documented rule says which is allowed)
- [x] the reflection plan is cached per type; a benchmark shows no per-row reflection cost beyond the first call

## Evidence

- Red first: commit d80ea46 (tests plus a stub that has the new signature but no plain-struct support) failed on ci-agent run 42904412952: unit, integration (postgres), mysql97 and mariadb jobs fail TestSnakeCase, TestQueryPlainStructScansByName, TestQueryRowsPlainStruct, TestQueryPlainStructUnmatchedIsNamedError, TestQueryPlainStructCaseFoldsColumnNames, TestQueryPlainStructRejectsBadTypes, TestQueryPlainStructOuterFieldShadowsEmbedded, TestQueryPlainStructWithoutColumnNames, and TestQueryPlainStructThirteenColumns on postgres, mysql97, mariadb and sqlite.
- Green: ci-agent run 42915567322 on c4cb09a, all 9 jobs, including TestQueryPlainStructThirteenColumns (13 columns, scrambled order, an embedded struct, a tag, a NULL pointer field) on PostgreSQL, MySQL 8.4/9.7, MariaDB and SQLite.
- Rule for an unmatched column or field: ErrValidation naming it, in both directions, plus a column named twice; documented in the raw_query.go comment, docs/ai-guide.md, docs/capabilities.md and docs/upgrading.md (0.7.0). Tested by TestQueryPlainStructUnmatchedIsNamedError (unit) and the unmatched case of the 13-column integration test.
- Cache: TestStructMetaIsBuiltOnceAndCached (same plan pointer on every call, a type error is cached too). Benchmark BenchmarkQueryPlainStruct (1000 rows x 13 columns) against BenchmarkQueryTupleRow, run on CI hardware (AMD EPYC 9V45, 3 runs each): plain struct 541 to 553 us/op, 868 KB/op, 2029 allocs/op; tuple row (2 columns, the metadata path) 83 to 91 us/op, 2019 allocs/op. So for a struct without embedded pointers the reflection path adds about 10 allocations per call and none per row (an embedded pointer struct is allocated per row, documented in docs/ai-guide.md); its time is about 42 ns per column. That run was a throwaway branch (worktree-agent-ad3b-bench, a lint-job step, deleted), since ci-agent does not run benchmarks.
- API: Query and QueryRows lose the PT type parameter (apidiff gate passes with the docs change); callers naming only T compile unchanged. Documented in docs/upgrading.md under 0.7.0.
