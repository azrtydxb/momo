# lock_timeout and statement_timeout of 0s cannot disable the timeout

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s4

## Description

migrate/options.go 33-34 and migrate/postgres/session.go 58 treat a zero duration as unset and apply the default, so a user cannot disable lock_timeout or statement_timeout (PostgreSQL treats 0 as off), for example for a long index build.

Expected: an explicit 0s sets the PostgreSQL timeout to 0; only an absent value uses the default.

## Acceptance criteria

- [x] momo.yaml with `statement_timeout: 0s` results in SET statement_timeout = 0 on the session
- [x] an absent key still uses the default
- [x] same for lock_timeout
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- TestMigrateTimeoutKeysReachTheSession/zero (momo.yaml 0s) and TestSetupTimeouts/explicit_zero_statement_timeout_is_sent: the session gets SET statement_timeout = 0. Mutation (Setup treats 0 as unset again) fails it. TestSessionTimeoutZeroOnPostgres and TestMigrateZeroTimeoutsInConfig check current_setting on the server in CI.
- TestMigrateTimeoutKeysReachTheSession/absent and TestSessionTimeouts/absent_keeps_the_defaults: lock_timeout 5s, statement_timeout untouched.
- TestSetupTimeouts/explicit_zero_lock_timeout_is_sent and TestSessionTimeouts/explicit_zero: SET lock_timeout = 0. Mutation (Set flag dropped in the engine) fails TestSessionTimeouts.
- Red first: SessionOptions had no way to say explicit zero, so a 0s key behaved as unset.
- Review fix: TestSetupTimeouts/sub-millisecond rounds a positive value up to 1ms instead of 0 (off).
