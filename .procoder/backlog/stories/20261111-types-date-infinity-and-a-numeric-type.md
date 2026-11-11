# types: Date infinity and a Numeric type

Status: open
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

- [ ] a regression test pins the fix: red before the change, green after
- [ ] `types.Date` scans and writes `infinity` and `-infinity` through `Infinity` and `NegInfinity`, has `IsFinite()`, and the finite behavior is unchanged
- [ ] `types.Numeric` holds digits, NaN, Infinity and -Infinity with `Sign`, `IsFinite`, `IsNaN`, `IsInfinite`, `ParseNumeric`
- [ ] both round-trip on PostgreSQL, including the special values, and the finite values round-trip on SQLite and MySQL
- [ ] a special Date or Numeric on SQLite or MySQL is `ErrValidation` and nothing is stored
- [ ] the generator type map and `--from-db` map `numeric` to `types.Numeric`, keeping precision and scale
- [ ] fuzz and unit tests cover parsing
- [ ] `docs/TYPE_MAPPING.md` and `docs/upgrading.md` describe the change

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
