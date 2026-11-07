# Raw SQL: accept PostgreSQL $n placeholders

Status: open
Created: 2026-11-08
Epic: momo-05-writes-queries
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

GitHub issue #45, found by the Kuvryn Owl port from sqlc and pgx: 598 raw SQL literals use native `$n` placeholders and 52 of them reuse a position. Raw SQL (Exec, Scalars, SQL, TypedSQL, RawSubquery) accepted only `?` and `@name`, so each statement had to be rewritten by hand. Done means that on the PostgreSQL dialect a fragment can use `$n` instead of `?`, a reused `$n` binds once, momo renumbers it when the fragment is embedded in a larger statement, mistakes fail at build time, other dialects reject `$n`, and the porting guide explains the move.

## Acceptance criteria

- [x] `$n` binds argument n on PostgreSQL in Exec, Scalars, SQL, TypedSQL and RawSubquery, and a reused `$n` binds one argument (unit test in raw_args_internal_test.go)
- [x] a fragment embedded after earlier binds (Where, TypedSQL with a RawSubquery argument) is renumbered correctly (golden test in raw_test.go, internal builder test)
- [x] mixing `$n` with `?`, `@name` or Named, `$n` out of range, `$0` and an unused argument are ErrValidation (unit test)
- [x] `$n` inside string literals, quoted identifiers, dollar-quoted bodies and comments is text (unit test)
- [x] `$n` on SQLite and MySQL is ErrUnsupported at build time (unit test)
- [x] FuzzScanRaw exists and is listed in the Makefile FUZZ_TARGETS
- [x] docs/ai-guide.md has a section on porting pgx and sqlc SQL
- [x] integration test on CI runs `$n` statements on PostgreSQL and asserts ErrUnsupported on the other dialects (suite.RawArgs)

## Evidence
CI run 42414061640 on ecc70c7 (unit, integration postgres) green: unit tests, golden tests, FuzzScanRaw and suite.RawArgs all passed.
