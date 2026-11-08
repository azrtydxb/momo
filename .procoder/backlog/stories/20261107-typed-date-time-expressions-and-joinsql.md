# Typed date/time expressions and JoinSQL

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: -

## Description

GitHub issue #40. A port from Prisma needs EXTRACT(EPOCH FROM ..), AT TIME ZONE, interval arithmetic, now() and date_trunc, and a Prisma.join equivalent for composing dynamic WHERE fragments, all without hand-written raw SQL. As a Go developer using momo, I get typed Extract, AtTimeZone, IntervalOf, IntervalExpr, AddInterval, SubInterval, Now, CurrentTimestamp and DateTrunc that render correctly on PostgreSQL and have SQLite and MySQL equivalents where they exist (AtTimeZone is ErrUnsupported there), with every field and unit name checked against an allowlist, plus JoinSQL and JoinTypedSQL that join raw fragments with their arguments under a validated fixed separator.

## Acceptance criteria

- [x] Extract, DateTrunc, interval arithmetic and Now render the expected SQL on PostgreSQL, SQLite and MySQL (golden tests).
- [x] A field, unit or separator outside its allowlist fails the build with ErrValidation, on every dialect.
- [x] The expressions execute against a real PostgreSQL in CI and return the expected values.
- [x] JoinSQL keeps each part's arguments and numbers placeholders across the whole statement; empty and nil parts are refused.

## Evidence

- Golden tests (TestExtractGolden, TestIntervalGolden, TestNowGolden, TestDateTruncGolden, TestJoinSQLGolden) pass in the unit job of CI run 42410488648 on 802e18a.
- TestTimeKeywordsValidated and TestJoinSQLValidation pass in the same run: names outside the allowlists give ErrValidation on every dialect.
- TestExtractIntegration, TestDateTruncIntegration, TestIntervalAndNowIntegration and TestJoinSQLIntegration pass in the integration (postgres) job of the same run; lint and fuzz-smoke are green too.
- Refresh after review fixes: CI run 42411536064 on 19ed918 is green (unit, integration (postgres), lint, fuzz-smoke). It adds TestAtTimeZone literal goldens, TestTimeKeywordsValidated zone and nil-count cases, TestJoinSQLGolden MySQL CONCAT and empty AND/OR cases, TestBoundTimeOperandIntegration and TestGroupByLocalDayIntegration.
