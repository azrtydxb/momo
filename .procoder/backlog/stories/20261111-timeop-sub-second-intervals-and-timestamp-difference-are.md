# timeop sub-second intervals and timestamp difference are missing

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #95. Two gaps in `timeop`.

1. Interval units start at `second`, so a TTL held as a `time.Duration` of
   1500ms cannot be added without rounding (Kuvryn-Owl binds
   `now() + $1::interval` with `"1500 milliseconds"` for lease expiry).
2. There is no difference of two timestamps (`a - b`, `age`, or
   `extract(epoch FROM a - b)`), so a pipeline duration percentile cannot stay in
   SQL (`percentile_cont(0.95) WITHIN GROUP (ORDER BY extract(epoch FROM
finished_at - started_at))`).

Reproduction: `timeop.IntervalOf(1500, timeop.IntervalMillisecond)` and
`timeop.Sub(a, b)` do not exist.

Observed: only whole-second and coarser units, and no subtraction of two
timestamps. Expected: millisecond and microsecond units, a `time.Duration` form,
and a timestamp difference readable as seconds, with a MySQL and SQLite rendering
or `ErrUnsupported`.

Root cause: `intervalUnits` was written with `second` as the finest unit and the
package has no operator whose result is an interval.

## Acceptance criteria

- [x] `IntervalMillisecond` and `IntervalMicrosecond` render exactly on PostgreSQL (`INTERVAL '1500 millisecond'`), as microseconds on MySQL and as fractional seconds on SQLite, for literal and bound counts (unit SQL tests)
- [x] `IntervalDuration`, `AddDuration` and `SubDuration` take a `time.Duration` and keep its sub-second part; counts out of range fail with `ErrValidation`
- [x] `Sub(a, b)`, `Age(a, b)` and `EpochSeconds` render `extract(epoch FROM a - b)` on PostgreSQL, `TIMESTAMPDIFF` on MySQL and `julianday` on SQLite; `Age` and a bare `Span` are `ErrUnsupported` where the dialect has no interval; nil operands are `ErrValidation`
- [x] on PostgreSQL a lease of +1500ms and a difference of 1.5 seconds, its negation and a 3 hour difference read back correctly (integration tests, SQLite too where it runs)
- [x] `docs/ai-guide.md`, `docs/capabilities.md`, `llms.txt` and `docs/upgrading.md` (0.8.0) describe the additions
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- Red: ci-agent run 42978437547 on 6c459f1 (tests only) failed to compile: `timeop/timeop_subsecond_test.go:25:39: undefined: timeop.IntervalMillisecond`, and `undefined: timeop.AddDuration`, `timeop.SubDuration`, `timeop.IntervalMicrosecond` in `timeop/timeop_subsecond_integration_test.go`.
- Green: ci-agent run 43006451426, headSha 34744ea0c4f9c460e5df530799a019229de80e64 (after merging sprint-9; SQLite rounding of 1500us and 1499us pinned in TestSubSecondIntervalIntegration; earlier green 42993926849 on 1ba675a), all jobs passed.
- Unit goldens for units, durations, Sub, Age, EpochSeconds, ErrUnsupported and ErrValidation: `timeop/timeop_subsecond_test.go`. Database round trips: `timeop/timeop_subsecond_integration_test.go`.
- Docs changed: `docs/ai-guide.md`, `docs/upgrading.md` (Upgrading to 0.8.0), `llms.txt`, `docs/capabilities.md` and `docs/reference.md` (generated: `internal/gendocs/blocks.go` plus CI artifacts).
