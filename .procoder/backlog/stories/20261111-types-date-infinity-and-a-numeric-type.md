# types: Date infinity and a Numeric type

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #105 (reported by the Kuvryn Orbit port, v0.7.0 audit).

Reproduce: create a PostgreSQL `date` column holding `'infinity'` or
`'-infinity'`, then scan it into `types.Date`; or try to model a `numeric`
column that holds `NaN`, `Infinity` or a value beyond `float64` precision.

Observed: `types.Date` wraps `time.Time` only, so it cannot hold `infinity` or
`-infinity` (the scan fails, and there is no way to write one). There is no
decimal type: `--from-db` and the generator carry `numeric` as a plain string
with no NaN or Infinity handling, and Orbit has 28 `pgtype.Numeric` sites.

Expected: `types.Date` carries `Infinity` and `NegInfinity` and has
`IsFinite()`; a new `types.Numeric` is a string-backed decimal with NaN and
Infinity flags and `Sign`, mapped to `numeric` by the generator and `--from-db`.
Both round-trip on PostgreSQL including the special values; on dialects
without them the special values are rejected with `ErrValidation`.

Root cause: `types.Date` was a one-field struct over `time.Time`, whose zero
value and layout have no representation for the PostgreSQL special dates, and
`types.Numeric` did not exist. The special-value rejection point for SQLite
(`internal/sqlshared` `checkArgs`, which already rejects a float NaN) did not
know either type, and the MySQL driver had no argument check at all.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] `types.Date` scans and writes `infinity` and `-infinity` through `Infinity` and `NegInfinity`, has `IsFinite()`, and the finite behavior is unchanged
- [x] `types.Numeric` holds digits, NaN, Infinity and -Infinity with `Sign`, `IsFinite`, `IsNaN`, `IsInfinite`, `ParseNumeric`
- [x] both round-trip on PostgreSQL, including the special values, and the finite values round-trip on SQLite and MySQL
- [x] a special Date or Numeric on SQLite or MySQL is `ErrValidation` and nothing is stored
- [x] the generator type map and `--from-db` map `numeric` to `types.Numeric`, keeping precision and scale
- [x] fuzz and unit tests cover parsing
- [x] `docs/TYPE_MAPPING.md` and `docs/upgrading.md` describe the change

## Evidence

- Red: run 42978764702 (branch worktree-agent-red-105, test-only commit 3104fbd, since deleted) failed to build: `col_type_mapping_integration_test.go:129:20: undefined: types.Numeric`, `types/codec_fuzz_test.go:72:8: d.IsFinite undefined (type Date has no field or method IsFinite)`, `types/codec_fuzz_test.go:87:9: undefined: Numeric`, `types/codec_test.go:23:16: undefined: Numeric`.
- Green: ci-agent run 42996856333 on sha 460405d9dfdc08d1beb61d43c9db3550459ce5e5, all 9 jobs success (unit, lint, integration postgres, integration-mysql mysql84/mysql97/mariadb, fuzz-smoke, apidiff-docs, bench-module).
- Date infinity, Numeric type and parsing: types/numeric_test.go, FuzzDateInfinityScan and FuzzNumericScan (in the Makefile FUZZ_TARGETS), green in unit and fuzz-smoke.
- PostgreSQL round trip of infinity, -infinity, NaN, Infinity, -Infinity and finite values, and ErrValidation on SQLite and MySQL (pool and transaction, nothing stored): types/codec_integration_test.go (DateInfinity, Numeric, NumericSpecial), internal/sqlshared/args_test.go, and the TestTypeMapping row for types.Numeric.
- Generator and --from-db: gen/typemap, gen/fromdb/mapping.go and TestTypeMappingDoc cover the new row; numeric(p,s) keeps precision and scale tags.
- Docs changed: docs/TYPE_MAPPING.md, docs/upgrading.md (Upgrading to 0.8.0), docs/migrations.md, docs/ai-guide.md, docs/reference.md (from the regenerated-docs artifact of run 42991131319).
- Review fix round: red run 43004754746 (throwaway branch worktree-agent-red-105b, deleted) failed `TestNumericPrecisionIsTextOffPostgres` (`sqlite amount: "numeric(12,2)", want TEXT`) and `TestCheckArgsSpecialInsideNull`. Green: ci-agent run 43011746988 on sha db96631027e9093476c205b7c74f13cbb0ea296f, all jobs success. Fixed: Numeric precision is TEXT off PostgreSQL (gen/schema.go), Scan takes float64, sql.Null[Date] and sql.Null[Numeric] are checked, gen/fromdb/numeric_test.go pins precision and scale, docs name PostgreSQL 14 for infinities.
